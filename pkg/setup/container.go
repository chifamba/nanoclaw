package setup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nanoclaw/nanoclaw/pkg/logger"
)

// SetupContainer handles building the container image and verifying it with a test run.
// Ports logic from setup/container.ts.
func SetupContainer(args []string) {
	projectRoot, _ := os.Getwd()
	image := "nanoclaw-agent:latest"
	// logFile := filepath.Join(projectRoot, "logs", "setup.log")

	var runtime string
	for i := 0; i < len(args); i++ {
		if args[i] == "--runtime" && i+1 < len(args) {
			runtime = args[i+1]
			i++
		}
	}

	if runtime == "" {
		EmitStatus("SETUP_CONTAINER", map[string]interface{}{
			"RUNTIME":  "unknown",
			"IMAGE":    image,
			"BUILD_OK": false,
			"TEST_OK":  false,
			"STATUS":   "failed",
			"ERROR":    "missing_runtime_flag",
			"LOG":      "logs/setup.log",
		})
		os.Exit(4)
	}

	// Validate runtime availability
	if runtime == "apple-container" && !CommandExists("container") {
		EmitStatus("SETUP_CONTAINER", map[string]interface{}{
			"RUNTIME":  runtime,
			"IMAGE":    image,
			"BUILD_OK": false,
			"TEST_OK":  false,
			"STATUS":   "failed",
			"ERROR":    "runtime_not_available",
			"LOG":      "logs/setup.log",
		})
		os.Exit(2)
	}

	if runtime == "docker" {
		if !CommandExists("docker") {
			EmitStatus("SETUP_CONTAINER", map[string]interface{}{
				"RUNTIME":  runtime,
				"IMAGE":    image,
				"BUILD_OK": false,
				"TEST_OK":  false,
				"STATUS":   "failed",
				"ERROR":    "runtime_not_available",
				"LOG":      "logs/setup.log",
			})
			os.Exit(2)
		}
		if err := exec.Command("docker", "info").Run(); err != nil {
			EmitStatus("SETUP_CONTAINER", map[string]interface{}{
				"RUNTIME":  runtime,
				"IMAGE":    image,
				"BUILD_OK": false,
				"TEST_OK":  false,
				"STATUS":   "failed",
				"ERROR":    "runtime_not_available",
				"LOG":      "logs/setup.log",
			})
			os.Exit(2)
		}
	}

	if runtime != "apple-container" && runtime != "docker" {
		EmitStatus("SETUP_CONTAINER", map[string]interface{}{
			"RUNTIME":  runtime,
			"IMAGE":    image,
			"BUILD_OK": false,
			"TEST_OK":  false,
			"STATUS":   "failed",
			"ERROR":    "unknown_runtime",
			"LOG":      "logs/setup.log",
		})
		os.Exit(4)
	}

	buildCmd := "docker"
	if runtime == "apple-container" {
		buildCmd = "container"
	}

	// Build
	buildOk := false
	logger.Info("Building container", map[string]interface{}{"runtime": runtime})
	
	cmd := exec.Command(buildCmd, "build", "-t", image, ".")
	cmd.Dir = filepath.Join(projectRoot, "container")
	// cmd.Stdout = os.Stdout // Or capture it
	// cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err == nil {
		buildOk = true
		logger.Info("Container build succeeded")
	} else {
		logger.Error("Container build failed", map[string]interface{}{"err": err})
	}

	// Test
	testOk := false
	if buildOk {
		logger.Info("Testing container")
		
		var testCmd *exec.Cmd
		if runtime == "apple-container" {
			testCmd = exec.Command("container", "run", "-i", "--rm", "--entrypoint", "/bin/echo", image, "Container OK")
		} else {
			testCmd = exec.Command("docker", "run", "-i", "--rm", "--entrypoint", "/bin/echo", image, "Container OK")
		}
		
		testCmd.Stdin = strings.NewReader("{}")
		var out bytes.Buffer
		testCmd.Stdout = &out
		
		if err := testCmd.Run(); err == nil {
			if strings.Contains(out.String(), "Container OK") {
				testOk = true
			}
		}
		
		if testOk {
			logger.Info("Container test result: success")
		} else {
			logger.Error("Container test failed")
		}
	}

	status := "failed"
	if buildOk && testOk {
		status = "success"
	}

	EmitStatus("SETUP_CONTAINER", map[string]interface{}{
		"RUNTIME":  runtime,
		"IMAGE":    image,
		"BUILD_OK": buildOk,
		"TEST_OK":  testOk,
		"STATUS":   status,
		"LOG":      "logs/setup.log",
	})

	if status == "failed" {
		os.Exit(1)
	}
}
