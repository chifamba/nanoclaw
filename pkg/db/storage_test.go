package db

import (
	"os"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) Storage {
	storage, err := NewSQLiteStorage(":memory:")
	require.NoError(t, err)
	return storage
}

func TestStoreMessage(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	t.Run("stores a message and retrieves it", func(t *testing.T) {
		err := s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:00.000Z", nil, nil, nil)
		assert.NoError(t, err)

		msg := types.NewMessage{
			ID:         "msg-1",
			ChatJID:    "group@g.us",
			Sender:     "123@s.whatsapp.net",
			SenderName: "Alice",
			Content:    "hello world",
			Timestamp:  "2024-01-01T00:00:01.000Z",
		}
		err = s.StoreMessage(msg)
		assert.NoError(t, err)

		messages, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:00.000Z", "Andy", 200)
		assert.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Equal(t, "msg-1", messages[0].ID)
		assert.Equal(t, "123@s.whatsapp.net", messages[0].Sender)
		assert.Equal(t, "Alice", messages[0].SenderName)
		assert.Equal(t, "hello world", messages[0].Content)
	})

	t.Run("filters out empty content", func(t *testing.T) {
		msg := types.NewMessage{
			ID:         "msg-2",
			ChatJID:    "group@g.us",
			Sender:     "111@s.whatsapp.net",
			SenderName: "Dave",
			Content:    "",
			Timestamp:  "2024-01-01T00:00:04.000Z",
		}
		err := s.StoreMessage(msg)
		assert.NoError(t, err)

		messages, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:00.000Z", "Andy", 200)
		assert.NoError(t, err)
		// msg-2 should be filtered because content is empty
		assert.Len(t, messages, 1) // only msg-1 from previous test
	})

	t.Run("upserts on duplicate id+chat_jid", func(t *testing.T) {
		msg1 := types.NewMessage{
			ID:         "msg-dup",
			ChatJID:    "group@g.us",
			Sender:     "123@s.whatsapp.net",
			SenderName: "Alice",
			Content:    "original",
			Timestamp:  "2024-01-01T00:00:01.000Z",
		}
		err := s.StoreMessage(msg1)
		assert.NoError(t, err)

		msg2 := types.NewMessage{
			ID:         "msg-dup",
			ChatJID:    "group@g.us",
			Sender:     "123@s.whatsapp.net",
			SenderName: "Alice",
			Content:    "updated",
			Timestamp:  "2024-01-01T00:00:01.000Z",
		}
		err = s.StoreMessage(msg2)
		assert.NoError(t, err)

		messages, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:00.000Z", "Andy", 200)
		assert.NoError(t, err)
		// msg-1, msg-dup (updated)
		assert.Len(t, messages, 2)
		found := false
		for _, m := range messages {
			if m.ID == "msg-dup" {
				assert.Equal(t, "updated", m.Content)
				found = true
			}
		}
		assert.True(t, found)
	})
}

func TestGetMessagesSince(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:00.000Z", nil, nil, nil)

	s.StoreMessage(types.NewMessage{ID: "m1", ChatJID: "group@g.us", Sender: "Alice@s.whatsapp.net", SenderName: "Alice", Content: "first", Timestamp: "2024-01-01T00:00:01.000Z"})
	s.StoreMessage(types.NewMessage{ID: "m2", ChatJID: "group@g.us", Sender: "Bob@s.whatsapp.net", SenderName: "Bob", Content: "second", Timestamp: "2024-01-01T00:00:02.000Z"})
	s.StoreMessage(types.NewMessage{ID: "m3", ChatJID: "group@g.us", Sender: "Bot@s.whatsapp.net", SenderName: "Bot", Content: "bot reply", Timestamp: "2024-01-01T00:00:03.000Z", IsBotMessage: true})
	s.StoreMessage(types.NewMessage{ID: "m4", ChatJID: "group@g.us", Sender: "Carol@s.whatsapp.net", SenderName: "Carol", Content: "third", Timestamp: "2024-01-01T00:00:04.000Z"})

	t.Run("returns messages after the given timestamp", func(t *testing.T) {
		msgs, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:02.000Z", "Andy", 200)
		assert.NoError(t, err)
		assert.Len(t, msgs, 1)
		assert.Equal(t, "third", msgs[0].Content)
	})

	t.Run("excludes bot messages via is_bot_message flag", func(t *testing.T) {
		msgs, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:00.000Z", "Andy", 200)
		assert.NoError(t, err)
		for _, m := range msgs {
			assert.NotEqual(t, "bot reply", m.Content)
		}
	})

	t.Run("filters pre-migration bot messages via content prefix backstop", func(t *testing.T) {
		s.StoreMessage(types.NewMessage{ID: "m5", ChatJID: "group@g.us", Sender: "Bot@s.whatsapp.net", SenderName: "Bot", Content: "Andy: old bot reply", Timestamp: "2024-01-01T00:00:05.000Z"})
		msgs, err := s.GetMessagesSince("group@g.us", "2024-01-01T00:00:04.000Z", "Andy", 200)
		assert.NoError(t, err)
		assert.Len(t, msgs, 0)
	})
}

func TestGetNewMessages(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	s.StoreChatMetadata("group1@g.us", "2024-01-01T00:00:00.000Z", nil, nil, nil)
	s.StoreChatMetadata("group2@g.us", "2024-01-01T00:00:00.000Z", nil, nil, nil)

	s.StoreMessage(types.NewMessage{ID: "a1", ChatJID: "group1@g.us", Sender: "user@s.whatsapp.net", SenderName: "User", Content: "g1 msg1", Timestamp: "2024-01-01T00:00:01.000Z"})
	s.StoreMessage(types.NewMessage{ID: "a2", ChatJID: "group2@g.us", Sender: "user@s.whatsapp.net", SenderName: "User", Content: "g2 msg1", Timestamp: "2024-01-01T00:00:02.000Z"})
	s.StoreMessage(types.NewMessage{ID: "a3", ChatJID: "group1@g.us", Sender: "user@s.whatsapp.net", SenderName: "User", Content: "bot reply", Timestamp: "2024-01-01T00:00:03.000Z", IsBotMessage: true})
	s.StoreMessage(types.NewMessage{ID: "a4", ChatJID: "group1@g.us", Sender: "user@s.whatsapp.net", SenderName: "User", Content: "g1 msg2", Timestamp: "2024-01-01T00:00:04.000Z"})

	t.Run("returns new messages across multiple groups", func(t *testing.T) {
		messages, newTimestamp, err := s.GetNewMessages([]string{"group1@g.us", "group2@g.us"}, "2024-01-01T00:00:00.000Z", "Andy", 200)
		assert.NoError(t, err)
		assert.Len(t, messages, 3)
		assert.Equal(t, "2024-01-01T00:00:04.000Z", newTimestamp)
	})

	t.Run("filters by timestamp", func(t *testing.T) {
		messages, _, err := s.GetNewMessages([]string{"group1@g.us", "group2@g.us"}, "2024-01-01T00:00:02.000Z", "Andy", 200)
		assert.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Equal(t, "g1 msg2", messages[0].Content)
	})
}

func TestStoreChatMetadata(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	t.Run("stores chat with JID as default name", func(t *testing.T) {
		err := s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:00.000Z", nil, nil, nil)
		assert.NoError(t, err)
		chats, err := s.GetAllChats()
		assert.NoError(t, err)
		assert.Len(t, chats, 1)
		assert.Equal(t, "group@g.us", chats[0].JID)
		assert.Equal(t, "group@g.us", chats[0].Name)
	})

	t.Run("stores chat with explicit name", func(t *testing.T) {
		name := "My Group"
		err := s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:00.000Z", &name, nil, nil)
		assert.NoError(t, err)
		chats, err := s.GetAllChats()
		assert.NoError(t, err)
		assert.Equal(t, "My Group", chats[0].Name)
	})

	t.Run("preserves newer timestamp on conflict", func(t *testing.T) {
		err := s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:05.000Z", nil, nil, nil)
		assert.NoError(t, err)
		err = s.StoreChatMetadata("group@g.us", "2024-01-01T00:00:01.000Z", nil, nil, nil)
		assert.NoError(t, err)
		chats, err := s.GetAllChats()
		assert.NoError(t, err)
		assert.Equal(t, "2024-01-01T00:00:05.000Z", chats[0].LastMessageTime)
	})
}

func TestTaskCRUD(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	t.Run("creates and retrieves a task", func(t *testing.T) {
		nextRun := "2024-06-01T00:00:00.000Z"
		task := types.ScheduledTask{
			ID:            "task-1",
			GroupFolder:   "main",
			ChatJID:       "group@g.us",
			Prompt:        "do something",
			ScheduleType:  "once",
			ScheduleValue: "2024-06-01T00:00:00.000Z",
			ContextMode:   "isolated",
			NextRun:       &nextRun,
			Status:        "active",
			CreatedAt:     "2024-01-01T00:00:00.000Z",
		}
		err := s.CreateTask(task)
		assert.NoError(t, err)

		retrieved, err := s.GetTaskById("task-1")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "do something", retrieved.Prompt)
		assert.Equal(t, "active", retrieved.Status)
	})

	t.Run("updates task status", func(t *testing.T) {
		err := s.UpdateTask("task-1", map[string]interface{}{"status": "paused"})
		assert.NoError(t, err)
		retrieved, _ := s.GetTaskById("task-1")
		assert.Equal(t, "paused", retrieved.Status)
	})

	t.Run("deletes a task", func(t *testing.T) {
		err := s.DeleteTask("task-1")
		assert.NoError(t, err)
		retrieved, _ := s.GetTaskById("task-1")
		assert.Nil(t, retrieved)
	})
}

func TestRegisteredGroupIsMain(t *testing.T) {
	s := setupTestDB(t)
	defer s.Close()

	t.Run("persists isMain=true through set/get round-trip", func(t *testing.T) {
		group := types.RegisteredGroup{
			Name:    "Main Chat",
			Folder:  "main",
			Trigger: "@Andy",
			AddedAt: "2024-01-01T00:00:00.000Z",
			IsMain:  true,
		}
		err := s.SetRegisteredGroup("main@s.whatsapp.net", group)
		assert.NoError(t, err)

		groups, err := s.GetAllRegisteredGroups()
		assert.NoError(t, err)
		g, ok := groups["main@s.whatsapp.net"]
		assert.True(t, ok)
		assert.True(t, g.IsMain)
		assert.Equal(t, "main", g.Folder)
	})
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
