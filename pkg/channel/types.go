package channel

import (
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// OnInboundMessage is a callback for incoming messages.
type OnInboundMessage func(chatJID string, message types.NewMessage)

// OnChatMetadata is a callback for chat metadata updates.
type OnChatMetadata func(chatJID string, timestamp string, name string, channel string, isGroup bool)

// ChannelOpts contains options for initializing a channel.
type ChannelOpts struct {
	OnMessage        OnInboundMessage
	OnChatMetadata   OnChatMetadata
	RegisteredGroups func() map[string]types.RegisteredGroup
}

// ChannelFactory is a function that creates a channel.
type ChannelFactory func(opts ChannelOpts) Channel

// Channel defines the interface for a messaging channel (e.g., Telegram, WhatsApp).
type Channel interface {
	Name() string
	Connect() error
	SendMessage(jid string, text string) error
	IsConnected() bool
	OwnsJID(jid string) bool
	Disconnect() error
}

// TypingChannel is an optional interface for channels that support typing indicators.
type TypingChannel interface {
	SetTyping(jid string, isTyping bool) error
}

// SyncableChannel is an optional interface for channels that support group synchronization.
type SyncableChannel interface {
	SyncGroups(force bool) error
}
