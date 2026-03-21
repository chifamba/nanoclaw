package container

import (
	"fmt"
	"reflect"
	"testing"
)

func TestReadonlyMountArgs(t *testing.T) {
	hostPath := "/host/path"
	containerPath := "/container/path"
	expected := []string{"-v", fmt.Sprintf("%s:%s:ro", hostPath, containerPath)}

	args := ReadonlyMountArgs(hostPath, containerPath)

	if !reflect.DeepEqual(args, expected) {
		t.Errorf("ReadonlyMountArgs(%q, %q) = %v, want %v", hostPath, containerPath, args, expected)
	}
}

func TestStopContainer(t *testing.T) {
	name := "nanoclaw-test-123"
	expected := fmt.Sprintf("%s stop %s", ContainerRuntimeBin, name)

	cmd := StopContainer(name)

	if cmd != expected {
		t.Errorf("StopContainer(%q) = %q, want %q", name, cmd, expected)
	}
}
