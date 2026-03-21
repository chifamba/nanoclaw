package messagepoller

import (
	"context"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/ipc"
	"github.com/nanoclaw/nanoclaw/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) StoreChatMetadata(chatJID string, timestamp string, name *string, channel *string, isGroup *bool) error {
	args := m.Called(chatJID, timestamp, name, channel, isGroup)
	return args.Error(0)
}

func (m *MockStorage) UpdateChatName(chatJID string, name string) error {
	args := m.Called(chatJID, name)
	return args.Error(0)
}

func (m *MockStorage) GetAllChats() ([]types.ChatInfo, error) {
	args := m.Called()
	return args.Get(0).([]types.ChatInfo), args.Error(1)
}

func (m *MockStorage) GetLastGroupSync() (*string, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	s := args.Get(0).(string)
	return &s, args.Error(1)
}

func (m *MockStorage) SetLastGroupSync() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStorage) StoreMessage(msg types.NewMessage) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockStorage) StoreMessageDirect(msg types.NewMessage) error {
	args := m.Called(msg)
	return args.Error(0)
}

func (m *MockStorage) GetNewMessages(jids []string, lastTimestamp string, botPrefix string, limit int) ([]types.NewMessage, string, error) {
	args := m.Called(jids, lastTimestamp, botPrefix, limit)
	return args.Get(0).([]types.NewMessage), args.String(1), args.Error(2)
}

func (m *MockStorage) GetMessagesSince(chatJid string, sinceTimestamp string, botPrefix string, limit int) ([]types.NewMessage, error) {
	args := m.Called(chatJid, sinceTimestamp, botPrefix, limit)
	return args.Get(0).([]types.NewMessage), args.Error(1)
}

func (m *MockStorage) CreateTask(task types.ScheduledTask) error {
	args := m.Called(task)
	return args.Error(0)
}

func (m *MockStorage) GetTaskById(id string) (*types.ScheduledTask, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.ScheduledTask), args.Error(1)
}

func (m *MockStorage) GetTasksForGroup(groupFolder string) ([]types.ScheduledTask, error) {
	args := m.Called(groupFolder)
	return args.Get(0).([]types.ScheduledTask), args.Error(1)
}

func (m *MockStorage) GetAllTasks() ([]types.ScheduledTask, error) {
	args := m.Called()
	return args.Get(0).([]types.ScheduledTask), args.Error(1)
}

func (m *MockStorage) UpdateTask(id string, updates map[string]interface{}) error {
	args := m.Called(id, updates)
	return args.Error(0)
}

func (m *MockStorage) DeleteTask(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockStorage) GetDueTasks() ([]types.ScheduledTask, error) {
	args := m.Called()
	return args.Get(0).([]types.ScheduledTask), args.Error(1)
}

func (m *MockStorage) UpdateTaskAfterRun(id string, nextRun *string, lastResult string) error {
	args := m.Called(id, nextRun, lastResult)
	return args.Error(0)
}

func (m *MockStorage) LogTaskRun(log types.TaskRunLog) error {
	args := m.Called(log)
	return args.Error(0)
}

func (m *MockStorage) GetRouterState(key string) (*string, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	s := args.Get(0).(string)
	return &s, args.Error(1)
}

func (m *MockStorage) SetRouterState(key string, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockStorage) GetSession(groupFolder string) (*string, error) {
	args := m.Called(groupFolder)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	s := args.Get(0).(string)
	return &s, args.Error(1)
}

func (m *MockStorage) SetSession(groupFolder string, sessionID string) error {
	args := m.Called(groupFolder, sessionID)
	return args.Error(0)
}

func (m *MockStorage) GetAllSessions() (map[string]string, error) {
	args := m.Called()
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockStorage) GetRegisteredGroup(jid string) (*types.RegisteredGroup, error) {
	args := m.Called(jid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.RegisteredGroup), args.Error(1)
}

func (m *MockStorage) SetRegisteredGroup(jid string, group types.RegisteredGroup) error {
	args := m.Called(jid, group)
	return args.Error(0)
}

func (m *MockStorage) GetAllRegisteredGroups() (map[string]types.RegisteredGroup, error) {
	args := m.Called()
	return args.Get(0).(map[string]types.RegisteredGroup), args.Error(1)
}

func (m *MockStorage) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStorage) InitDatabase() error { return nil }

type MockDelegate struct {
	mock.Mock
}

func (m *MockDelegate) SendMessage(ctx context.Context, jid string, text string) error {
	args := m.Called(ctx, jid, text)
	return args.Error(0)
}

func (m *MockDelegate) GetAvailableGroups() []ipc.AvailableGroup {
	args := m.Called()
	return args.Get(0).([]ipc.AvailableGroup)
}

func (m *MockDelegate) RegisteredGroups() map[string]types.RegisteredGroup {
	args := m.Called()
	return args.Get(0).(map[string]types.RegisteredGroup)
}

func (m *MockDelegate) WriteGroupsSnapshot(groupFolder string, isMain bool, availableGroups []ipc.AvailableGroup, registeredJids []string) error {
	args := m.Called(groupFolder, isMain, availableGroups, registeredJids)
	return args.Error(0)
}

func TestMessagePoller_Poll(t *testing.T) {
	mockStorage := new(MockStorage)
	mockDelegate := new(MockDelegate)
	
	p := NewMessagePoller(mockStorage, nil, nil, mockDelegate)
	assert.NotNil(t, p)
}
