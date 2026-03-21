package channel

import (
	"sync"
)

var (
	registry = make(map[string]ChannelFactory)
	mu       sync.RWMutex
)

// RegisterChannel adds a channel factory to the global registry.
func RegisterChannel(name string, factory ChannelFactory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// GetChannelFactory retrieves a channel factory by its name.
func GetChannelFactory(name string) ChannelFactory {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// GetRegisteredChannelNames returns a list of all registered channel names.
func GetRegisteredChannelNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
