package db

import (
	"regexp"
	"strings"
)

var groupFolderPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
var reservedFolders = map[string]bool{
	"global": true,
}

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
