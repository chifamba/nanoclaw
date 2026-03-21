package senderallowlist

import (
	"encoding/json"
	"os"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

// DefaultConfig is the configuration used when no file is found or on errors.
var DefaultConfig = SenderAllowlistConfig{
	Default: ChatAllowlistEntry{
		Allow: AllowValue{IsWildcard: true},
		Mode:  "trigger",
	},
	Chats:     make(map[string]ChatAllowlistEntry),
	LogDenied: true,
}

func isRawEntryValid(raw interface{}) bool {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}

	allow, hasAllow := m["allow"]
	if !hasAllow {
		return false
	}

	// allow must be '*' or []interface{} (where all are strings)
	if s, ok := allow.(string); ok {
		if s != "*" {
			return false
		}
	} else if l, ok := allow.([]interface{}); ok {
		for _, v := range l {
			if _, ok := v.(string); !ok {
				return false
			}
		}
	} else {
		return false
	}

	mode, _ := m["mode"].(string)
	if mode != "trigger" && mode != "drop" {
		return false
	}

	return true
}

// LoadSenderAllowlist loads the configuration from the given path or the default path.
func LoadSenderAllowlist(pathOverride string) SenderAllowlistConfig {
	filePath := pathOverride
	if filePath == "" {
		filePath = config.SenderAllowlistPath
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return DefaultConfig
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return DefaultConfig
	}

	var cfg SenderAllowlistConfig

	// Handle logDenied default: true if not explicitly false
	cfg.LogDenied = true
	if ld, ok := raw["logDenied"].(bool); ok {
		cfg.LogDenied = ld
	}

	// Handle default entry
	defaultRaw, hasDefault := raw["default"]
	if !hasDefault || !isRawEntryValid(defaultRaw) {
		return DefaultConfig
	}
	defaultData, _ := json.Marshal(defaultRaw)
	_ = json.Unmarshal(defaultData, &cfg.Default)

	// Handle chats
	cfg.Chats = make(map[string]ChatAllowlistEntry)
	if chatsRaw, ok := raw["chats"].(map[string]interface{}); ok {
		for jid, entryRaw := range chatsRaw {
			if isRawEntryValid(entryRaw) {
				entryData, _ := json.Marshal(entryRaw)
				var entry ChatAllowlistEntry
				_ = json.Unmarshal(entryData, &entry)
				cfg.Chats[jid] = entry
			}
		}
	}

	return cfg
}

func getEntry(chatJid string, cfg SenderAllowlistConfig) ChatAllowlistEntry {
	if entry, ok := cfg.Chats[chatJid]; ok {
		return entry
	}
	return cfg.Default
}

// IsSenderAllowed checks if a sender is allowed in a given chat.
func IsSenderAllowed(chatJid string, sender string, cfg SenderAllowlistConfig) bool {
	entry := getEntry(chatJid, cfg)
	if entry.Allow.IsWildcard {
		return true
	}
	for _, s := range entry.Allow.Allowed {
		if s == sender {
			return true
		}
	}
	return false
}

// ShouldDropMessage checks if messages from a chat should be dropped.
func ShouldDropMessage(chatJid string, cfg SenderAllowlistConfig) bool {
	return getEntry(chatJid, cfg).Mode == "drop"
}

// IsTriggerAllowed checks if a trigger is allowed and handles logging logic.
func IsTriggerAllowed(chatJid string, sender string, cfg SenderAllowlistConfig) bool {
	allowed := IsSenderAllowed(chatJid, sender, cfg)
	if !allowed && cfg.LogDenied {
		// Log denied trigger
	}
	return allowed
}
