package container

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestIsValidGroupFolder(t *testing.T) {
	tests := []struct {
		folder string
		want   bool
	}{
		{"main", true},
		{"family-chat", true},
		{"Team_42", true},
		{"../../etc", false},
		{"/tmp", false},
		{"global", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsValidGroupFolder(tt.folder); got != tt.want {
			t.Errorf("IsValidGroupFolder(%q) = %v, want %v", tt.folder, got, tt.want)
		}
	}
}

func TestResolveGroupFolderPath(t *testing.T) {
	folder := "family-chat"
	got := ResolveGroupFolderPath(folder)
	want := filepath.Join("groups", "family-chat")
	if !strings.HasSuffix(got, want) {
		t.Errorf("ResolveGroupFolderPath(%q) = %v, want %v", folder, got, want)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("ResolveGroupFolderPath should have panicked for escaping path")
		}
	}()
	// This will panic inside AssertValidGroupFolder
	ResolveGroupFolderPath("../../etc")
}

func TestResolveGroupIpcPath(t *testing.T) {
	folder := "family-chat"
	got := ResolveGroupIpcPath(folder)
	want := filepath.Join("data", "ipc", "family-chat")
	if !strings.HasSuffix(got, want) {
		t.Errorf("ResolveGroupIpcPath(%q) = %v, want %v", folder, got, want)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("ResolveGroupIpcPath should have panicked for escaping path")
		}
	}()
	ResolveGroupIpcPath("/tmp")
}
