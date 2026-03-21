package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func init() {
	channel.RegisterChannel("gmail", NewGmailChannel)
}

type GmailChannel struct {
	opts      channel.ChannelOpts
	srv       *gmail.Service
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
}

func NewGmailChannel(opts channel.ChannelOpts) channel.Channel {
	return &GmailChannel{
		opts: opts,
	}
}

func (c *GmailChannel) Name() string {
	return "gmail"
}

func (c *GmailChannel) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	configDir := os.Getenv("GMAIL_CONFIG_DIR")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".gmail-mcp")
	}

	credentialsFile := filepath.Join(configDir, "gcp-oauth.keys.json")
	tokenFile := filepath.Join(configDir, "credentials.json")

	if _, err := os.Stat(credentialsFile); os.IsNotExist(err) {
		return fmt.Errorf("gmail credentials file not found at %s. run skill /add-gmail to set up", credentialsFile)
	}

	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return fmt.Errorf("unable to read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(b, gmail.GmailReadonlyScope, gmail.GmailSendScope, gmail.GmailModifyScope)
	if err != nil {
		return fmt.Errorf("unable to parse client secret file to config: %v", err)
	}

	if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
		return fmt.Errorf("gmail token file not found at %s. run skill /add-gmail to authorize", tokenFile)
	}

	tok, err := tokenFromFile(tokenFile)
	if err != nil {
		return fmt.Errorf("unable to read token from file: %v", err)
	}

	ctx := context.Background()
	client := config.Client(ctx, tok)
	srv, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("unable to retrieve Gmail client: %v", err)
	}

	c.srv = srv
	c.connected = true
	c.ctx, c.cancel = context.WithCancel(ctx)

	go c.pollLoop()

	logger.Info("Gmail channel connected", "configDir", configDir)
	return nil
}

func (c *GmailChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *GmailChannel) OwnsJID(jid string) bool {
	return strings.HasPrefix(jid, "gmail:")
}

func (c *GmailChannel) SyncGroups(force bool) error {
	// Gmail channel doesn't have "groups" in the traditional sense.
	// We could sync contacts, but for now it's a no-op.
	return nil
}

func (c *GmailChannel) SetTyping(jid string, isTyping bool) error {
	// Gmail doesn't support typing indicators.
	return nil
}

func (c *GmailChannel) SendMessage(jid string, text string) error {
	email := strings.TrimPrefix(jid, "gmail:")
	if email == "" {
		return fmt.Errorf("invalid gmail JID: %s", jid)
	}

	var message gmail.Message
	msgStr := fmt.Sprintf("To: %s\r\nSubject: Re: NanoClaw Response\r\n\r\n%s", email, text)
	message.Raw = base64.URLEncoding.EncodeToString([]byte(msgStr))

	_, err := c.srv.Users.Messages.Send("me", &message).Do()
	if err != nil {
		return fmt.Errorf("failed to send gmail message: %v", err)
	}

	return nil
}

func (c *GmailChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.cancel()
	c.connected = false
	return nil
}

func (c *GmailChannel) pollLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial poll to set lastID
	c.poll()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.poll()
		}
	}
}

func (c *GmailChannel) poll() {
	// Query: is:unread category:primary
	res, err := c.srv.Users.Messages.List("me").Q("is:unread category:primary").Do()
	if err != nil {
		logger.Error("Failed to list gmail messages", "err", err)
		return
	}

	// Messages are returned in reverse chronological order
	// Process from oldest to newest if possible, but for simplicity we'll just process them
	for i := len(res.Messages) - 1; i >= 0; i-- {
		m := res.Messages[i]
		msg, err := c.srv.Users.Messages.Get("me", m.Id).Format("full").Do()
		if err != nil {
			logger.Error("Failed to get gmail message", "id", m.Id, "err", err)
			continue
		}

		c.processMessage(msg)
	}
}

func (c *GmailChannel) processMessage(msg *gmail.Message) {
	var from, subject, body string
	for _, h := range msg.Payload.Headers {
		if h.Name == "From" {
			from = h.Value
		} else if h.Name == "Subject" {
			subject = h.Value
		}
	}

	body = extractBody(msg.Payload)

	email := extractEmail(from)
	if email == "" {
		return
	}

	chatJID := "gmail:" + email
	c.opts.OnChatMetadata(chatJID, time.Now().Format(time.RFC3339), from, "gmail", false)

	content := fmt.Sprintf("[Email from %s] Subject: %s\n\n%s", from, subject, body)

	c.opts.OnMessage(chatJID, types.NewMessage{
		ID:        msg.Id,
		ChatJID:   chatJID,
		Sender:    from,
		Content:   content,
		Timestamp: time.Now().Format(time.RFC3339),
	})

	// Mark as read
	err := c.srv.Users.Messages.BatchModify("me", &gmail.BatchModifyMessagesRequest{
		Ids:            []string{msg.Id},
		RemoveLabelIds: []string{"UNREAD"},
	}).Do()
	if err != nil {
		logger.Error("Failed to mark gmail message as read", "id", msg.Id, "err", err)
	}
}

func extractBody(payload *gmail.MessagePart) string {
	if payload.Body != nil && payload.Body.Data != "" {
		data, _ := base64.URLEncoding.DecodeString(payload.Body.Data)
		return string(data)
	}

	for _, p := range payload.Parts {
		if p.MimeType == "text/plain" && p.Body != nil && p.Body.Data != "" {
			data, _ := base64.URLEncoding.DecodeString(p.Body.Data)
			return string(data)
		}
		if p.MimeType == "multipart/alternative" || p.MimeType == "multipart/mixed" {
			if body := extractBody(p); body != "" {
				return body
			}
		}
	}
	return ""
}

var emailRegex = regexp.MustCompile(`<([^>]+)>|([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

func extractEmail(from string) string {
	matches := emailRegex.FindStringSubmatch(from)
	if len(matches) > 1 && matches[1] != "" {
		return matches[1]
	}
	if len(matches) > 2 && matches[2] != "" {
		return matches[2]
	}
	return ""
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}
