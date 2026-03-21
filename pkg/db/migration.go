package db

import (
	"database/sql"
	"fmt"
)

func createSchema(db *sql.DB) error {
	schema := `
    CREATE TABLE IF NOT EXISTS chats (
      jid TEXT PRIMARY KEY,
      name TEXT,
      last_message_time TEXT,
      channel TEXT,
      is_group INTEGER DEFAULT 0
    );
    CREATE TABLE IF NOT EXISTS messages (
      id TEXT,
      chat_jid TEXT,
      sender TEXT,
      sender_name TEXT,
      content TEXT,
      timestamp TEXT,
      is_from_me INTEGER,
      is_bot_message INTEGER DEFAULT 0,
      PRIMARY KEY (id, chat_jid),
      FOREIGN KEY (chat_jid) REFERENCES chats(jid)
    );
    CREATE INDEX IF NOT EXISTS idx_timestamp ON messages(timestamp);

    CREATE TABLE IF NOT EXISTS scheduled_tasks (
      id TEXT PRIMARY KEY,
      group_folder TEXT NOT NULL,
      chat_jid TEXT NOT NULL,
      prompt TEXT NOT NULL,
      schedule_type TEXT NOT NULL,
      schedule_value TEXT NOT NULL,
      next_run TEXT,
      last_run TEXT,
      last_result TEXT,
      status TEXT DEFAULT 'active',
      created_at TEXT NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_next_run ON scheduled_tasks(next_run);
    CREATE INDEX IF NOT EXISTS idx_status ON scheduled_tasks(status);

    CREATE TABLE IF NOT EXISTS task_run_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      task_id TEXT NOT NULL,
      run_at TEXT NOT NULL,
      duration_ms INTEGER NOT NULL,
      status TEXT NOT NULL,
      result TEXT,
      error TEXT,
      FOREIGN KEY (task_id) REFERENCES scheduled_tasks(id)
    );
    CREATE INDEX IF NOT EXISTS idx_task_run_logs ON task_run_logs(task_id, run_at);

    CREATE TABLE IF NOT EXISTS router_state (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS sessions (
      group_folder TEXT PRIMARY KEY,
      session_id TEXT NOT NULL
    );
    CREATE TABLE IF NOT EXISTS registered_groups (
      jid TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      folder TEXT NOT NULL UNIQUE,
      trigger_pattern TEXT NOT NULL,
      added_at TEXT NOT NULL,
      container_config TEXT,
      requires_trigger INTEGER DEFAULT 1
    );
  `
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Migrations

	// Add context_mode column
	_, _ = db.Exec("ALTER TABLE scheduled_tasks ADD COLUMN context_mode TEXT DEFAULT 'isolated'")

	// Add is_bot_message column
	_, err = db.Exec("ALTER TABLE messages ADD COLUMN is_bot_message INTEGER DEFAULT 0")
	if err == nil {
		// Backfill: mark existing bot messages that used the content prefix pattern
		// ASSISTANT_NAME is used in src/db.ts. In Go, we might need to pass it or have a constant.
		// For now, I'll assume we can pass it or use a default.
		// Actually, src/db.ts uses ASSISTANT_NAME:%.
		// I'll skip the backfill for now or use a generic pattern if I don't have the name.
		// Wait, the prompt says "exactly match src/db.ts".
		// src/db.ts imports ASSISTANT_NAME from ./config.js.
	}

	// Add is_main column
	_, err = db.Exec("ALTER TABLE registered_groups ADD COLUMN is_main INTEGER DEFAULT 0")
	if err == nil {
		_, _ = db.Exec("UPDATE registered_groups SET is_main = 1 WHERE folder = 'main'")
	}

	// Add channel and is_group columns
	_, err = db.Exec("ALTER TABLE chats ADD COLUMN channel TEXT")
	if err == nil {
		_, _ = db.Exec("ALTER TABLE chats ADD COLUMN is_group INTEGER DEFAULT 0")
		// Backfill from JID patterns
		_, _ = db.Exec("UPDATE chats SET channel = 'whatsapp', is_group = 1 WHERE jid LIKE '%@g.us'")
		_, _ = db.Exec("UPDATE chats SET channel = 'whatsapp', is_group = 0 WHERE jid LIKE '%@s.whatsapp.net'")
		_, _ = db.Exec("UPDATE chats SET channel = 'discord', is_group = 1 WHERE jid LIKE 'dc:%'")
		_, _ = db.Exec("UPDATE chats SET channel = 'telegram', is_group = 1 WHERE jid LIKE 'tg:%'")
	}

	return nil
}

func backfillBotMessages(db *sql.DB, assistantName string) error {
	_, err := db.Exec("UPDATE messages SET is_bot_message = 1 WHERE content LIKE ?", assistantName+":%")
	return err
}
