package container

import (
	"os/exec"
)

// ContainerInput for NanoClaw
type ContainerInput struct {
	Prompt          string `json:"prompt"`
	SessionID       string `json:"sessionId,omitempty"`
	GroupFolder     string `json:"groupFolder"`
	ChatJid         string `json:"chatJid"`
	IsMain          bool   `json:"isMain"`
	IsScheduledTask bool   `json:"isScheduledTask,omitempty"`
	AssistantName   string `json:"assistantName,omitempty"`
}

// ContainerOutput from NanoClaw
type ContainerOutput struct {
	Status       string `json:"status"` // 'success' | 'error'
	Result       string `json:"result,omitempty"`
	NewSessionID string `json:"newSessionId,omitempty"`
	Error        string `json:"error,omitempty"`
}

// VolumeMount for container execution
type VolumeMount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readonly"`
}

// ProcessInfo holds information about a running container process
type ProcessInfo struct {
	Cmd           *exec.Cmd
	ContainerName string
}
