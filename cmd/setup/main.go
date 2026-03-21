package main

import (
	"fmt"
	"os"

	"github.com/nanoclaw/nanoclaw/pkg/setup"
)

func main() {
	args := os.Args[1:]
	var stepName string
	var stepIdx = -1

	for i, arg := range args {
		if arg == "--step" && i+1 < len(args) {
			stepName = args[i+1]
			stepIdx = i
			break
		}
	}

	if stepName == "" {
		fmt.Printf("Usage: %s --step <environment|container|groups|register|mounts|service|verify> [args...]\n", os.Args[0])
		os.Exit(1)
	}

	// Filter out --step and its value from args
	stepArgs := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if i == stepIdx || i == stepIdx+1 {
			continue
		}
		if args[i] == "--" {
			continue
		}
		stepArgs = append(stepArgs, args[i])
	}

	switch stepName {
	case "environment":
		setup.CheckEnvironment()
	case "container":
		setup.SetupContainer(stepArgs)
	case "groups":
		setup.SyncGroups(stepArgs)
	case "register":
		setup.Register(stepArgs)
	case "mounts":
		setup.ConfigureMounts(stepArgs)
	case "service":
		setup.RunService(stepArgs)
	case "verify":
		setup.RunVerify(stepArgs)
	default:
		fmt.Printf("Unknown step: %s\n", stepName)
		fmt.Printf("Available steps: environment, container, groups, register, mounts, service, verify\n")
		os.Exit(1)
	}
}
