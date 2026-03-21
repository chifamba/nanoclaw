package messagepoller

import (
	"context"
	"fmt"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/container"
	"github.com/nanoclaw/nanoclaw/pkg/db"
	"github.com/nanoclaw/nanoclaw/pkg/ipc"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/router"
	"github.com/nanoclaw/nanoclaw/pkg/senderallowlist"
	"github.com/nanoclaw/nanoclaw/pkg/taskqueue"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type MessagePollerDelegate interface {
	SendMessage(ctx context.Context, jid string, text string) error
	GetAvailableGroups() []ipc.AvailableGroup
	RegisteredGroups() map[string]types.RegisteredGroup
	WriteGroupsSnapshot(groupFolder string, isMain bool, availableGroups []ipc.AvailableGroup, registeredJids []string) error
}

type MessagePoller struct {
	storage  db.Storage
	queue    *taskqueue.GroupQueue
	Channels []channel.Channel
	delegate MessagePollerDelegate
	trigger  chan struct{}
}

func NewMessagePoller(storage db.Storage, queue *taskqueue.GroupQueue, channels []channel.Channel, delegate MessagePollerDelegate) *MessagePoller {
	return &MessagePoller{
		storage:  storage,
		queue:    queue,
		Channels: channels,
		delegate: delegate,
		trigger:  make(chan struct{}, 1),
	}
}

// Trigger wakes up the poller immediately instead of waiting for the next tick
func (p *MessagePoller) Trigger() {
	select {
	case p.trigger <- struct{}{}:
	default:
	}
}

func (p *MessagePoller) Start(ctx context.Context) {
	logger.Info("Message loop started", "assistant", config.AssistantName)

	ticker := time.NewTicker(time.Duration(config.PollInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.poll(); err != nil {
				logger.Error("Error in message loop", "err", err)
			}
		case <-p.trigger:
			if err := p.poll(); err != nil {
				logger.Error("Error in message loop (triggered)", "err", err)
			}
		}
	}
}

func (p *MessagePoller) poll() error {
	groups, err := p.storage.GetAllRegisteredGroups()
	if err != nil {
		return err
	}

	jids := make([]string, 0, len(groups))
	for jid := range groups {
		jids = append(jids, jid)
	}

	lastTsPtr, err := p.storage.GetRouterState("last_timestamp")
	if err != nil {
		return err
	}
	lastTs := ""
	if lastTsPtr != nil {
		lastTs = *lastTsPtr
	}

	messages, newTimestamp, err := p.storage.GetNewMessages(jids, lastTs, config.AssistantName, 100)
	if err != nil {
		return err
	}

	if len(messages) > 0 {
		logger.Info("New messages", "count", len(messages))

		if err := p.storage.SetRouterState("last_timestamp", newTimestamp); err != nil {
			return err
		}

		messagesByGroup := make(map[string][]types.NewMessage)
		for _, msg := range messages {
			messagesByGroup[msg.ChatJID] = append(messagesByGroup[msg.ChatJID], msg)
		}

		for chatJid := range messagesByGroup {
			if _, ok := groups[chatJid]; !ok {
				continue
			}

			// pull all messages since lastAgentTimestamp to ensure no gaps
			lastAgentTsKey := fmt.Sprintf("last_agent_timestamp_%s", chatJid)
			lastAgentTsPtr, _ := p.storage.GetRouterState(lastAgentTsKey)
			lastAgentTs := ""
			if lastAgentTsPtr != nil {
				lastAgentTs = *lastAgentTsPtr
			}

			allPending, err := p.storage.GetMessagesSince(chatJid, lastAgentTs, config.AssistantName, 100)
			if err != nil {
				logger.Error("Failed to get messages since", "chatJid", chatJid, "err", err)
				continue
			}

			if len(allPending) == 0 {
				continue
			}

			formatted := router.FormatMessages(allPending, config.Timezone)

			if p.queue.SendMessage(chatJid, formatted) {
				logger.Debug("Piped messages to active container", "chatJid", chatJid, "count", len(allPending))
				lastMsgTs := allPending[len(allPending)-1].Timestamp
				_ = p.storage.SetRouterState(lastAgentTsKey, lastMsgTs)
			} else {
				p.queue.EnqueueMessageCheck(chatJid)
			}
		}
	}

	return nil
}

// ProcessGroupMessages is the worker function for GroupQueue
func (p *MessagePoller) ProcessGroupMessages(chatJid string) (bool, error) {
	group, err := p.storage.GetRegisteredGroup(chatJid)
	if err != nil || group == nil {
		return true, nil
	}

	ch := router.FindChannel(p.Channels, chatJid)
	if ch == nil {
		logger.Warn("No channel owns JID, skipping messages", "chatJid", chatJid)
		return true, nil
	}

	isMain := group.IsMain
	lastAgentTsKey := fmt.Sprintf("last_agent_timestamp_%s", chatJid)
	lastAgentTsPtr, _ := p.storage.GetRouterState(lastAgentTsKey)
	lastAgentTs := ""
	if lastAgentTsPtr != nil {
		lastAgentTs = *lastAgentTsPtr
	}

	missedMessages, err := p.storage.GetMessagesSince(chatJid, lastAgentTs, config.AssistantName, 100)
	if err != nil {
		return true, err
	}

	if len(missedMessages) == 0 {
		return true, nil
	}

	// Trigger check
	if !isMain && (group.RequiresTrigger == nil || *group.RequiresTrigger) {
		allowlistCfg := senderallowlist.LoadSenderAllowlist("")
		hasTrigger := false
		for _, m := range missedMessages {
			if config.TriggerPattern.MatchString(m.Content) && (m.IsFromMe || senderallowlist.IsTriggerAllowed(chatJid, m.Sender, allowlistCfg)) {
				hasTrigger = true
				break
			}
		}
		if !hasTrigger {
			return true, nil
		}
	}

	prompt := router.FormatMessages(missedMessages, config.Timezone)
	sessionIDPtr, _ := p.storage.GetSession(group.Folder)
	sessionID := ""
	if sessionIDPtr != nil {
		sessionID = *sessionIDPtr
	}

	// Cursor advancement
	previousCursor := lastAgentTs
	lastMsgTs := missedMessages[len(missedMessages)-1].Timestamp
	_ = p.storage.SetRouterState(lastAgentTsKey, lastMsgTs)

	logger.Info("Processing messages", "group", group.Name, "count", len(missedMessages))

	// Snapshots
	tasks, _ := p.storage.GetAllTasks()
	container.WriteTasksSnapshot(group.Folder, isMain, tasks)
	
	availableGroups := p.delegate.GetAvailableGroups()
	registeredJids := []string{}
	for jid := range p.delegate.RegisteredGroups() {
		registeredJids = append(registeredJids, jid)
	}
	p.delegate.WriteGroupsSnapshot(group.Folder, isMain, availableGroups, registeredJids)

	// Typing indicator (best effort)
	if t, ok := ch.(channel.TypingChannel); ok {
		t.SetTyping(chatJid, true)
		defer t.SetTyping(chatJid, false)
	}

	var outputSentToUser bool
	var newSessionID string

	input := container.ContainerInput{
		Prompt:        prompt,
		SessionID:     sessionID,
		GroupFolder:   group.Folder,
		ChatJid:       chatJid,
		IsMain:        isMain,
		AssistantName: config.AssistantName,
	}

	output, err := container.RunContainerAgent(
		*group,
		input,
		func(proc container.ProcessInfo) {
			p.queue.RegisterProcess(chatJid, proc.ContainerName, group.Folder)
		},
		func(out container.ContainerOutput) {
			if out.NewSessionID != "" {
				newSessionID = out.NewSessionID
				_ = p.storage.SetSession(group.Folder, newSessionID)
			}
			if out.Result != "" {
				text := router.StripInternalTags(out.Result)
				if text != "" {
					p.delegate.SendMessage(context.Background(), chatJid, text)
					outputSentToUser = true
				}
				p.queue.NotifyIdle(chatJid)
			}
		},
	)

	if err != nil || output.Status == "error" {
		if outputSentToUser {
			logger.Warn("Agent error after output sent, skipping cursor rollback", "group", group.Name)
			return true, nil
		}
		_ = p.storage.SetRouterState(lastAgentTsKey, previousCursor)
		logger.Warn("Agent error, rolled back cursor", "group", group.Name, "err", err, "outErr", output.Error)
		return false, nil
	}

	return true, nil
}
