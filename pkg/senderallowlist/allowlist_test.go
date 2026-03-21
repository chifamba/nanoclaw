package senderallowlist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSenderAllowlist(t *testing.T) {
	tempDir := t.TempDir()
	
	t.Run("non-existent file returns default", func(t *testing.T) {
		cfg := LoadSenderAllowlist(filepath.Join(tempDir, "missing.json"))
		if !cfg.Default.Allow.IsWildcard || cfg.Default.Mode != "trigger" {
			t.Errorf("expected default config, got %+v", cfg.Default)
		}
		if len(cfg.Chats) != 0 {
			t.Errorf("expected no chats, got %d", len(cfg.Chats))
		}
		if !cfg.LogDenied {
			t.Errorf("expected logDenied to be true by default")
		}
	})

	t.Run("invalid JSON returns default", func(t *testing.T) {
		path := filepath.Join(tempDir, "invalid.json")
		_ = os.WriteFile(path, []byte("invalid json"), 0644)
		cfg := LoadSenderAllowlist(path)
		if !cfg.Default.Allow.IsWildcard {
			t.Errorf("expected default config on invalid JSON")
		}
	})

	t.Run("valid JSON with some invalid chat entries", func(t *testing.T) {
		path := filepath.Join(tempDir, "valid.json")
		jsonContent := `{
			"default": { "allow": ["admin"], "mode": "trigger" },
			"chats": {
				"chat1": { "allow": "*", "mode": "drop" },
				"chat2": { "allow": "not-a-list-or-star", "mode": "trigger" },
				"chat3": { "allow": "*", "mode": "invalid-mode" }
			},
			"logDenied": false
		}`
		_ = os.WriteFile(path, []byte(jsonContent), 0644)
		cfg := LoadSenderAllowlist(path)

		if cfg.Default.Allow.IsWildcard || len(cfg.Default.Allow.Allowed) != 1 || cfg.Default.Allow.Allowed[0] != "admin" {
			t.Errorf("incorrect default allow: %+v", cfg.Default.Allow)
		}
		if cfg.LogDenied {
			t.Errorf("expected logDenied to be false")
		}
		if len(cfg.Chats) != 1 {
			t.Errorf("expected 1 valid chat, got %d", len(cfg.Chats))
		}
		if entry, ok := cfg.Chats["chat1"]; !ok || !entry.Allow.IsWildcard || entry.Mode != "drop" {
			t.Errorf("incorrect chat1 entry: %+v", entry)
		}
	})
}

func TestIsSenderAllowed(t *testing.T) {
	cfg := SenderAllowlistConfig{
		Default: ChatAllowlistEntry{
			Allow: AllowValue{Allowed: []string{"admin"}},
			Mode:  "trigger",
		},
		Chats: map[string]ChatAllowlistEntry{
			"chat1": {Allow: AllowValue{IsWildcard: true}, Mode: "trigger"},
			"chat2": {Allow: AllowValue{Allowed: []string{"user1", "user2"}}, Mode: "trigger"},
		},
	}

	tests := []struct {
		chat    string
		sender  string
		allowed bool
	}{
		{"chat1", "any", true},
		{"chat2", "user1", true},
		{"chat2", "user3", false},
		{"chat3", "admin", true},
		{"chat3", "other", false},
	}

	for _, tt := range tests {
		if got := IsSenderAllowed(tt.chat, tt.sender, cfg); got != tt.allowed {
			t.Errorf("IsSenderAllowed(%q, %q) = %v; want %v", tt.chat, tt.sender, got, tt.allowed)
		}
	}
}

func TestShouldDropMessage(t *testing.T) {
	cfg := SenderAllowlistConfig{
		Default: ChatAllowlistEntry{Mode: "trigger"},
		Chats: map[string]ChatAllowlistEntry{
			"chat1": {Mode: "drop"},
		},
	}

	if !ShouldDropMessage("chat1", cfg) {
		t.Errorf("ShouldDropMessage(chat1) should be true")
	}
	if ShouldDropMessage("chat2", cfg) {
		t.Errorf("ShouldDropMessage(chat2) should be false")
	}
}
