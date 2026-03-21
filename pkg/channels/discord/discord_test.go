package discord

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscordChannel_Name(t *testing.T) {
	c := &DiscordChannel{}
	assert.Equal(t, "discord", c.Name())
}

func TestDiscordChannel_OwnsJID(t *testing.T) {
	c := &DiscordChannel{}
	assert.True(t, c.OwnsJID("discord:12345"))
	assert.False(t, c.OwnsJID("tg:12345"))
}

func TestDiscordChannel_IsConnected(t *testing.T) {
	c := &DiscordChannel{}
	assert.False(t, c.IsConnected())
}
