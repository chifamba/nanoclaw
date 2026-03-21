package router

import (
	"github.com/nanoclaw/nanoclaw/pkg/channel"
)

// FindChannel searches for a channel that owns the given JID.
func FindChannel(channels []channel.Channel, jid string) channel.Channel {
	for _, c := range channels {
		if c.OwnsJID(jid) {
			return c
		}
	}
	return nil
}
