package types

type NewMessage struct {
	ID           string `json:"id"`
	ChatJID      string `json:"chat_jid"`
	Sender       string `json:"sender"`
	SenderName   string `json:"sender_name"`
	Content      string `json:"content"`
	Timestamp    string `json:"timestamp"`
	IsFromMe     bool   `json:"is_from_me,omitempty"`
	IsBotMessage bool   `json:"is_bot_message,omitempty"`
}

type AdditionalMount struct {
	HostPath      string `json:"hostPath"`
	ContainerPath string `json:"containerPath,omitempty"`
	ReadOnly      bool   `json:"readonly,omitempty"`
}

type ContainerConfig struct {
	AdditionalMounts []AdditionalMount `json:"additionalMounts,omitempty"`
	Timeout          int               `json:"timeout,omitempty"`
}

type RegisteredGroup struct {
	Name            string           `json:"name"`
	Folder          string           `json:"folder"`
	Trigger         string           `json:"trigger"`
	AddedAt         string           `json:"added_at"`
	ContainerConfig *ContainerConfig `json:"containerConfig,omitempty"`
	RequiresTrigger *bool            `json:"requiresTrigger,omitempty"`
	IsMain          bool             `json:"isMain,omitempty"`
}

type ScheduledTask struct {
	ID            string  `json:"id"`
	GroupFolder   string  `json:"group_folder"`
	ChatJID       string  `json:"chat_jid"`
	Prompt        string  `json:"prompt"`
	ScheduleType  string  `json:"schedule_type"` // 'cron' | 'interval' | 'once'
	ScheduleValue string  `json:"schedule_value"`
	ContextMode   string  `json:"context_mode"` // 'group' | 'isolated'
	NextRun       *string `json:"next_run"`
	LastRun       *string `json:"last_run"`
	LastResult    *string `json:"last_result"`
	Status        string  `json:"status"` // 'active' | 'paused' | 'completed'
	CreatedAt     string  `json:"created_at"`
}

type TaskRunLog struct {
	ID         int64   `json:"id,omitempty"`
	TaskID     string  `json:"task_id"`
	RunAt      string  `json:"run_at"`
	DurationMS int     `json:"duration_ms"`
	Status     string  `json:"status"` // 'success' | 'error'
	Result     *string `json:"result"`
	Error      *string `json:"error"`
}

type ChatInfo struct {
	JID             string `json:"jid"`
	Name            string `json:"name"`
	LastMessageTime string `json:"last_message_time"`
	Channel         string `json:"channel"`
	IsGroup         int    `json:"is_group"`
}
