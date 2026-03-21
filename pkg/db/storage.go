package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/types"
	_ "github.com/mattn/go-sqlite3"
)

type Storage interface {
	StoreChatMetadata(chatJID string, timestamp string, name *string, channel *string, isGroup *bool) error
	UpdateChatName(chatJID string, name string) error
	GetAllChats() ([]types.ChatInfo, error)
	GetLastGroupSync() (*string, error)
	SetLastGroupSync() error
	StoreMessage(msg types.NewMessage) error
	StoreMessageDirect(msg types.NewMessage) error
	GetNewMessages(jids []string, lastTimestamp string, botPrefix string, limit int) ([]types.NewMessage, string, error)
	GetMessagesSince(chatJid string, sinceTimestamp string, botPrefix string, limit int) ([]types.NewMessage, error)
	CreateTask(task types.ScheduledTask) error
	GetTaskById(id string) (*types.ScheduledTask, error)
	GetTasksForGroup(groupFolder string) ([]types.ScheduledTask, error)
	GetAllTasks() ([]types.ScheduledTask, error)
	UpdateTask(id string, updates map[string]interface{}) error
	DeleteTask(id string) error
	GetDueTasks() ([]types.ScheduledTask, error)
	UpdateTaskAfterRun(id string, nextRun *string, lastResult string) error
	LogTaskRun(log types.TaskRunLog) error
	GetRouterState(key string) (*string, error)
	SetRouterState(key string, value string) error
	GetSession(groupFolder string) (*string, error)
	SetSession(groupFolder string, sessionID string) error
	GetAllSessions() (map[string]string, error)
	GetRegisteredGroup(jid string) (*types.RegisteredGroup, error)
	SetRegisteredGroup(jid string, group types.RegisteredGroup) error
	GetAllRegisteredGroups() (map[string]types.RegisteredGroup, error)
	Close() error
}

type SQLiteStorage struct {
	db *sql.DB
}

func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStorage{db: db}, nil
}

func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

func (s *SQLiteStorage) StoreChatMetadata(chatJID string, timestamp string, name *string, channel *string, isGroup *bool) error {
	var group interface{}
	if isGroup != nil {
		if *isGroup {
			group = 1
		} else {
			group = 0
		}
	}

	if name != nil {
		_, err := s.db.Exec(`
			INSERT INTO chats (jid, name, last_message_time, channel, is_group) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(jid) DO UPDATE SET
				name = excluded.name,
				last_message_time = MAX(last_message_time, excluded.last_message_time),
				channel = COALESCE(excluded.channel, channel),
				is_group = COALESCE(excluded.is_group, is_group)
		`, chatJID, *name, timestamp, channel, group)
		return err
	} else {
		_, err := s.db.Exec(`
			INSERT INTO chats (jid, name, last_message_time, channel, is_group) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(jid) DO UPDATE SET
				last_message_time = MAX(last_message_time, excluded.last_message_time),
				channel = COALESCE(excluded.channel, channel),
				is_group = COALESCE(excluded.is_group, is_group)
		`, chatJID, chatJID, timestamp, channel, group)
		return err
	}
}

func (s *SQLiteStorage) UpdateChatName(chatJID string, name string) error {
	now := time.Now().Format(time.RFC3339) // Use RFC3339 for ISO 8601
	_, err := s.db.Exec(`
		INSERT INTO chats (jid, name, last_message_time) VALUES (?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET name = excluded.name
	`, chatJID, name, now)
	return err
}

func (s *SQLiteStorage) GetAllChats() ([]types.ChatInfo, error) {
	rows, err := s.db.Query(`
		SELECT jid, IFNULL(name, ''), last_message_time, IFNULL(channel, ''), IFNULL(is_group, 0)
		FROM chats
		ORDER BY last_message_time DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []types.ChatInfo
	for rows.Next() {
		var c types.ChatInfo
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessageTime, &c.Channel, &c.IsGroup); err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	return chats, nil
}

func (s *SQLiteStorage) GetLastGroupSync() (*string, error) {
	var lastSync string
	err := s.db.QueryRow(`SELECT last_message_time FROM chats WHERE jid = '__group_sync__'`).Scan(&lastSync)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &lastSync, nil
}

func (s *SQLiteStorage) SetLastGroupSync() error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO chats (jid, name, last_message_time) VALUES ('__group_sync__', '__group_sync__', ?)`, now)
	return err
}

func (s *SQLiteStorage) StoreMessage(msg types.NewMessage) error {
	isFromMe := 0
	if msg.IsFromMe {
		isFromMe = 1
	}
	isBotMessage := 0
	if msg.IsBotMessage {
		isBotMessage = 1
	}
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO messages (id, chat_jid, sender, sender_name, content, timestamp, is_from_me, is_bot_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, msg.ID, msg.ChatJID, msg.Sender, msg.SenderName, msg.Content, msg.Timestamp, isFromMe, isBotMessage)
	return err
}

func (s *SQLiteStorage) StoreMessageDirect(msg types.NewMessage) error {
	return s.StoreMessage(msg)
}

func (s *SQLiteStorage) GetNewMessages(jids []string, lastTimestamp string, botPrefix string, limit int) ([]types.NewMessage, string, error) {
	if len(jids) == 0 {
		return nil, lastTimestamp, nil
	}

	placeholders := make([]string, len(jids))
	args := make([]interface{}, 0, len(jids)+3)
	args = append(args, lastTimestamp)
	for i, jid := range jids {
		placeholders[i] = "?"
		args = append(args, jid)
	}
	args = append(args, botPrefix+":%", limit)

	sql := fmt.Sprintf(`
		SELECT * FROM (
			SELECT id, chat_jid, sender, sender_name, content, timestamp, is_from_me
			FROM messages
			WHERE timestamp > ? AND chat_jid IN (%s)
				AND is_bot_message = 0 AND content NOT LIKE ?
				AND content != '' AND content IS NOT NULL
			ORDER BY timestamp DESC
			LIMIT ?
		) ORDER BY timestamp
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(sql, args...)
	if err != nil {
		return nil, lastTimestamp, err
	}
	defer rows.Close()

	var messages []types.NewMessage
	newTimestamp := lastTimestamp
	for rows.Next() {
		var m types.NewMessage
		var isFromMe int
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.Sender, &m.SenderName, &m.Content, &m.Timestamp, &isFromMe); err != nil {
			return nil, lastTimestamp, err
		}
		m.IsFromMe = isFromMe == 1
		if m.Timestamp > newTimestamp {
			newTimestamp = m.Timestamp
		}
		messages = append(messages, m)
	}

	return messages, newTimestamp, nil
}

func (s *SQLiteStorage) GetMessagesSince(chatJid string, sinceTimestamp string, botPrefix string, limit int) ([]types.NewMessage, error) {
	sql := `
		SELECT * FROM (
			SELECT id, chat_jid, sender, sender_name, content, timestamp, is_from_me
			FROM messages
			WHERE chat_jid = ? AND timestamp > ?
				AND is_bot_message = 0 AND content NOT LIKE ?
				AND content != '' AND content IS NOT NULL
			ORDER BY timestamp DESC
			LIMIT ?
		) ORDER BY timestamp
	`
	rows, err := s.db.Query(sql, chatJid, sinceTimestamp, botPrefix+":%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []types.NewMessage
	for rows.Next() {
		var m types.NewMessage
		var isFromMe int
		if err := rows.Scan(&m.ID, &m.ChatJID, &m.Sender, &m.SenderName, &m.Content, &m.Timestamp, &isFromMe); err != nil {
			return nil, err
		}
		m.IsFromMe = isFromMe == 1
		messages = append(messages, m)
	}
	return messages, nil
}

func (s *SQLiteStorage) CreateTask(task types.ScheduledTask) error {
	_, err := s.db.Exec(`
		INSERT INTO scheduled_tasks (id, group_folder, chat_jid, prompt, schedule_type, schedule_value, context_mode, next_run, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.GroupFolder, task.ChatJID, task.Prompt, task.ScheduleType, task.ScheduleValue, task.ContextMode, task.NextRun, task.Status, task.CreatedAt)
	return err
}

func (s *SQLiteStorage) GetTaskById(id string) (*types.ScheduledTask, error) {
	var t types.ScheduledTask
	err := s.db.QueryRow("SELECT * FROM scheduled_tasks WHERE id = ?", id).Scan(
		&t.ID, &t.GroupFolder, &t.ChatJID, &t.Prompt, &t.ScheduleType, &t.ScheduleValue, &t.NextRun, &t.LastRun, &t.LastResult, &t.Status, &t.CreatedAt, &t.ContextMode,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteStorage) GetTasksForGroup(groupFolder string) ([]types.ScheduledTask, error) {
	rows, err := s.db.Query("SELECT * FROM scheduled_tasks WHERE group_folder = ? ORDER BY created_at DESC", groupFolder)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []types.ScheduledTask
	for rows.Next() {
		var t types.ScheduledTask
		if err := rows.Scan(&t.ID, &t.GroupFolder, &t.ChatJID, &t.Prompt, &t.ScheduleType, &t.ScheduleValue, &t.NextRun, &t.LastRun, &t.LastResult, &t.Status, &t.CreatedAt, &t.ContextMode); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *SQLiteStorage) GetAllTasks() ([]types.ScheduledTask, error) {
	rows, err := s.db.Query("SELECT * FROM scheduled_tasks ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []types.ScheduledTask
	for rows.Next() {
		var t types.ScheduledTask
		if err := rows.Scan(&t.ID, &t.GroupFolder, &t.ChatJID, &t.Prompt, &t.ScheduleType, &t.ScheduleValue, &t.NextRun, &t.LastRun, &t.LastResult, &t.Status, &t.CreatedAt, &t.ContextMode); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *SQLiteStorage) UpdateTask(id string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	fields := make([]string, 0, len(updates))
	args := make([]interface{}, 0, len(updates)+1)

	for k, v := range updates {
		fields = append(fields, fmt.Sprintf("%s = ?", k))
		args = append(args, v)
	}
	args = append(args, id)

	sql := fmt.Sprintf("UPDATE scheduled_tasks SET %s WHERE id = ?", strings.Join(fields, ", "))
	_, err := s.db.Exec(sql, args...)
	return err
}

func (s *SQLiteStorage) DeleteTask(id string) error {
	// Delete child records first (FK constraint)
	_, err := s.db.Exec("DELETE FROM task_run_logs WHERE task_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM scheduled_tasks WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) GetDueTasks() ([]types.ScheduledTask, error) {
	now := time.Now().Format(time.RFC3339)
	rows, err := s.db.Query(`
		SELECT * FROM scheduled_tasks
		WHERE status = 'active' AND next_run IS NOT NULL AND next_run <= ?
		ORDER BY next_run
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []types.ScheduledTask
	for rows.Next() {
		var t types.ScheduledTask
		if err := rows.Scan(&t.ID, &t.GroupFolder, &t.ChatJID, &t.Prompt, &t.ScheduleType, &t.ScheduleValue, &t.NextRun, &t.LastRun, &t.LastResult, &t.Status, &t.CreatedAt, &t.ContextMode); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *SQLiteStorage) UpdateTaskAfterRun(id string, nextRun *string, lastResult string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE scheduled_tasks
		SET next_run = ?, last_run = ?, last_result = ?, status = CASE WHEN ? IS NULL THEN 'completed' ELSE status END
		WHERE id = ?
	`, nextRun, now, lastResult, nextRun, id)
	return err
}

func (s *SQLiteStorage) LogTaskRun(log types.TaskRunLog) error {
	_, err := s.db.Exec(`
		INSERT INTO task_run_logs (task_id, run_at, duration_ms, status, result, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, log.TaskID, log.RunAt, log.DurationMS, log.Status, log.Result, log.Error)
	return err
}

func (s *SQLiteStorage) GetRouterState(key string) (*string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM router_state WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *SQLiteStorage) SetRouterState(key string, value string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO router_state (key, value) VALUES (?, ?)", key, value)
	return err
}

func (s *SQLiteStorage) GetSession(groupFolder string) (*string, error) {
	var sessionID string
	err := s.db.QueryRow("SELECT session_id FROM sessions WHERE group_folder = ?", groupFolder).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sessionID, nil
}

func (s *SQLiteStorage) SetSession(groupFolder string, sessionID string) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO sessions (group_folder, session_id) VALUES (?, ?)", groupFolder, sessionID)
	return err
}

func (s *SQLiteStorage) GetAllSessions() (map[string]string, error) {
	rows, err := s.db.Query("SELECT group_folder, session_id FROM sessions")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make(map[string]string)
	for rows.Next() {
		var folder, sessionID string
		if err := rows.Scan(&folder, &sessionID); err != nil {
			return nil, err
		}
		sessions[folder] = sessionID
	}
	return sessions, nil
}

func (s *SQLiteStorage) GetRegisteredGroup(jid string) (*types.RegisteredGroup, error) {
	var row struct {
		jid             string
		name            string
		folder          string
		trigger_pattern string
		added_at        string
		container_config sql.NullString
		requires_trigger sql.NullInt64
		is_main          sql.NullInt64
	}

	err := s.db.QueryRow("SELECT * FROM registered_groups WHERE jid = ?", jid).Scan(
		&row.jid, &row.name, &row.folder, &row.trigger_pattern, &row.added_at, &row.container_config, &row.requires_trigger, &row.is_main,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if !IsValidGroupFolder(row.folder) {
		return nil, nil // Or return error? TypeScript logger.warns and returns undefined
	}

	group := &types.RegisteredGroup{
		Name:    row.name,
		Folder:  row.folder,
		Trigger: row.trigger_pattern,
		AddedAt: row.added_at,
		IsMain:  row.is_main.Int64 == 1,
	}

	if row.container_config.Valid {
		var config types.ContainerConfig
		if err := json.Unmarshal([]byte(row.container_config.String), &config); err == nil {
			group.ContainerConfig = &config
		}
	}

	if row.requires_trigger.Valid {
		val := row.requires_trigger.Int64 == 1
		group.RequiresTrigger = &val
	}

	return group, nil
}

func (s *SQLiteStorage) SetRegisteredGroup(jid string, group types.RegisteredGroup) error {
	if !IsValidGroupFolder(group.Folder) {
		return fmt.Errorf("invalid group folder: %s", group.Folder)
	}

	var containerConfig *string
	if group.ContainerConfig != nil {
		data, err := json.Marshal(group.ContainerConfig)
		if err != nil {
			return err
		}
		s := string(data)
		containerConfig = &s
	}

	requiresTrigger := 1
	if group.RequiresTrigger != nil && !*group.RequiresTrigger {
		requiresTrigger = 0
	}

	isMain := 0
	if group.IsMain {
		isMain = 1
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO registered_groups (jid, name, folder, trigger_pattern, added_at, container_config, requires_trigger, is_main)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, jid, group.Name, group.Folder, group.Trigger, group.AddedAt, containerConfig, requiresTrigger, isMain)
	return err
}

func (s *SQLiteStorage) GetAllRegisteredGroups() (map[string]types.RegisteredGroup, error) {
	rows, err := s.db.Query("SELECT * FROM registered_groups")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make(map[string]types.RegisteredGroup)
	for rows.Next() {
		var row struct {
			jid             string
			name            string
			folder          string
			trigger_pattern string
			added_at        string
			container_config sql.NullString
			requires_trigger sql.NullInt64
			is_main          sql.NullInt64
		}
		if err := rows.Scan(&row.jid, &row.name, &row.folder, &row.trigger_pattern, &row.added_at, &row.container_config, &row.requires_trigger, &row.is_main); err != nil {
			return nil, err
		}

		if !IsValidGroupFolder(row.folder) {
			continue
		}

		group := types.RegisteredGroup{
			Name:    row.name,
			Folder:  row.folder,
			Trigger: row.trigger_pattern,
			AddedAt: row.added_at,
			IsMain:  row.is_main.Int64 == 1,
		}

		if row.container_config.Valid {
			var config types.ContainerConfig
			if err := json.Unmarshal([]byte(row.container_config.String), &config); err == nil {
				group.ContainerConfig = &config
			}
		}

		if row.requires_trigger.Valid {
			val := row.requires_trigger.Int64 == 1
			group.RequiresTrigger = &val
		}

		groups[row.jid] = group
	}
	return groups, nil
}
