package ipc

import "encoding/json"

// MessageEvent represents a message to be sent via the IPC.
type MessageEvent struct {
	Type     string `json:"type"`
	ChatJID  string `json:"chatJid"`
	Text     string `json:"text"`
}

// TaskEvent represents a task-related event via the IPC.
type TaskEvent struct {
	Type            string          `json:"type"`
	TaskID          string          `json:"taskId,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	ScheduleType    string          `json:"schedule_type,omitempty"`
	ScheduleValue   string          `json:"schedule_value,omitempty"`
	ContextMode     string          `json:"context_mode,omitempty"`
	GroupFolder     string          `json:"groupFolder,omitempty"`
	ChatJID         string          `json:"chatJid,omitempty"`
	TargetJID       string          `json:"targetJid,omitempty"`
	JID             string          `json:"jid,omitempty"`
	Name            string          `json:"name,omitempty"`
	Folder          string          `json:"folder,omitempty"`
	Trigger         string          `json:"trigger,omitempty"`
	RequiresTrigger *bool           `json:"requiresTrigger,omitempty"`
	ContainerConfig json.RawMessage `json:"containerConfig,omitempty"`
}

// AvailableGroup represents a group that can be registered.
type AvailableGroup struct {
	JID          string `json:"jid"`
	Name         string `json:"name"`
	LastActivity string `json:"lastActivity"`
	IsRegistered bool   `json:"isRegistered"`
}

// IPCData holds shared dependencies and state for the IPC watcher.
type IPCData struct {
	Groups   []AvailableGroup `json:"groups"`
	LastSync string           `json:"lastSync"`
}

