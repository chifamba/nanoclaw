package telegram

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/env"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type botAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
	GetSelf() tgbotapi.User
}

type realBotAPI struct {
	*tgbotapi.BotAPI
}

func (b *realBotAPI) GetSelf() tgbotapi.User {
	return b.Self
}

type TelegramChannel struct {
	token   string
	opts    channel.ChannelOpts
	bot     botAPI
	stop    chan struct{}
	mu      sync.RWMutex
}

func init() {
	channel.RegisterChannel("telegram", func(opts channel.ChannelOpts) channel.Channel {
		envVars := env.ReadEnvFile([]string{"TELEGRAM_BOT_TOKEN"})
		token := os.Getenv("TELEGRAM_BOT_TOKEN")
		if token == "" {
			token = envVars["TELEGRAM_BOT_TOKEN"]
		}
		if token == "" {
			logger.Warn("Telegram: TELEGRAM_BOT_TOKEN not set")
			return nil
		}
		return &TelegramChannel{
			token: token,
			opts:  opts,
			stop:  make(chan struct{}),
		}
	})
}

func (c *TelegramChannel) Name() string {
	return "telegram"
}

func (c *TelegramChannel) Connect() error {
	bot, err := tgbotapi.NewBotAPI(c.token)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram bot: %w", err)
	}

	c.mu.Lock()
	c.bot = &realBotAPI{bot}
	c.mu.Unlock()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := c.bot.GetUpdatesChan(u)

	go func() {
		for {
			select {
			case <-c.stop:
				return
			case update, ok := <-updates:
				if !ok {
					return
				}
				if update.Message != nil {
					c.handleMessage(update.Message)
				}
			}
		}
	}()

	logger.Info(fmt.Sprintf("Telegram bot connected: @%s", bot.Self.UserName))
	fmt.Printf("\n  Telegram bot: @%s\n", bot.Self.UserName)
	fmt.Printf("  Send /chatid to the bot to get a chat's registration ID\n\n")

	return nil
}

func (c *TelegramChannel) handleMessage(msg *tgbotapi.Message) {
	if msg.IsCommand() {
		c.handleCommand(msg)
		return
	}

	chatJID := fmt.Sprintf("tg:%d", msg.Chat.ID)
	timestamp := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02T15:04:05.000Z")
	
	// Determine sender details
	sender := ""
	if msg.From != nil {
		sender = strconv.FormatInt(msg.From.ID, 10)
	}
	
	senderName := ""
	if msg.From != nil {
		senderName = msg.From.FirstName
		if senderName == "" {
			senderName = msg.From.UserName
		}
		if senderName == "" {
			senderName = sender
		}
	}
	if senderName == "" {
		senderName = "Unknown"
	}

	// Determine chat name
	chatName := ""
	if msg.Chat.Type == "private" {
		chatName = senderName
	} else {
		chatName = msg.Chat.Title
	}
	if chatName == "" {
		chatName = chatJID
	}

	// Store chat metadata for discovery
	isGroup := msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
	c.opts.OnChatMetadata(chatJID, timestamp, chatName, "telegram", isGroup)

	// Check if this is a text message
	if msg.Text != "" {
		content := msg.Text
		
		// Translate mentions
		c.mu.RLock()
		botSelf := c.bot.GetSelf()
		c.mu.RUnlock()
		
		botUsername := botSelf.UserName
		isBotMentioned := false
		if msg.Entities != nil {
			for _, entity := range msg.Entities {
				if entity.Type == "mention" {
					if entity.Offset+entity.Length <= len(content) {
						mention := content[entity.Offset : entity.Offset+entity.Length]
						if strings.ToLower(mention) == "@"+strings.ToLower(botUsername) {
							isBotMentioned = true
							break
						}
					}
				}
			}
		}

		if isBotMentioned && !config.TriggerPattern.MatchString(content) {
			content = fmt.Sprintf("@%s %s", config.AssistantName, content)
		}

		c.deliverMessage(chatJID, msg.MessageID, sender, senderName, content, timestamp)
	} else {
		// Handle non-text messages
		c.handleNonText(msg, chatJID, timestamp, sender, senderName)
	}
}

func (c *TelegramChannel) handleCommand(msg *tgbotapi.Message) {
	chatId := msg.Chat.ID
	switch msg.Command() {
	case "chatid":
		chatType := msg.Chat.Type
		chatName := ""
		if chatType == "private" {
			if msg.From != nil {
				chatName = msg.From.FirstName
			} else {
				chatName = "Private"
			}
		} else {
			chatName = msg.Chat.Title
		}
		if chatName == "" {
			chatName = "Unknown"
		}
		
		reply := fmt.Sprintf("Chat ID: `tg:%d`\nName: %s\nType: %s", chatId, chatName, chatType)
		response := tgbotapi.NewMessage(chatId, reply)
		response.ParseMode = tgbotapi.ModeMarkdown
		c.mu.RLock()
		bot := c.bot
		c.mu.RUnlock()
		if bot != nil {
			bot.Send(response)
		}
	case "ping":
		reply := fmt.Sprintf("%s is online.", config.AssistantName)
		response := tgbotapi.NewMessage(chatId, reply)
		c.mu.RLock()
		bot := c.bot
		c.mu.RUnlock()
		if bot != nil {
			bot.Send(response)
		}
	}
}

func (c *TelegramChannel) deliverMessage(chatJID string, msgID int, sender string, senderName string, content string, timestamp string) {
	// Only deliver for registered groups
	groups := c.opts.RegisteredGroups()
	if _, ok := groups[chatJID]; !ok {
		logger.Debug(fmt.Sprintf("Message from unregistered Telegram chat: %s", chatJID))
		return
	}

	c.opts.OnMessage(chatJID, types.NewMessage{
		ID:         strconv.Itoa(msgID),
		ChatJID:    chatJID,
		Sender:     sender,
		SenderName: senderName,
		Content:    content,
		Timestamp:  timestamp,
		IsFromMe:   false,
	})
	
	logger.Info(fmt.Sprintf("Telegram message stored: %s from %s", chatJID, senderName))
}

func (c *TelegramChannel) handleNonText(msg *tgbotapi.Message, chatJID string, timestamp string, sender string, senderName string) {
	placeholder := ""
	caption := msg.Caption
	if caption != "" {
		caption = " " + caption
	}

	if msg.Photo != nil {
		placeholder = "[Photo]"
	} else if msg.Video != nil {
		placeholder = "[Video]"
	} else if msg.Voice != nil {
		placeholder = "[Voice message]"
	} else if msg.Audio != nil {
		placeholder = "[Audio]"
	} else if msg.Document != nil {
		name := msg.Document.FileName
		if name == "" {
			name = "file"
		}
		placeholder = fmt.Sprintf("[Document: %s]", name)
	} else if msg.Sticker != nil {
		placeholder = fmt.Sprintf("[Sticker %s]", msg.Sticker.Emoji)
	} else if msg.Location != nil {
		placeholder = "[Location]"
	} else if msg.Contact != nil {
		placeholder = "[Contact]"
	}

	if placeholder != "" {
		c.deliverMessage(chatJID, msg.MessageID, sender, senderName, placeholder+caption, timestamp)
	}
}

func (c *TelegramChannel) SendMessage(jid string, text string) error {
	c.mu.RLock()
	bot := c.bot
	c.mu.RUnlock()
	if bot == nil {
		return fmt.Errorf("bot not initialized")
	}

	numericIdStr := strings.TrimPrefix(jid, "tg:")
	chatID, err := strconv.ParseInt(numericIdStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat jid: %s", jid)
	}

	// Telegram has a 4096 character limit per message — split if needed
	const MAX_LENGTH = 4096
	runes := []rune(text)
	if len(runes) <= MAX_LENGTH {
		return c.sendWithFallback(chatID, text)
	}

	for i := 0; i < len(runes); i += MAX_LENGTH {
		end := i + MAX_LENGTH
		if end > len(runes) {
			end = len(runes)
		}
		part := string(runes[i:end])
		if err := c.sendWithFallback(chatID, part); err != nil {
			return err
		}
	}

	logger.Info(fmt.Sprintf("Telegram message sent: %s", jid))
	return nil
}

func (c *TelegramChannel) sendWithFallback(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	
	c.mu.RLock()
	bot := c.bot
	c.mu.RUnlock()
	if bot == nil {
		return fmt.Errorf("bot disconnected")
	}

	_, err := bot.Send(msg)
	if err != nil {
		// Fallback: send as plain text if Markdown parsing fails
		logger.Debug(fmt.Sprintf("Markdown send failed, falling back to plain text: %v", err))
		msg.ParseMode = ""
		_, err = bot.Send(msg)
	}
	return err
}

func (c *TelegramChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bot != nil
}

func (c *TelegramChannel) OwnsJID(jid string) bool {
	return strings.HasPrefix(jid, "tg:")
}

func (c *TelegramChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bot != nil {
		close(c.stop)
		c.bot = nil
		logger.Info("Telegram bot stopped")
	}
	return nil
}

func (c *TelegramChannel) SyncGroups(force bool) error {
	// Telegram bots cannot easily list all groups they are in.
	// We discover groups as messages arrive.
	return nil
}

func (c *TelegramChannel) SetTyping(jid string, isTyping bool) error {
	c.mu.RLock()
	bot := c.bot
	c.mu.RUnlock()
	if bot == nil || !isTyping {
		return nil
	}

	numericIdStr := strings.TrimPrefix(jid, "tg:")
	chatID, err := strconv.ParseInt(numericIdStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat jid: %s", jid)
	}

	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, err = bot.Send(action)
	if err != nil {
		logger.Debug(fmt.Sprintf("Failed to send Telegram typing indicator: %v", err))
	}
	return nil
}
