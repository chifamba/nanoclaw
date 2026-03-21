package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/redis/go-redis/v9"
)

type CachedStorage struct {
	base   Storage
	client *redis.Client
}

func NewCachedStorage(base Storage, redisURL string) (*CachedStorage, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &CachedStorage{
		base:   base,
		client: client,
	}, nil
}

func (c *CachedStorage) Close() error {
	err1 := c.client.Close()
	err2 := c.base.Close()
	if err1 != nil {
		return err1
	}
	return err2
}

func (c *CachedStorage) StoreChatMetadata(chatJID string, timestamp string, name *string, channel *string, isGroup *bool) error {
	return c.base.StoreChatMetadata(chatJID, timestamp, name, channel, isGroup)
}

func (c *CachedStorage) UpdateChatName(chatJID string, name string) error {
	return c.base.UpdateChatName(chatJID, name)
}

func (c *CachedStorage) GetAllChats() ([]types.ChatInfo, error) {
	return c.base.GetAllChats()
}

func (c *CachedStorage) GetLastGroupSync() (*string, error) {
	return c.base.GetLastGroupSync()
}

func (c *CachedStorage) SetLastGroupSync() error {
	return c.base.SetLastGroupSync()
}

func (c *CachedStorage) StoreMessage(msg types.NewMessage) error {
	return c.base.StoreMessage(msg)
}

func (c *CachedStorage) StoreMessageDirect(msg types.NewMessage) error {
	return c.base.StoreMessageDirect(msg)
}

func (c *CachedStorage) GetNewMessages(jids []string, lastTimestamp string, botPrefix string, limit int) ([]types.NewMessage, string, error) {
	return c.base.GetNewMessages(jids, lastTimestamp, botPrefix, limit)
}

func (c *CachedStorage) GetMessagesSince(chatJid string, sinceTimestamp string, botPrefix string, limit int) ([]types.NewMessage, error) {
	return c.base.GetMessagesSince(chatJid, sinceTimestamp, botPrefix, limit)
}

func (c *CachedStorage) CreateTask(task types.ScheduledTask) error {
	return c.base.CreateTask(task)
}

func (c *CachedStorage) GetTaskById(id string) (*types.ScheduledTask, error) {
	return c.base.GetTaskById(id)
}

func (c *CachedStorage) GetTasksForGroup(groupFolder string) ([]types.ScheduledTask, error) {
	return c.base.GetTasksForGroup(groupFolder)
}

func (c *CachedStorage) GetAllTasks() ([]types.ScheduledTask, error) {
	return c.base.GetAllTasks()
}

func (c *CachedStorage) UpdateTask(id string, updates map[string]interface{}) error {
	return c.base.UpdateTask(id, updates)
}

func (c *CachedStorage) DeleteTask(id string) error {
	return c.base.DeleteTask(id)
}

func (c *CachedStorage) GetDueTasks() ([]types.ScheduledTask, error) {
	return c.base.GetDueTasks()
}

func (c *CachedStorage) UpdateTaskAfterRun(id string, nextRun *string, lastResult string) error {
	return c.base.UpdateTaskAfterRun(id, nextRun, lastResult)
}

func (c *CachedStorage) LogTaskRun(log types.TaskRunLog) error {
	return c.base.LogTaskRun(log)
}

func (c *CachedStorage) GetRouterState(key string) (*string, error) {
	ctx := context.Background()
	redisKey := "router_state:" + key
	val, err := c.client.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		// Cache miss
		res, err := c.base.GetRouterState(key)
		if err != nil {
			return nil, err
		}
		if res != nil {
			c.client.Set(ctx, redisKey, *res, 1*time.Hour)
		}
		return res, nil
	} else if err != nil {
		return c.base.GetRouterState(key)
	}
	return &val, nil
}

func (c *CachedStorage) SetRouterState(key string, value string) error {
	ctx := context.Background()
	redisKey := "router_state:" + key
	if err := c.base.SetRouterState(key, value); err != nil {
		return err
	}
	// Update cache
	c.client.Set(ctx, redisKey, value, 1*time.Hour)
	return nil
}

func (c *CachedStorage) GetSession(groupFolder string) (*string, error) {
	ctx := context.Background()
	redisKey := "session:" + groupFolder
	val, err := c.client.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		res, err := c.base.GetSession(groupFolder)
		if err != nil {
			return nil, err
		}
		if res != nil {
			c.client.Set(ctx, redisKey, *res, 24*time.Hour)
		}
		return res, nil
	} else if err != nil {
		return c.base.GetSession(groupFolder)
	}
	return &val, nil
}

func (c *CachedStorage) SetSession(groupFolder string, sessionID string) error {
	ctx := context.Background()
	redisKey := "session:" + groupFolder
	if err := c.base.SetSession(groupFolder, sessionID); err != nil {
		return err
	}
	c.client.Set(ctx, redisKey, sessionID, 24*time.Hour)
	return nil
}

func (c *CachedStorage) GetAllSessions() (map[string]string, error) {
	// Let's not cache this as it returns all and we might miss updates easily if we cache the whole map.
	// Can just pass through.
	return c.base.GetAllSessions()
}

func (c *CachedStorage) GetRegisteredGroup(jid string) (*types.RegisteredGroup, error) {
	ctx := context.Background()
	redisKey := "group:" + jid
	val, err := c.client.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		res, err := c.base.GetRegisteredGroup(jid)
		if err != nil {
			return nil, err
		}
		if res != nil {
			data, _ := json.Marshal(res)
			c.client.Set(ctx, redisKey, data, 24*time.Hour)
		}
		return res, nil
	} else if err != nil {
		return c.base.GetRegisteredGroup(jid)
	}

	var group types.RegisteredGroup
	if err := json.Unmarshal([]byte(val), &group); err != nil {
		return c.base.GetRegisteredGroup(jid)
	}
	return &group, nil
}

func (c *CachedStorage) SetRegisteredGroup(jid string, group types.RegisteredGroup) error {
	ctx := context.Background()
	redisKey := "group:" + jid
	if err := c.base.SetRegisteredGroup(jid, group); err != nil {
		return err
	}

	// Cache the new value
	data, _ := json.Marshal(group)
	c.client.Set(ctx, redisKey, data, 24*time.Hour)

	// Invalidate the "all groups" cache
	c.client.Del(ctx, "all_groups")

	return nil
}

func (c *CachedStorage) GetAllRegisteredGroups() (map[string]types.RegisteredGroup, error) {
	ctx := context.Background()
	redisKey := "all_groups"
	val, err := c.client.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		groups, err := c.base.GetAllRegisteredGroups()
		if err != nil {
			return nil, err
		}
		data, _ := json.Marshal(groups)
		c.client.Set(ctx, redisKey, data, 24*time.Hour)
		return groups, nil
	} else if err != nil {
		return c.base.GetAllRegisteredGroups()
	}

	var groups map[string]types.RegisteredGroup
	if err := json.Unmarshal([]byte(val), &groups); err != nil {
		return c.base.GetAllRegisteredGroups()
	}
	return groups, nil
}
