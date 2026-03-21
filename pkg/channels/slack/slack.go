package slack

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/env"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type SlackChannel struct {
	botToken  string
	appToken  string
	opts      channel.ChannelOpts
	api       *slack.Client
	socket    *socketmode.Client
	stop      chan struct{}
	mu        sync.RWMutex
	botUserID string
}

func init() {
	channel.RegisterChannel("slack", func(opts channel.ChannelOpts) channel.Channel {
		envVars := env.ReadEnvFile([]string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"})
		botToken := os.Getenv("SLACK_BOT_TOKEN")
		if botToken == "" {
			botToken = envVars["SLACK_BOT_TOKEN"]
		}
		appToken := os.Getenv("SLACK_APP_TOKEN")
		if appToken == "" {
			appToken = envVars["SLACK_APP_TOKEN"]
		}
		if botToken == "" || appToken == "" {
			logger.Warn("Slack: SLACK_BOT_TOKEN or SLACK_APP_TOKEN not set")
			return nil
		}
		return &SlackChannel{
			botToken: botToken,
			appToken: appToken,
			opts:     opts,
			stop:     make(chan struct{}),
		}
	})
}

func (c *SlackChannel) Name() string {
	return "slack"
}

func (c *SlackChannel) Connect() error {
	api := slack.New(
		c.botToken,
		slack.OptionAppLevelToken(c.appToken),
	)

	socket := socketmode.New(api)

	c.mu.Lock()
	c.api = api
	c.socket = socket
	c.mu.Unlock()

	authTest, err := api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack auth test failed: %w", err)
	}
	c.botUserID = authTest.UserID

	go func() {
		for {
			select {
			case <-c.stop:
				return
			case evt := <-socket.Events:
				switch evt.Type {
				case socketmode.EventTypeEventsAPI:
					eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok {
						continue
					}
					socket.Ack(*evt.Request)

					switch eventsAPIEvent.Type {
					case slackevents.CallbackEvent:
						innerEvent := eventsAPIEvent.InnerEvent
						switch ev := innerEvent.Data.(type) {
						case *slackevents.MessageEvent:
							c.handleMessage(ev)
						case *slackevents.AppMentionEvent:
							c.handleAppMention(ev)
						}
					}
				}
			}
		}
	}()

	go func() {
		err := socket.Run()
		if err != nil {
			logger.Error(fmt.Sprintf("Slack socket mode error: %v", err))
		}
	}()

	logger.Info(fmt.Sprintf("Slack bot connected: %s (ID: %s)", authTest.User, authTest.UserID))
	fmt.Printf("\n  Slack bot: %s (ID: %s)\n", authTest.User, authTest.UserID)
	fmt.Printf("  The bot will respond to direct messages and mentions in channels.\n\n")

	return nil
}

func (c *SlackChannel) handleMessage(ev *slackevents.MessageEvent) {
	// Ignore messages from bots
	if ev.BotID != "" || ev.User == "" {
		return
	}

	// Only process text messages
	if ev.Text == "" {
		return
	}

	chatJID := fmt.Sprintf("slack:%s", ev.Channel)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	sender := ev.User
	
	// Default sender name to user ID
	senderName := sender
	
	// If we have API, try to get more info
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()

	chatName := chatJID
	isGroup := !strings.HasPrefix(ev.Channel, "D") // 'D' is for IM in Slack

	if api != nil {
		user, err := api.GetUserInfo(sender)
		if err == nil {
			senderName = user.RealName
			if senderName == "" {
				senderName = user.Name
			}
		}

		info, err := api.GetConversationInfo(&slack.GetConversationInfoInput{
			ChannelID: ev.Channel,
		})
		if err == nil {
			chatName = info.Name
			isGroup = (info.IsChannel || info.IsGroup) && !info.IsIM
		}
	}

	c.opts.OnChatMetadata(chatJID, timestamp, chatName, "slack", isGroup)

	content := ev.Text
	
	// Check for bot mention in text if it's not a DM
	isMentioned := false
	if !isGroup { // DM
		isMentioned = true
	} else if strings.Contains(content, "<@"+c.botUserID+">") {
		isMentioned = true
	}

	if isMentioned && !config.TriggerPattern.MatchString(content) {
		content = fmt.Sprintf("@%s %s", config.AssistantName, content)
	}

	c.deliverMessage(chatJID, ev.EventTimeStamp, sender, senderName, content, timestamp)
}

func (c *SlackChannel) handleAppMention(ev *slackevents.AppMentionEvent) {
	chatJID := fmt.Sprintf("slack:%s", ev.Channel)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	sender := ev.User
	
	senderName := sender
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api != nil {
		user, err := api.GetUserInfo(sender)
		if err == nil {
			senderName = user.RealName
			if senderName == "" {
				senderName = user.Name
			}
		}
	}

	content := ev.Text
	if !config.TriggerPattern.MatchString(content) {
		content = fmt.Sprintf("@%s %s", config.AssistantName, content)
	}

	c.deliverMessage(chatJID, ev.EventTimeStamp, sender, senderName, content, timestamp)
}

func (c *SlackChannel) deliverMessage(chatJID string, msgID string, sender string, senderName string, content string, timestamp string) {
	groups := c.opts.RegisteredGroups()
	if _, ok := groups[chatJID]; !ok {
		logger.Debug(fmt.Sprintf("Message from unregistered Slack channel: %s", chatJID))
		return
	}

	c.opts.OnMessage(chatJID, types.NewMessage{
		ID:         msgID,
		ChatJID:    chatJID,
		Sender:     sender,
		SenderName: senderName,
		Content:    content,
		Timestamp:  timestamp,
		IsFromMe:   false,
	})
	
	logger.Info(fmt.Sprintf("Slack message stored: %s from %s", chatJID, senderName))
}

func (c *SlackChannel) SendMessage(jid string, text string) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("api not initialized")
	}

	channelID := strings.TrimPrefix(jid, "slack:")
	_, _, err := api.PostMessage(channelID, slack.MsgOptionText(text, false))
	if err != nil {
		return fmt.Errorf("failed to send slack message: %w", err)
	}

	logger.Info(fmt.Sprintf("Slack message sent: %s", jid))
	return nil
}

func (c *SlackChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.api != nil
}

func (c *SlackChannel) OwnsJID(jid string) bool {
	return strings.HasPrefix(jid, "slack:")
}

func (c *SlackChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.socket != nil {
		close(c.stop)
		c.socket = nil
		c.api = nil
		logger.Info("Slack bot stopped")
	}
	return nil
}

func (c *SlackChannel) SetTyping(jid string, isTyping bool) error {
	// Slack API doesn't support a simple way to send typing indicators via SocketMode/Web API.
	// RTM has SendUserTyping, but we're not using RTM.
	return nil
}

func (c *SlackChannel) SyncGroups(force bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("api not initialized")
	}

	params := &slack.GetConversationsParameters{
		Types: []string{"public_channel", "private_channel", "mpim", "im"},
	}

	for {
		channels, nextCursor, err := api.GetConversations(params)
		if err != nil {
			return fmt.Errorf("failed to get slack conversations: %w", err)
		}

		for _, ch := range channels {
			chatJID := fmt.Sprintf("slack:%s", ch.ID)
			chatName := ch.Name
			if ch.IsIM {
				chatName = "Direct Message"
			}
			isGroup := (ch.IsChannel || ch.IsGroup) && !ch.IsIM
			c.opts.OnChatMetadata(chatJID, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), chatName, "slack", isGroup)
		}

		if nextCursor == "" {
			break
		}
		params.Cursor = nextCursor
	}

	return nil
}
