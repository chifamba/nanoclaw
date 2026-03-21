package router

import (
	"testing"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	"github.com/stretchr/testify/assert"
)

type mockChannel struct {
	name string
	jids []string
}

func (m *mockChannel) Name() string { return m.name }
func (m *mockChannel) Connect() error { return nil }
func (m *mockChannel) SendMessage(jid string, text string) error { return nil }
func (m *mockChannel) IsConnected() bool { return true }
func (m *mockChannel) OwnsJID(jid string) bool {
	for _, j := range m.jids {
		if j == jid {
			return true
		}
	}
	return false
}
func (m *mockChannel) Disconnect() error { return nil }

func TestFindChannel(t *testing.T) {
	c1 := &mockChannel{name: "c1", jids: []string{"jid1", "jid2"}}
	c2 := &mockChannel{name: "c2", jids: []string{"jid3"}}
	channels := []channel.Channel{c1, c2}

	t.Run("finds existing channel for JID", func(t *testing.T) {
		assert.Equal(t, c1, FindChannel(channels, "jid1"))
		assert.Equal(t, c1, FindChannel(channels, "jid2"))
		assert.Equal(t, c2, FindChannel(channels, "jid3"))
	})

	t.Run("returns nil for unknown JID", func(t *testing.T) {
		assert.Nil(t, FindChannel(channels, "unknown"))
	})
}
