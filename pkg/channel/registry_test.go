package channel

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelRegistry(t *testing.T) {
	t.Run("GetChannelFactory returns nil for unknown channel", func(t *testing.T) {
		assert.Nil(t, GetChannelFactory("nonexistent"))
	})

	t.Run("RegisterChannel and GetChannelFactory round-trip", func(t *testing.T) {
		var factory ChannelFactory = func(opts ChannelOpts) Channel { return nil }
		RegisterChannel("test-channel", factory)
		
		retrieved := GetChannelFactory("test-channel")
		assert.NotNil(t, retrieved)
		
		// In Go, functions are not directly comparable, but we can compare their pointers.
		assert.Equal(t, reflect.ValueOf(factory).Pointer(), reflect.ValueOf(retrieved).Pointer())
	})

	t.Run("GetRegisteredChannelNames includes registered channels", func(t *testing.T) {
		RegisterChannel("another-channel", func(opts ChannelOpts) Channel { return nil })
		names := GetRegisteredChannelNames()
		assert.Contains(t, names, "test-channel")
		assert.Contains(t, names, "another-channel")
	})

	t.Run("later registration overwrites earlier one", func(t *testing.T) {
		var factory1 ChannelFactory = func(opts ChannelOpts) Channel { return nil }
		var factory2 ChannelFactory = func(opts ChannelOpts) Channel { return nil }
		
		RegisterChannel("overwrite-test", factory1)
		RegisterChannel("overwrite-test", factory2)
		
		retrieved := GetChannelFactory("overwrite-test")
		assert.Equal(t, reflect.ValueOf(factory2).Pointer(), reflect.ValueOf(retrieved).Pointer())
	})
}
