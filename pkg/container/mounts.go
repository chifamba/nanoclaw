package container

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/mount"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// BuildVolumeMounts prepares the list of volume mounts for a container
func BuildVolumeMounts(group types.RegisteredGroup, isMain bool) []VolumeMount {
	mounts := []VolumeMount{}
	projectRoot, _ := os.Getwd()
	groupDir := ResolveGroupFolderPath(group.Folder)

	if isMain {
		// Main gets the project root read-only.
		mounts = append(mounts, VolumeMount{
			HostPath:      projectRoot,
			ContainerPath: "/workspace/project",
			ReadOnly:      true,
		})

		// Shadow .env so the agent cannot read secrets from the mounted project root.
		if ContainerRuntimeBin != "container" {
			emptyEnvPath := filepath.Join(config.DataDir, "empty-env")
			if _, err := os.Stat(emptyEnvPath); os.IsNotExist(err) {
				os.MkdirAll(config.DataDir, 0755)
				os.WriteFile(emptyEnvPath, []byte(""), 0644)
			}
			mounts = append(mounts, VolumeMount{
				HostPath:      emptyEnvPath,
				ContainerPath: "/workspace/project/.env",
				ReadOnly:      true,
			})
		}

		// Main also gets its group folder as the working directory
		mounts = append(mounts, VolumeMount{
			HostPath:      groupDir,
			ContainerPath: "/workspace/group",
			ReadOnly:      false,
		})
	} else {
		// Other groups only get their own folder
		mounts = append(mounts, VolumeMount{
			HostPath:      groupDir,
			ContainerPath: "/workspace/group",
			ReadOnly:      false,
		})

		// Global memory directory (read-only for non-main)
		globalDir := filepath.Join(config.GroupsDir, "global")
		if _, err := os.Stat(globalDir); err == nil {
			mounts = append(mounts, VolumeMount{
				HostPath:      globalDir,
				ContainerPath: "/workspace/global",
				ReadOnly:      true,
			})
		}
	}

	// Per-group Claude sessions directory
	groupSessionsDir := filepath.Join(config.DataDir, "sessions", group.Folder, ".claude")
	os.MkdirAll(groupSessionsDir, 0755)
	settingsFile := filepath.Join(groupSessionsDir, "settings.json")
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		settings := map[string]interface{}{
			"env": map[string]string{
				"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS":         "1",
				"CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD": "1",
				"CLAUDE_CODE_DISABLE_AUTO_MEMORY":              "0",
			},
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		os.WriteFile(settingsFile, append(data, '\n'), 0644)
	}

	// Sync skills from container/skills/ into each group's .claude/skills/
	skillsSrc := filepath.Join(projectRoot, "container", "skills")
	skillsDst := filepath.Join(groupSessionsDir, "skills")
	if _, err := os.Stat(skillsSrc); err == nil {
		entries, _ := os.ReadDir(skillsSrc)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			srcDir := filepath.Join(skillsSrc, entry.Name())
			dstDir := filepath.Join(skillsDst, entry.Name())
			copyDir(srcDir, dstDir)
		}
	}
	mounts = append(mounts, VolumeMount{
		HostPath:      groupSessionsDir,
		ContainerPath: "/home/node/.claude",
		ReadOnly:      false,
	})

	// Per-group IPC namespace
	groupIpcDir := ResolveGroupIpcPath(group.Folder)
	os.MkdirAll(filepath.Join(groupIpcDir, "messages"), 0755)
	os.MkdirAll(filepath.Join(groupIpcDir, "tasks"), 0755)
	os.MkdirAll(filepath.Join(groupIpcDir, "input"), 0755)
	mounts = append(mounts, VolumeMount{
		HostPath:      groupIpcDir,
		ContainerPath: "/workspace/ipc",
		ReadOnly:      false,
	})

	// Copy agent-runner source into a per-group writable location
	agentRunnerSrc := filepath.Join(projectRoot, "container", "agent-runner", "src")
	groupAgentRunnerDir := filepath.Join(config.DataDir, "sessions", group.Folder, "agent-runner-src")
	if _, err := os.Stat(groupAgentRunnerDir); os.IsNotExist(err) {
		if _, err := os.Stat(agentRunnerSrc); err == nil {
			copyDir(agentRunnerSrc, groupAgentRunnerDir)
		}
	}
	mounts = append(mounts, VolumeMount{
		HostPath:      groupAgentRunnerDir,
		ContainerPath: "/app/src",
		ReadOnly:      false,
	})

	// Additional mounts validated against external allowlist
	if group.ContainerConfig != nil && len(group.ContainerConfig.AdditionalMounts) > 0 {
		// Convert pkg/types.AdditionalMount to pkg/mount.AdditionalMount
		mountItems := make([]mount.AdditionalMount, len(group.ContainerConfig.AdditionalMounts))
		for i, m := range group.ContainerConfig.AdditionalMounts {
			mountItems[i] = mount.AdditionalMount{
				HostPath:      m.HostPath,
				ContainerPath: m.ContainerPath,
				Readonly:      &m.ReadOnly,
			}
		}
		validatedMounts := mount.ValidateAdditionalMounts(mountItems, group.Name, isMain)
		for _, vm := range validatedMounts {
			mounts = append(mounts, VolumeMount{
				HostPath:      vm.HostPath,
				ContainerPath: vm.ContainerPath,
				ReadOnly:      vm.Readonly,
			})
		}
	}

	return mounts
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, si.Mode())
}
