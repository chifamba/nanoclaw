package slack

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSlackChannel_Name(t *testing.T) {
	c := &SlackChannel{}
	assert.Equal(t, "slack", c.Name())
}

func TestSlackChannel_OwnsJID(t *testing.T) {
	c := &SlackChannel{}
	assert.True(t, c.OwnsJID("slack:C12345"))
	assert.False(t, c.OwnsJID("tg:12345"))
}

func TestSlackChannel_IsConnected(t *testing.T) {
	c := &SlackChannel{}
	assert.False(t, c.IsConnected())
}
