package mount

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

func TestIsValidContainerPath(t *testing.T) {
	tests := []struct {
		path  string
		want  bool
	}{
		{"safe", true},
		{"safe/path", true},
		{"../unsafe", false},
		{"/absolute", false},
		{"", false},
		{" ", false},
		{"safe/../unsafe", false},
	}

	for _, tt := range tests {
		if got := isValidContainerPath(tt.path); got != tt.want {
			t.Errorf("isValidContainerPath(%q) = %v; want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatchesBlockedPattern(t *testing.T) {
	patterns := []string{".ssh", "secret"}
	
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/app", ""},
		{"/home/user/.ssh/id_rsa", ".ssh"},
		{"/home/user/my-secret-file.txt", "secret"},
		{"/home/user/documents/secret/info.txt", "secret"},
	}

	for _, tt := range tests {
		if got := matchesBlockedPattern(tt.path, patterns); got != tt.want {
			t.Errorf("matchesBlockedPattern(%q) = %q; want %q", tt.path, got, tt.want)
		}
	}
}

func TestLoadAllowlist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mount-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	allowlistPath := filepath.Join(tmpDir, "allowlist.json")
	allowlist := MountAllowlist{
		AllowedRoots: []AllowedRoot{
			{Path: "~/projects", AllowReadWrite: true},
		},
		BlockedPatterns: []string{"custom-blocked"},
		NonMainReadOnly: true,
	}

	data, _ := json.Marshal(allowlist)
	if err := os.WriteFile(allowlistPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadAllowlist(allowlistPath)
	if err != nil {
		t.Errorf("LoadAllowlist() error = %v", err)
		return
	}

	if len(loaded.AllowedRoots) != 1 || loaded.AllowedRoots[0].Path != "~/projects" {
		t.Errorf("Loaded allowed roots mismatch: %+v", loaded.AllowedRoots)
	}

	foundCustom := false
	for _, p := range loaded.BlockedPatterns {
		if p == "custom-blocked" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Error("Custom blocked pattern not found in merged patterns")
	}

	// Default pattern should also be there
	foundSSH := false
	for _, p := range loaded.BlockedPatterns {
		if p == ".ssh" {
			foundSSH = true
			break
		}
	}
	if !foundSSH {
		t.Error("Default blocked pattern (.ssh) not found in merged patterns")
	}
}

func TestValidateMount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mount-validate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create some directories for testing
	allowedDir := filepath.Join(tmpDir, "allowed")
	os.MkdirAll(allowedDir, 0755)
	
	blockedFile := filepath.Join(allowedDir, "my-secret.txt")
	os.WriteFile(blockedFile, []byte("secret"), 0644)

	outsideDir := filepath.Join(tmpDir, "outside")
	os.MkdirAll(outsideDir, 0755)

	allowlistPath := filepath.Join(tmpDir, "allowlist.json")
	allowlist := MountAllowlist{
		AllowedRoots: []AllowedRoot{
			{Path: allowedDir, AllowReadWrite: true, Description: "Allowed Root"},
		},
		BlockedPatterns: []string{"secret"},
		NonMainReadOnly: true,
	}
	data, _ := json.Marshal(allowlist)
	os.WriteFile(allowlistPath, data, 0644)

	// Set config path and reset cache
	oldPath := config.MountAllowlistPath
	config.MountAllowlistPath = allowlistPath
	defer func() { config.MountAllowlistPath = oldPath }()
	ResetCache()

	boolFalse := false
	boolTrue := true

	tests := []struct {
		name    string
		mount   AdditionalMount
		isMain  bool
		want    bool
		wantRO  bool
	}{
		{
			name:   "Allowed read-only",
			mount:  AdditionalMount{HostPath: allowedDir},
			isMain: true,
			want:   true,
			wantRO: true,
		},
		{
			name:   "Allowed read-write for main",
			mount:  AdditionalMount{HostPath: allowedDir, Readonly: &boolFalse},
			isMain: true,
			want:   true,
			wantRO: false,
		},
		{
			name:   "Forced read-only for non-main",
			mount:  AdditionalMount{HostPath: allowedDir, Readonly: &boolFalse},
			isMain: false,
			want:   true,
			wantRO: true,
		},
		{
			name:   "Blocked pattern",
			mount:  AdditionalMount{HostPath: blockedFile},
			isMain: true,
			want:   false,
		},
		{
			name:   "Outside allowed root",
			mount:  AdditionalMount{HostPath: outsideDir},
			isMain: true,
			want:   false,
		},
		{
			name:   "Invalid container path",
			mount:  AdditionalMount{HostPath: allowedDir, ContainerPath: "../etc"},
			isMain: true,
			want:   false,
		},
		{
			name:   "Explicit read-only",
			mount:  AdditionalMount{HostPath: allowedDir, Readonly: &boolTrue},
			isMain: true,
			want:   true,
			wantRO: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateMount(tt.mount, tt.isMain)
			if got.Allowed != tt.want {
				t.Errorf("ValidateMount() allowed = %v; want %v. Reason: %s", got.Allowed, tt.want, got.Reason)
			}
			if tt.want && got.EffectiveReadonly != tt.wantRO {
				t.Errorf("ValidateMount() EffectiveReadonly = %v; want %v", got.EffectiveReadonly, tt.wantRO)
			}
		})
	}
}

func TestValidateAdditionalMounts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mount-multi-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	allowedDir := filepath.Join(tmpDir, "allowed")
	os.MkdirAll(allowedDir, 0755)

	allowlistPath := filepath.Join(tmpDir, "allowlist.json")
	allowlist := MountAllowlist{
		AllowedRoots: []AllowedRoot{
			{Path: allowedDir, AllowReadWrite: true},
		},
		BlockedPatterns: []string{},
		NonMainReadOnly: false,
	}
	data, _ := json.Marshal(allowlist)
	os.WriteFile(allowlistPath, data, 0644)

	config.MountAllowlistPath = allowlistPath
	ResetCache()

	mounts := []AdditionalMount{
		{HostPath: allowedDir, ContainerPath: "ok"},
		{HostPath: "/nonexistent", ContainerPath: "bad"},
	}

	validated := ValidateAdditionalMounts(mounts, "test-group", true)

	if len(validated) != 1 {
		t.Errorf("Expected 1 validated mount, got %d", len(validated))
	}

	if len(validated) > 0 {
		if validated[0].ContainerPath != "/workspace/extra/ok" {
			t.Errorf("Unexpected container path: %s", validated[0].ContainerPath)
		}
	}
}
