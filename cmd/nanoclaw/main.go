package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/channel"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/discord"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/gmail"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/slack"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/telegram"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/whatsapp"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/container"
	"github.com/nanoclaw/nanoclaw/pkg/db"
	"github.com/nanoclaw/nanoclaw/pkg/ipc"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/messagepoller"
	"github.com/nanoclaw/nanoclaw/pkg/proxy"
	"github.com/nanoclaw/nanoclaw/pkg/router"
	"github.com/nanoclaw/nanoclaw/pkg/scheduler"
	"github.com/nanoclaw/nanoclaw/pkg/taskqueue"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type mainApp struct {
	storage  db.Storage
	queue    *taskqueue.GroupQueue
	channels []channel.Channel
	mu       sync.RWMutex
}

func (a *mainApp) SendMessage(ctx context.Context, jid string, text string) error {
	ch := router.FindChannel(a.channels, jid)
	if ch == nil {
		return fmt.Errorf("no channel for JID: %s", jid)
	}
	return ch.SendMessage(jid, text)
}

func (a *mainApp) RegisteredGroups() map[string]types.RegisteredGroup {
	groups, err := a.storage.GetAllRegisteredGroups()
	if err != nil {
		logger.Error("Failed to get registered groups", "err", err)
		return make(map[string]types.RegisteredGroup)
	}
	return groups
}

func (a *mainApp) RegisterGroup(jid string, group types.RegisteredGroup) error {
	err := a.storage.SetRegisteredGroup(jid, group)
	if err != nil {
		return err
	}

	groupDir := container.ResolveGroupFolderPath(group.Folder)
	os.MkdirAll(filepath.Join(groupDir, "logs"), 0755)

	logger.Info("Group registered", "jid", jid, "name", group.Name, "folder", group.Folder)
	return nil
}

func (a *mainApp) SyncGroups(force bool) error {
	logger.Info("Syncing groups across all channels", "force", force)
	for _, ch := range a.channels {
		if s, ok := ch.(channel.SyncableChannel); ok {
			if err := s.SyncGroups(force); err != nil {
				logger.Error("Failed to sync groups for channel", "name", ch.Name(), "err", err)
			}
		}
	}
	return nil
}

func (a *mainApp) GetAvailableGroups() []ipc.AvailableGroup {
	chats, err := a.storage.GetAllChats()
	if err != nil {
		logger.Error("Failed to get all chats", "err", err)
		return nil
	}

	groups := a.RegisteredGroups()
	registeredJids := make(map[string]bool)
	for jid := range groups {
		registeredJids[jid] = true
	}

	var available []ipc.AvailableGroup
	for _, c := range chats {
		if c.JID == "__group_sync__" || c.IsGroup == 0 {
			continue
		}
		available = append(available, ipc.AvailableGroup{
			JID:          c.JID,
			Name:         c.Name,
			LastActivity: c.LastMessageTime,
			IsRegistered: registeredJids[c.JID],
		})
	}
	return available
}

func (a *mainApp) WriteGroupsSnapshot(groupFolder string, isMain bool, availableGroups []ipc.AvailableGroup, registeredJids []string) error {
	var adapted []container.AvailableGroup
	for _, g := range availableGroups {
		adapted = append(adapted, container.AvailableGroup{
			JID:          g.JID,
			Name:         g.Name,
			LastActivity: g.LastActivity,
			IsRegistered: g.IsRegistered,
		})
	}

	return container.WriteGroupsSnapshot(groupFolder, isMain, adapted)
}

func (a *mainApp) GetTaskByID(id string) (*types.ScheduledTask, error) {
	return a.storage.GetTaskById(id)
}

func (a *mainApp) CreateTask(task types.ScheduledTask) error {
	return a.storage.CreateTask(task)
}

func (a *mainApp) UpdateTask(id string, updates map[string]interface{}) error {
	return a.storage.UpdateTask(id, updates)
}

func (a *mainApp) DeleteTask(id string) error {
	return a.storage.DeleteTask(id)
}

func main() {
	config.Load()
	logger.Info("Starting NanoClaw", "assistant", config.AssistantName)

	// 1. Ensure data dirs exist
	os.MkdirAll(config.DataDir, 0755)
	os.MkdirAll(filepath.Join(config.DataDir, "ipc"), 0755)
	os.MkdirAll(config.StoreDir, 0755)
	os.MkdirAll(config.GroupsDir, 0755)

	// 2. Start credential proxy
	proxyServer, err := proxy.StartCredentialProxy(config.CredentialProxyPort, "0.0.0.0")
	if err != nil {
		logger.Error("Failed to start credential proxy", "err", err)
		os.Exit(1)
	}

	// 3. Initialize DB
	dbPath := filepath.Join(config.StoreDir, "messages.db")
	storage, err := db.NewSQLiteStorage(dbPath)
	if err != nil {
		logger.Error("Failed to initialize database", "err", err)
		os.Exit(1)
	}
	defer storage.Close()

	// 4. Initialize GroupQueue
	queue := taskqueue.NewGroupQueue()

	app := &mainApp{
		storage: storage,
		queue:   queue,
	}

	// 5. Setup channels
	opts := channel.ChannelOpts{
		OnMessage: func(chatJID string, msg types.NewMessage) {
			storage.StoreMessage(msg)
		},
		OnChatMetadata: func(chatJID string, timestamp string, name string, channel string, isGroup bool) {
			storage.StoreChatMetadata(chatJID, timestamp, &name, &channel, &isGroup)
		},
		RegisteredGroups: func() map[string]types.RegisteredGroup {
			return app.RegisteredGroups()
		},
	}

	for _, name := range channel.GetRegisteredChannelNames() {
		factory := channel.GetChannelFactory(name)
		if factory == nil {
			continue
		}
		ch := factory(opts)
		if ch == nil {
			logger.Warn("Channel configured but failed to initialize", "name", name)
			continue
		}
		if err := ch.Connect(); err != nil {
			logger.Error("Failed to connect channel", "name", name, "err", err)
			continue
		}
		app.channels = append(app.channels, ch)
	}

	if len(app.channels) == 0 {
		logger.Error("No channels connected", nil)
		os.Exit(1)
	}

	// 6. Start Subsystems
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Message Poller & Worker wiring
	poller := messagepoller.NewMessagePoller(storage, queue, app.channels, app)
	queue.SetProcessMessagesFn(poller.ProcessGroupMessages)
	go poller.Start(ctx)

	// Scheduler
	sched := scheduler.NewTaskScheduler(storage, queue)
	go sched.Start(ctx)

	// IPC Watcher
	watcher := ipc.NewWatcher(app)
	if err := watcher.Start(ctx); err != nil {
		logger.Error("Failed to start IPC watcher", "err", err)
	}

	// 7. Graceful Shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Shutdown signal received", "signal", sig)
	cancel()

	// Wait for grace period
	proxyServer.Shutdown(context.Background())
	queue.Shutdown(10 * time.Second)
	for _, ch := range app.channels {
		ch.Disconnect()
	}

	logger.Info("NanoClaw stopped")
}
