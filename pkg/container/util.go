package container

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/config"
)

var (
	groupFolderPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	reservedFolders    = map[string]bool{
		"global": true,
	}
)

// IsValidGroupFolder checks if a folder name is valid for a group
func IsValidGroupFolder(folder string) bool {
	if folder == "" {
		return false
	}
	if folder != strings.TrimSpace(folder) {
		return false
	}
	if !groupFolderPattern.MatchString(folder) {
		return false
	}
	if strings.Contains(folder, "/") || strings.Contains(folder, "\\") {
		return false
	}
	if strings.Contains(folder, "..") {
		return false
	}
	if reservedFolders[strings.ToLower(folder)] {
		return false
	}
	return true
}

// AssertValidGroupFolder panics if a folder name is invalid
func AssertValidGroupFolder(folder string) {
	if !IsValidGroupFolder(folder) {
		panic(fmt.Sprintf("Invalid group folder %q", folder))
	}
}

func ensureWithinBase(baseDir, resolvedPath string) error {
	rel, err := filepath.Rel(baseDir, resolvedPath)
	if err != nil {
		return err
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("path escapes base directory: %s", resolvedPath)
	}
	return nil
}

// ResolveGroupFolderPath returns the absolute path to a group's folder
func ResolveGroupFolderPath(folder string) string {
	AssertValidGroupFolder(folder)
	groupPath, _ := filepath.Abs(filepath.Join(config.GroupsDir, folder))
	if err := ensureWithinBase(config.GroupsDir, groupPath); err != nil {
		panic(err)
	}
	return groupPath
}

// ResolveGroupIpcPath returns the absolute path to a group's IPC folder
func ResolveGroupIpcPath(folder string) string {
	AssertValidGroupFolder(folder)
	ipcBaseDir, _ := filepath.Abs(filepath.Join(config.DataDir, "ipc"))
	ipcPath, _ := filepath.Abs(filepath.Join(ipcBaseDir, folder))
	if err := ensureWithinBase(ipcBaseDir, ipcPath); err != nil {
		panic(err)
	}
	return ipcPath
}
