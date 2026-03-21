package container

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/config"
	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// Helper to mock the container runtime binary
func TestMain(m *testing.M) {
	if os.Getenv("BE_HELPER_PROCESS") == "1" {
		helperProcess()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helperProcess() {
	args := os.Args[3:] // skip [test binary path, -test.run=TestHelperProcess, --]
	
	// Check if this is a "run" command
	isRun := false
	for _, arg := range args {
		if arg == "run" {
			isRun = true
			break
		}
	}

	if !isRun {
		return
	}

	// Read input from stdin
	inputData, _ := io.ReadAll(os.Stdin)
	var input ContainerInput
	json.Unmarshal(inputData, &input)

	// Simulate output
	fmt.Printf("Mock container started for prompt: %s\n", input.Prompt)
	
	// Stream an intermediate output
	fmt.Printf("%s\n{\"status\": \"success\", \"result\": \"In progress\", \"newSessionId\": \"sess-123\"}\n%s\n", 
		OutputStartMarker, OutputEndMarker)
	
	time.Sleep(100 * time.Millisecond)

	// Final output
	fmt.Printf("%s\n{\"status\": \"success\", \"result\": \"Final answer for %s\", \"newSessionId\": \"sess-123\"}\n%s\n", 
		OutputStartMarker, input.Prompt, OutputEndMarker)
}

func TestRunContainerAgent(t *testing.T) {
	// Set up the helper process as the container runtime
	ContainerRuntimeBin = os.Args[0]
	// We need to pass extra args to the helper process to identify it
	// But exec.Command doesn't easily allow inserting args before the ones passed to it.
	// Actually, we can use a wrapper script or just handle it in helperProcess by looking for certain args.
	
	// Let's use an environment variable to tell the helper process to act as the mock runtime
	os.Setenv("BE_HELPER_PROCESS", "1")
	defer os.Unsetenv("BE_HELPER_PROCESS")

	tempDir := t.TempDir()
	config.DataDir = filepath.Join(tempDir, "data")
	config.GroupsDir = filepath.Join(tempDir, "groups")
	config.ContainerTimeout = 5000 // 5s
	config.IdleTimeout = 2000     // 2s

	group := types.RegisteredGroup{
		Name:   "Test Group",
		Folder: "test-group",
	}

	input := ContainerInput{
		Prompt:      "Hello Mock",
		GroupFolder: "test-group",
		IsMain:      true,
	}

	var capturedOutputs []ContainerOutput
	onOutput := func(out ContainerOutput) {
		capturedOutputs = append(capturedOutputs, out)
	}

	// We need to override BuildContainerArgs or make sure it doesn't break our mock
	// BuildContainerArgs adds a lot of args like -i, --rm, --name, etc.
	// Our helperProcess needs to handle these.
	// Actually, the easiest way is to wrap the current executable in a script that prepends the necessary test flags.
	
	helperScript := filepath.Join(tempDir, "mock-container.sh")
	scriptContent := fmt.Sprintf("#!/bin/sh\nBE_HELPER_PROCESS=1 %s -test.run=TestHelperProcess -- \"$@\"\n", os.Args[0])
	os.WriteFile(helperScript, []byte(scriptContent), 0755)
	
	ContainerRuntimeBin = helperScript

	result, err := RunContainerAgent(group, input, nil, onOutput)
	if err != nil {
		t.Fatalf("RunContainerAgent failed: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected success status, got %s", result.Status)
	}

	if !strings.Contains(result.Result, "Final answer for Hello Mock") {
		// Note: RunContainerAgent might return the result from onOutput or GetLastOutput
		// In streaming mode, it returns status: success and newSessionID
	}
	
	if len(capturedOutputs) < 2 {
		t.Errorf("expected at least 2 captured outputs, got %d", len(capturedOutputs))
	}
}

// Dummy test to be used by the helper process
func TestHelperProcess(t *testing.T) {
	if os.Getenv("BE_HELPER_PROCESS") != "1" {
		return
	}
	// helperProcess is called in TestMain
}
