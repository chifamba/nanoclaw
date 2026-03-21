package senderallowlist

import (
	"encoding/json"
	"fmt"
)

// AllowValue represents either "*" or a list of allowed senders.
type AllowValue struct {
	IsWildcard bool
	Allowed    []string
}

func (a *AllowValue) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "*" {
			a.IsWildcard = true
			a.Allowed = nil
			return nil
		}
		return fmt.Errorf("invalid string value for allow: %s", s)
	}

	var l []string
	if err := json.Unmarshal(data, &l); err == nil {
		a.IsWildcard = false
		a.Allowed = l
		return nil
	}

	return fmt.Errorf("allow must be '*' or an array of strings")
}

func (a AllowValue) MarshalJSON() ([]byte, error) {
	if a.IsWildcard {
		return json.Marshal("*")
	}
	return json.Marshal(a.Allowed)
}

// ChatAllowlistEntry defines the allowlist policy for a specific chat or the default.
type ChatAllowlistEntry struct {
	Allow AllowValue `json:"allow"`
	Mode  string     `json:"mode"` // "trigger" or "drop"
}

// SenderAllowlistConfig holds the overall allowlist configuration.
type SenderAllowlistConfig struct {
	Default   ChatAllowlistEntry            `json:"default"`
	Chats     map[string]ChatAllowlistEntry `json:"chats"`
	LogDenied bool                          `json:"logDenied"`
}
