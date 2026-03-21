package telegram

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockBotAPI is a mock of the botAPI interface
type MockBotAPI struct {
	mock.Mock
}

func (m *MockBotAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	args := m.Called(c)
	return args.Get(0).(tgbotapi.Message), args.Error(1)
}

func (m *MockBotAPI) GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	args := m.Called(config)
	return args.Get(0).(tgbotapi.UpdatesChannel)
}

func (m *MockBotAPI) GetSelf() tgbotapi.User {
	args := m.Called()
	return args.Get(0).(tgbotapi.User)
}

func TestTelegramChannel_Name(t *testing.T) {
	c := &TelegramChannel{}
	assert.Equal(t, "telegram", c.Name())
}

func TestTelegramChannel_OwnsJID(t *testing.T) {
	c := &TelegramChannel{}
	assert.True(t, c.OwnsJID("tg:12345"))
	assert.True(t, c.OwnsJID("tg:abcdef"))
	assert.False(t, c.OwnsJID("wa:12345"))
	assert.False(t, c.OwnsJID("12345"))
}

func TestTelegramChannel_IsConnected(t *testing.T) {
	c := &TelegramChannel{}
	assert.False(t, c.IsConnected())

	c.bot = &MockBotAPI{}
	assert.True(t, c.IsConnected())
}

func TestTelegramChannel_handleMessage_DeliverMessage(t *testing.T) {
	mockBot := &MockBotAPI{}
	mockBot.On("GetSelf").Return(tgbotapi.User{UserName: "test_bot"})

	delivered := false
	opts := channel.ChannelOpts{
		OnMessage: func(chatJID string, msg types.NewMessage) {
			delivered = true
			assert.Equal(t, "tg:100", chatJID)
			assert.Equal(t, "Hello", msg.Content)
			assert.Equal(t, "999", msg.Sender)
		},
		OnChatMetadata: func(chatJID string, timestamp string, name string, channel string, isGroup bool) {
			assert.Equal(t, "tg:100", chatJID)
			assert.Equal(t, "telegram", channel)
		},
		RegisteredGroups: func() map[string]types.RegisteredGroup {
			return map[string]types.RegisteredGroup{
				"tg:100": {Name: "Test Group"},
			}
		},
	}

	c := &TelegramChannel{
		bot:  mockBot,
		opts: opts,
	}

	msg := &tgbotapi.Message{
		MessageID: 1,
		Date:      1600000000,
		Chat: &tgbotapi.Chat{
			ID:   100,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        999,
			FirstName: "Test User",
		},
		Text: "Hello",
	}

	c.handleMessage(msg)
	assert.True(t, delivered)
}

func TestTelegramChannel_handleMessage_Mentions(t *testing.T) {
	mockBot := &MockBotAPI{}
	mockBot.On("GetSelf").Return(tgbotapi.User{UserName: "test_bot"})

	delivered := false
	opts := channel.ChannelOpts{
		OnMessage: func(chatJID string, msg types.NewMessage) {
			delivered = true
			// TriggerPattern defaults to @AssistantName in config
			// So @test_bot Hello should become @AssistantName @test_bot Hello
			assert.Contains(t, msg.Content, "@"+config.AssistantName)
			assert.Contains(t, msg.Content, "@test_bot")
		},
		OnChatMetadata: func(chatJID string, timestamp string, name string, channel string, isGroup bool) {},
		RegisteredGroups: func() map[string]types.RegisteredGroup {
			return map[string]types.RegisteredGroup{
				"tg:100": {Name: "Test Group"},
			}
		},
	}

	c := &TelegramChannel{
		bot:  mockBot,
		opts: opts,
	}

	msg := &tgbotapi.Message{
		MessageID: 1,
		Date:      1600000000,
		Chat: &tgbotapi.Chat{
			ID:   100,
			Type: "private",
		},
		From: &tgbotapi.User{
			ID:        999,
			FirstName: "Test User",
		},
		Text: "@test_bot Hello",
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 0, Length: 9},
		},
	}

	c.handleMessage(msg)
	assert.True(t, delivered)
}
