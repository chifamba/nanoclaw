package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	waTypes "go.mau.fi/whatsmeow/types"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/protobuf/proto"
)

func init() {
	channel.RegisterChannel("whatsapp", NewWhatsAppChannel)
}

type WhatsAppChannel struct {
	opts      channel.ChannelOpts
	client    *whatsmeow.Client
	connected bool
	mu        sync.RWMutex
}

func NewWhatsAppChannel(opts channel.ChannelOpts) channel.Channel {
	return &WhatsAppChannel{
		opts: opts,
	}
}

func (c *WhatsAppChannel) Name() string {
	return "whatsapp"
}

func (c *WhatsAppChannel) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	dbPath := filepath.Join(config.StoreDir, "whatsapp.db")
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), waLog.Noop)
	if err != nil {
		return fmt.Errorf("failed to open whatsapp store: %v", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get whatsapp device: %v", err)
	}

	client := whatsmeow.NewClient(deviceStore, waLog.Noop)
	c.client = client
	client.AddEventHandler(c.handleEvent)

	if client.Store.ID == nil {
		// No ID stored, need to login with QR
		qrChan, _ := client.GetQRChannel(ctx)
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect to whatsapp: %v", err)
		}
		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
					fmt.Println("WhatsApp: Scan the QR code above to login")
				} else {
					fmt.Println("WhatsApp: Login event:", evt.Event)
				}
			}
		}()
	} else {
		// Already logged in, just connect
		err = client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect to whatsapp: %v", err)
		}
	}

	c.connected = true
	return nil
}

func (c *WhatsAppChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.client.IsConnected()
}

func (c *WhatsAppChannel) OwnsJID(jid string) bool {
	return strings.HasSuffix(jid, "@s.whatsapp.net") || strings.HasSuffix(jid, "@g.us")
}

func (c *WhatsAppChannel) SendMessage(jid string, text string) error {
	targetJID, err := waTypes.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid whatsapp JID: %v", err)
	}

	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	_, err = c.client.SendMessage(context.Background(), targetJID, msg)
	return err
}

func (c *WhatsAppChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.client.Disconnect()
	c.connected = false
	return nil
}

func (c *WhatsAppChannel) SetTyping(jid string, isTyping bool) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected || !c.client.IsConnected() {
		return fmt.Errorf("whatsapp not connected")
	}

	targetJID, err := waTypes.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("invalid whatsapp JID: %v", err)
	}

	presence := waTypes.ChatPresenceComposing
	if !isTyping {
		presence = waTypes.ChatPresencePaused
	}

	return c.client.SendChatPresence(context.Background(), targetJID, presence, waTypes.ChatPresenceMediaText)
}

func (c *WhatsAppChannel) SyncGroups(force bool) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.connected || !c.client.IsConnected() {
		return fmt.Errorf("whatsapp not connected")
	}

	groups, err := c.client.GetJoinedGroups(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get joined groups: %v", err)
	}

	for _, g := range groups {
		c.opts.OnChatMetadata(g.JID.String(), time.Now().Format(time.RFC3339), g.Name, "whatsapp", true)
	}
	return nil
}

func (c *WhatsAppChannel) handleEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		c.handleMessage(v)
	case *events.Receipt:
		if v.Type == events.ReceiptTypeRead || v.Type == events.ReceiptTypeReadSelf {
			// Handle read receipts if needed
		}
	case *events.Connected, *events.PushNameSetting:
		if c.client.Store.ID != nil && len(c.client.Store.ID.String()) > 0 {
			// Sync groups when connected
			go c.SyncGroups(false)
		}
	}
}

func (c *WhatsAppChannel) handleMessage(evt *events.Message) {
	// Skip messages from self unless needed
	if evt.Info.IsFromMe {
		return
	}

	chatJID := evt.Info.Chat.String()
	senderJID := evt.Info.Sender.String()
	senderName := evt.Info.PushName
	if senderName == "" {
		senderName = evt.Info.Sender.User
	}

	var content string
	if msg := evt.Message.GetConversation(); msg != "" {
		content = msg
	} else if msg := evt.Message.GetExtendedTextMessage().GetText(); msg != "" {
		content = msg
	} else {
		// Non-text message
		return
	}

	c.opts.OnChatMetadata(chatJID, evt.Info.Timestamp.Format(time.RFC3339), evt.Info.Chat.User, "whatsapp", evt.Info.IsGroup)

	c.opts.OnMessage(chatJID, types.NewMessage{
		ID:         evt.Info.ID,
		ChatJID:    chatJID,
		Sender:     senderJID,
		SenderName: senderName,
		Content:    content,
		Timestamp:  evt.Info.Timestamp.Format(time.RFC3339),
	})
}
