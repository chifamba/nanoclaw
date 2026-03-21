package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

func TestBuildVolumeMounts(t *testing.T) {
	tempDir := t.TempDir()
	config.DataDir = filepath.Join(tempDir, "data")
	config.GroupsDir = filepath.Join(tempDir, "groups")
	
	// Prepare dummy group
	group := types.RegisteredGroup{
		Name:   "Test Group",
		Folder: "test-group",
	}

	// Create necessary folders
	os.MkdirAll(config.GroupsDir, 0755)
	os.MkdirAll(config.DataDir, 0755)
	
	groupDir := filepath.Join(config.GroupsDir, group.Folder)
	os.MkdirAll(groupDir, 0755)

	// Test case: Main group
	mountsMain := BuildVolumeMounts(group, true)
	
	foundWorkspaceGroup := false
	foundClaudeSessions := false
	foundIpc := false
	
	for _, m := range mountsMain {
		if m.ContainerPath == "/workspace/group" {
			foundWorkspaceGroup = true
			if m.HostPath != groupDir {
				t.Errorf("expected /workspace/group to mount %s, got %s", groupDir, m.HostPath)
			}
		}
		if m.ContainerPath == "/home/node/.claude" {
			foundClaudeSessions = true
		}
		if m.ContainerPath == "/workspace/ipc" {
			foundIpc = true
		}
	}

	if !foundWorkspaceGroup {
		t.Error("expected /workspace/group mount not found")
	}
	if !foundClaudeSessions {
		t.Error("expected /home/node/.claude mount not found")
	}
	if !foundIpc {
		t.Error("expected /workspace/ipc mount not found")
	}

	// Test case: Non-main group
	mountsOther := BuildVolumeMounts(group, false)
	foundGlobal := false
	
	// Create global directory to trigger its mounting
	globalDir := filepath.Join(config.GroupsDir, "global")
	os.MkdirAll(globalDir, 0755)
	
	mountsWithGlobal := BuildVolumeMounts(group, false)
	for _, m := range mountsWithGlobal {
		if m.ContainerPath == "/workspace/global" {
			foundGlobal = true
			if !m.ReadOnly {
				t.Error("expected /workspace/global to be readonly")
			}
		}
	}
	
	if !foundGlobal {
		t.Error("expected /workspace/global mount not found when global directory exists")
	}
	
	_ = mountsOther
}
