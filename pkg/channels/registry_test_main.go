package main

import (
	"fmt"
	"github.com/nanoclaw/nanoclaw/pkg/channel"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/slack"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/discord"
	_ "github.com/nanoclaw/nanoclaw/pkg/channels/telegram"
)

func main() {
	names := channel.GetRegisteredChannelNames()
	fmt.Printf("Registered channels: %v\n", names)
}
