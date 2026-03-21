package mount

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

var (
	defaultBlockedPatterns = []string{
		".ssh",
		".gnupg",
		".gpg",
		".aws",
		".azure",
		".gcloud",
		".kube",
		".docker",
		"credentials",
		".env",
		".netrc",
		".npmrc",
		".pypirc",
		"id_rsa",
		"id_ed25519",
		"private_key",
		".secret",
	}

	cachedAllowlist   *MountAllowlist
	allowlistLoadOnce sync.Once
	allowlistLoadErr  error
)

// LoadAllowlist loads the mount allowlist from the specified path
func LoadAllowlist(path string) (*MountAllowlist, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("mount allowlist not found at %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read mount allowlist: %w", err)
	}

	var allowlist MountAllowlist
	if err := json.Unmarshal(data, &allowlist); err != nil {
		return nil, fmt.Errorf("failed to parse mount allowlist: %w", err)
	}

	// Merge with default blocked patterns and deduplicate
	patternMap := make(map[string]bool)
	for _, p := range defaultBlockedPatterns {
		patternMap[p] = true
	}
	for _, p := range allowlist.BlockedPatterns {
		patternMap[p] = true
	}

	mergedPatterns := make([]string, 0, len(patternMap))
	for p := range patternMap {
		mergedPatterns = append(mergedPatterns, p)
	}
	allowlist.BlockedPatterns = mergedPatterns

	return &allowlist, nil
}

// GetCachedAllowlist returns the cached allowlist, loading it if necessary from config.MountAllowlistPath
func GetCachedAllowlist() (*MountAllowlist, error) {
	allowlistLoadOnce.Do(func() {
		cachedAllowlist, allowlistLoadErr = LoadAllowlist(config.MountAllowlistPath)
		if allowlistLoadErr != nil {
			log.Printf("Warning: Failed to load mount allowlist: %v", allowlistLoadErr)
		} else {
			log.Printf("Mount allowlist loaded successfully from %s", config.MountAllowlistPath)
		}
	})
	return cachedAllowlist, allowlistLoadErr
}

// ResetCache clears the cached allowlist (useful for testing)
func ResetCache() {
	cachedAllowlist = nil
	allowlistLoadErr = nil
	allowlistLoadOnce = sync.Once{}
}

// expandPath expands ~ to the home directory and resolves to an absolute path
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	abs, _ := filepath.Abs(p)
	return abs
}

// getRealPath returns the real path, resolving symlinks
func getRealPath(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}

// matchesBlockedPattern checks if a path matches any blocked pattern
func matchesBlockedPattern(realPath string, blockedPatterns []string) string {
	parts := strings.Split(realPath, string(filepath.Separator))

	for _, pattern := range blockedPatterns {
		// Check if any path component matches the pattern
		for _, part := range parts {
			if part == pattern || strings.Contains(part, pattern) {
				return pattern
			}
		}

		// Also check if the full path contains the pattern
		if strings.Contains(realPath, pattern) {
			return pattern
		}
	}

	return ""
}

// findAllowedRoot checks if a real path is under an allowed root
func findAllowedRoot(realPath string, allowedRoots []AllowedRoot) *AllowedRoot {
	for _, root := range allowedRoots {
		expandedRoot := expandPath(root.Path)
		realRoot, err := getRealPath(expandedRoot)
		if err != nil {
			// Allowed root doesn't exist, skip it
			continue
		}

		// Check if realPath is under realRoot
		rel, err := filepath.Rel(realRoot, realPath)
		if err != nil {
			continue
		}

		if !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
			return &root
		}
	}

	return nil
}

// isValidContainerPath validates the container path to prevent escaping /workspace/extra/
func isValidContainerPath(containerPath string) bool {
	// Must not contain .. to prevent path traversal
	if strings.Contains(containerPath, "..") {
		return false
	}

	// Must not be absolute (it will be prefixed with /workspace/extra/)
	if filepath.IsAbs(containerPath) || strings.HasPrefix(containerPath, "/") {
		return false
	}

	// Must not be empty
	if strings.TrimSpace(containerPath) == "" {
		return false
	}

	return true
}

// ValidateMount validates a single additional mount against the allowlist
func ValidateMount(mount AdditionalMount, isMain bool) MountValidationResult {
	allowlist, err := GetCachedAllowlist()
	if err != nil || allowlist == nil {
		reason := "No mount allowlist configured"
		if err != nil {
			reason = fmt.Sprintf("Failed to load mount allowlist: %v", err)
		}
		return MountValidationResult{
			Allowed: false,
			Reason:  reason,
		}
	}

	// Derive containerPath from hostPath basename if not specified
	containerPath := mount.ContainerPath
	if containerPath == "" {
		containerPath = filepath.Base(mount.HostPath)
	}

	// Validate container path
	if !isValidContainerPath(containerPath) {
		return MountValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Invalid container path: %q - must be relative, non-empty, and not contain \"..\"", containerPath),
		}
	}

	// Expand and resolve host path
	expandedPath := expandPath(mount.HostPath)
	realPath, err := getRealPath(expandedPath)
	if err != nil {
		return MountValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Host path does not exist: %q (expanded: %q)", mount.HostPath, expandedPath),
		}
	}

	// Check blocked patterns
	if blockedMatch := matchesBlockedPattern(realPath, allowlist.BlockedPatterns); blockedMatch != "" {
		return MountValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Path matches blocked pattern %q: %q", blockedMatch, realPath),
		}
	}

	// Check under allowed root
	allowedRoot := findAllowedRoot(realPath, allowlist.AllowedRoots)
	if allowedRoot == nil {
		rootPaths := make([]string, len(allowlist.AllowedRoots))
		for i, r := range allowlist.AllowedRoots {
			rootPaths[i] = expandPath(r.Path)
		}
		return MountValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("Path %q is not under any allowed root. Allowed roots: %s", realPath, strings.Join(rootPaths, ", ")),
		}
	}

	// Determine effective readonly
	requestedReadWrite := mount.Readonly != nil && !*mount.Readonly
	effectiveReadonly := true

	if requestedReadWrite {
		if !isMain && allowlist.NonMainReadOnly {
			effectiveReadonly = true
			log.Printf("Mount forced to read-only for non-main group: %s", mount.HostPath)
		} else if !allowedRoot.AllowReadWrite {
			effectiveReadonly = true
			log.Printf("Mount forced to read-only - root does not allow read-write: %s (root: %s)", mount.HostPath, allowedRoot.Path)
		} else {
			effectiveReadonly = false
		}
	}

	description := ""
	if allowedRoot.Description != "" {
		description = fmt.Sprintf(" (%s)", allowedRoot.Description)
	}

	return MountValidationResult{
		Allowed:               true,
		Reason:                fmt.Sprintf("Allowed under root %q%s", allowedRoot.Path, description),
		RealHostPath:          realPath,
		ResolvedContainerPath: containerPath,
		EffectiveReadonly:     effectiveReadonly,
	}
}

// ValidateAdditionalMounts validates all additional mounts for a group
func ValidateAdditionalMounts(mounts []AdditionalMount, groupName string, isMain bool) []ValidatedMount {
	validated := make([]ValidatedMount, 0, len(mounts))

	for _, mount := range mounts {
		result := ValidateMount(mount, isMain)
		if result.Allowed {
			validated = append(validated, ValidatedMount{
				HostPath:      result.RealHostPath,
				ContainerPath: "/workspace/extra/" + result.ResolvedContainerPath,
				Readonly:      result.EffectiveReadonly,
			})
		} else {
			log.Printf("[%s] Additional mount REJECTED: %s (reason: %s)", groupName, mount.HostPath, result.Reason)
		}
	}

	return validated
}

// GenerateAllowlistTemplate generates a template allowlist JSON
func GenerateAllowlistTemplate() string {
	template := MountAllowlist{
		AllowedRoots: []AllowedRoot{
			{
				Path:           "~/projects",
				AllowReadWrite: true,
				Description:    "Development projects",
			},
			{
				Path:           "~/repos",
				AllowReadWrite: true,
				Description:    "Git repositories",
			},
			{
				Path:           "~/Documents/work",
				AllowReadWrite: false,
				Description:    "Work documents (read-only)",
			},
		},
		BlockedPatterns: []string{
			"password",
			"secret",
			"token",
		},
		NonMainReadOnly: true,
	}

	data, _ := json.MarshalIndent(template, "", "  ")
	return string(data)
}
