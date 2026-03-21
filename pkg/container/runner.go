package container

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/logger"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// RunContainerAgent executes the agent within a container and handles IPC
func RunContainerAgent(
	group types.RegisteredGroup,
	input ContainerInput,
	onProcess func(ProcessInfo),
	onOutput func(ContainerOutput),
) (ContainerOutput, error) {
	startTime := time.Now()

	groupDir := ResolveGroupFolderPath(group.Folder)
	os.MkdirAll(groupDir, 0755)

	mounts := BuildVolumeMounts(group, input.IsMain)
	safeName := regexp.MustCompile(`[^a-zA-Z0-9-]`).ReplaceAllString(group.Folder, "-")
	containerName := fmt.Sprintf("nanoclaw-%s-%d", safeName, time.Now().UnixNano()/int64(time.Millisecond))
	containerArgs := BuildContainerArgs(mounts, containerName, input.IsMain)

	logger.Debug("Container mount configuration", "group", group.Name, "containerName", containerName, "mounts", mounts, "args", strings.Join(containerArgs, " "))

	logger.Info("Spawning container agent", "group", group.Name, "containerName", containerName, "mountCount", len(mounts), "isMain", input.IsMain)

	logsDir := filepath.Join(groupDir, "logs")
	os.MkdirAll(logsDir, 0755)

	cmd := exec.Command(ContainerRuntimeBin, containerArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return ContainerOutput{Status: "error", Error: err.Error()}, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ContainerOutput{Status: "error", Error: err.Error()}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ContainerOutput{Status: "error", Error: err.Error()}, err
	}

	if err := cmd.Start(); err != nil {
		logger.Error("Container spawn error", "group", group.Name, "containerName", containerName, "error", err)
		return ContainerOutput{Status: "error", Error: fmt.Sprintf("Container spawn error: %v", err)}, err
	}

	if onProcess != nil {
		onProcess(ProcessInfo{Cmd: cmd, ContainerName: containerName})
	}

	// Write input to stdin
	inputBytes, _ := json.Marshal(input)
	stdin.Write(inputBytes)
	stdin.Close()

	var stdout, stderr strings.Builder
	var stdoutTruncated, stderrTruncated bool
	var hadStreamingOutput bool
	var newSessionID string

	// Activity detected — reset hard timeout
	timeoutMs := config.ContainerTimeout
	if group.ContainerConfig != nil && group.ContainerConfig.Timeout > 0 {
		timeoutMs = group.ContainerConfig.Timeout
	}
	// Grace period
	timeoutMs = maxInt(timeoutMs, config.IdleTimeout+30000)
	timer := time.NewTimer(time.Duration(timeoutMs) * time.Millisecond)
	resetTimeout := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(time.Duration(timeoutMs) * time.Millisecond)
	}

	parser := &OutputParser{
		OnActivityDetected: func() {
			hadStreamingOutput = true
			resetTimeout()
		},
		OnOutput: func(out ContainerOutput) {
			if out.NewSessionID != "" {
				newSessionID = out.NewSessionID
			}
			if onOutput != nil {
				onOutput(out)
			}
		},
	}

	// Read stdout
	go func() {
		reader := bufio.NewReader(stdoutPipe)
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				if !stdoutTruncated {
					if stdout.Len()+n > config.ContainerMaxOutputSize {
						stdout.WriteString(chunk[:config.ContainerMaxOutputSize-stdout.Len()])
						stdoutTruncated = true
						logger.Warn("Container stdout truncated due to size limit", "group", group.Name)
					} else {
						stdout.WriteString(chunk)
					}
				}
				parser.ParseChunk(chunk)
			}
			if err != nil {
				break
			}
		}
	}()

	// Read stderr
	go func() {
		reader := bufio.NewReader(stderrPipe)
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					logger.Debug("Container stderr", "container", group.Folder, "line", trimmed)
				}
				if !stderrTruncated {
					if stderr.Len()+len(line) > config.ContainerMaxOutputSize {
						stderr.WriteString(line[:config.ContainerMaxOutputSize-stderr.Len()])
						stderrTruncated = true
						logger.Warn("Container stderr truncated due to size limit", "group", group.Name)
					} else {
						stderr.WriteString(line)
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var finalOutput ContainerOutput
	var finalErr error

	select {
	case <-timer.C:
		logger.Error("Container timeout, stopping gracefully", "group", group.Name, "containerName", containerName)
		exec.Command(ContainerRuntimeBin, "stop", containerName).Run()
		// Force kill after a while if it doesn't stop
		time.Sleep(2 * time.Second)
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		if hadStreamingOutput {
			finalOutput = ContainerOutput{
				Status:       "success",
				NewSessionID: newSessionID,
			}
		} else {
			finalOutput = ContainerOutput{
				Status: "error",
				Error:  fmt.Sprintf("Container timed out after %dms", timeoutMs),
			}
		}
	case err := <-done:
		duration := time.Since(startTime)
		code := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				code = exitError.ExitCode()
			} else {
				code = -1
			}
		}

		// Logging
		ts := time.Now().Format("2006-01-02T15-04-05")
		logFile := filepath.Join(logsDir, fmt.Sprintf("container-%s.log", ts))
		writeLog(logFile, group, input, containerArgs, mounts, stdout.String(), stderr.String(), duration, code, stdoutTruncated, stderrTruncated)

		if code != 0 {
			logger.Error("Container exited with error", "group", group.Name, "code", code, "duration", duration, "logFile", logFile)
			finalOutput = ContainerOutput{
				Status: "error",
				Error:  fmt.Sprintf("Container exited with code %d: %s", code, lastN(stderr.String(), 200)),
			}
		} else {
			if onOutput != nil {
				logger.Info("Container completed (streaming mode)", "group", group.Name, "duration", duration, "newSessionId", newSessionID)
				finalOutput = ContainerOutput{
					Status:       "success",
					NewSessionID: newSessionID,
				}
			} else {
				out, parseErr := GetLastOutput(stdout.String())
				if parseErr != nil {
					logger.Error("Failed to parse container output", "group", group.Name, "error", parseErr)
					finalOutput = ContainerOutput{
						Status: "error",
						Error:  fmt.Sprintf("Failed to parse container output: %v", parseErr),
					}
				} else {
					logger.Info("Container completed", "group", group.Name, "duration", duration, "status", out.Status)
					finalOutput = out
				}
			}
		}
	}

	return finalOutput, finalErr
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func writeLog(logFile string, group types.RegisteredGroup, input ContainerInput, args []string, mounts []VolumeMount, stdout, stderr string, duration time.Duration, code int, stdoutTruncated, stderrTruncated bool) {
	var sb strings.Builder
	sb.WriteString("=== Container Run Log ===\n")
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Group: %s\n", group.Name))
	sb.WriteString(fmt.Sprintf("IsMain: %v\n", input.IsMain))
	sb.WriteString(fmt.Sprintf("Duration: %v\n", duration))
	sb.WriteString(fmt.Sprintf("Exit Code: %d\n", code))
	sb.WriteString(fmt.Sprintf("Stdout Truncated: %v\n", stdoutTruncated))
	sb.WriteString(fmt.Sprintf("Stderr Truncated: %v\n", stderrTruncated))
	sb.WriteString("\n")

	// verbose or error
	if code != 0 {
		sb.WriteString("=== Input ===\n")
		inputBytes, _ := json.MarshalIndent(input, "", "  ")
		sb.WriteString(string(inputBytes))
		sb.WriteString("\n\n=== Container Args ===\n")
		sb.WriteString(strings.Join(args, " "))
		sb.WriteString("\n\n=== Mounts ===\n")
		for _, m := range mounts {
			sb.WriteString(fmt.Sprintf("%s -> %s %v\n", m.HostPath, m.ContainerPath, m.ReadOnly))
		}
		sb.WriteString("\n=== Stderr ===\n")
		sb.WriteString(stderr)
		sb.WriteString("\n\n=== Stdout ===\n")
		sb.WriteString(stdout)
	} else {
		sb.WriteString("=== Input Summary ===\n")
		sb.WriteString(fmt.Sprintf("Prompt length: %d chars\n", len(input.Prompt)))
		sb.WriteString(fmt.Sprintf("Session ID: %s\n", input.SessionID))
		sb.WriteString("\n=== Mounts ===\n")
		for _, m := range mounts {
			sb.WriteString(fmt.Sprintf("%s %v\n", m.ContainerPath, m.ReadOnly))
		}
	}

	os.WriteFile(logFile, []byte(sb.String()), 0644)
}
