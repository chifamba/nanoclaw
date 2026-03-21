package discord

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/env"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

type DiscordChannel struct {
	token   string
	opts    channel.ChannelOpts
	session *discordgo.Session
	stop    chan struct{}
	mu      sync.RWMutex
}

func init() {
	channel.RegisterChannel("discord", func(opts channel.ChannelOpts) channel.Channel {
		envVars := env.ReadEnvFile([]string{"DISCORD_BOT_TOKEN"})
		token := os.Getenv("DISCORD_BOT_TOKEN")
		if token == "" {
			token = envVars["DISCORD_BOT_TOKEN"]
		}
		if token == "" {
			logger.Warn("Discord: DISCORD_BOT_TOKEN not set")
			return nil
		}
		if !strings.HasPrefix(token, "Bot ") {
			token = "Bot " + token
		}
		return &DiscordChannel{
			token: token,
			opts:  opts,
			stop:  make(chan struct{}),
		}
	})
}

func (c *DiscordChannel) Name() string {
	return "discord"
}

func (c *DiscordChannel) Connect() error {
	dg, err := discordgo.New(c.token)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	dg.AddHandler(c.handleMessage)
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent

	c.mu.Lock()
	c.session = dg
	c.mu.Unlock()

	err = dg.Open()
	if err != nil {
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	logger.Info(fmt.Sprintf("Discord bot connected: %s#%s", dg.State.User.Username, dg.State.User.Discriminator))
	fmt.Printf("\n  Discord bot: %s#%s\n", dg.State.User.Username, dg.State.User.Discriminator)
	fmt.Printf("  The bot will respond to direct messages and mentions.\n\n")

	return nil
}

func (c *DiscordChannel) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	chatJID := fmt.Sprintf("discord:%s", m.ChannelID)
	timestamp := m.Timestamp.Format("2006-01-02T15:04:05.000Z")
	sender := m.Author.ID
	senderName := m.Author.Username
	
	// Determine chat name
	chatName := chatJID
	isGroup := true
	channel, err := s.State.Channel(m.ChannelID)
	if err == nil {
		chatName = channel.Name
		isGroup = channel.Type != discordgo.ChannelTypeDM
	}

	c.opts.OnChatMetadata(chatJID, timestamp, chatName, "discord", isGroup)

	content := m.Content
	
	// Check for bot mention
	isMentioned := false
	if !isGroup { // DM
		isMentioned = true
	} else {
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				isMentioned = true
				break
			}
		}
	}

	if isMentioned && !config.TriggerPattern.MatchString(content) {
		content = fmt.Sprintf("@%s %s", config.AssistantName, content)
	}

	c.deliverMessage(chatJID, m.ID, sender, senderName, content, timestamp)
}

func (c *DiscordChannel) deliverMessage(chatJID string, msgID string, sender string, senderName string, content string, timestamp string) {
	groups := c.opts.RegisteredGroups()
	if _, ok := groups[chatJID]; !ok {
		logger.Debug(fmt.Sprintf("Message from unregistered Discord channel: %s", chatJID))
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
	
	logger.Info(fmt.Sprintf("Discord message stored: %s from %s", chatJID, senderName))
}

func (c *DiscordChannel) SendMessage(jid string, text string) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("session not initialized")
	}

	channelID := strings.TrimPrefix(jid, "discord:")
	_, err := session.ChannelMessageSend(channelID, text)
	if err != nil {
		return fmt.Errorf("failed to send discord message: %w", err)
	}

	logger.Info(fmt.Sprintf("Discord message sent: %s", jid))
	return nil
}

func (c *DiscordChannel) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session != nil
}

func (c *DiscordChannel) OwnsJID(jid string) bool {
	return strings.HasPrefix(jid, "discord:")
}

func (c *DiscordChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		c.session.Close()
		c.session = nil
		logger.Info("Discord bot stopped")
	}
	return nil
}

func (c *DiscordChannel) SetTyping(jid string, isTyping bool) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil || !isTyping {
		return nil
	}

	channelID := strings.TrimPrefix(jid, "discord:")
	return session.ChannelTyping(channelID)
}

func (c *DiscordChannel) SyncGroups(force bool) error {
	c.mu.RLock()
	session := c.session
	c.mu.RUnlock()
	if session == nil {
		return fmt.Errorf("session not initialized")
	}

	timestamp := "now" // or time.Now().UTC().Format(...)
	
	// Private channels (DMs)
	for _, ch := range session.State.PrivateChannels {
		chatJID := fmt.Sprintf("discord:%s", ch.ID)
		chatName := "Direct Message"
		if len(ch.Recipients) > 0 {
			chatName = ch.Recipients[0].Username
		}
		c.opts.OnChatMetadata(chatJID, timestamp, chatName, "discord", false)
	}

	// Guild channels
	for _, guild := range session.State.Guilds {
		channels, err := session.GuildChannels(guild.ID)
		if err != nil {
			logger.Debug(fmt.Sprintf("Failed to get discord guild channels for %s: %v", guild.ID, err))
			continue
		}

		for _, ch := range channels {
			if ch.Type == discordgo.ChannelTypeGuildText {
				chatJID := fmt.Sprintf("discord:%s", ch.ID)
				c.opts.OnChatMetadata(chatJID, timestamp, ch.Name, "discord", true)
			}
		}
	}

	return nil
}
