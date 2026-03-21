package container

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

func TestIsValidGroupFolder(t *testing.T) {
	tests := []struct {
		folder string
		want   bool
	}{
		{"valid-folder", true},
		{"Valid_Folder-123", true},
		{"", false},
		{" ", false},
		{"global", false}, // reserved
		{"..", false},
		{"folder/with/slash", false},
		{"a" + strings.Repeat("a", 63), true},
		{"a" + strings.Repeat("a", 64), false},
	}

	for _, tt := range tests {
		if got := IsValidGroupFolder(tt.folder); got != tt.want {
			t.Errorf("IsValidGroupFolder(%q) = %v, want %v", tt.folder, got, tt.want)
		}
	}
}

func TestResolveGroupFolderPath(t *testing.T) {
	tempDir := t.TempDir()
	config.GroupsDir = tempDir

	folder := "test-group"
	got := ResolveGroupFolderPath(folder)
	want, _ := filepath.Abs(filepath.Join(tempDir, folder))
	
	if got != want {
		t.Errorf("ResolveGroupFolderPath(%q) = %v, want %v", folder, got, want)
	}

	// Test escape attempt
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("ResolveGroupFolderPath should have panicked for escaping path")
		}
	}()
	// This will panic inside AssertValidGroupFolder
	ResolveGroupFolderPath("..")
}
