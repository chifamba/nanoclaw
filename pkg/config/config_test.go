package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig(t *testing.T) {
	// Backup environment
	originalAssistantName := os.Getenv("ASSISTANT_NAME")
	originalHasOwnNumber := os.Getenv("ASSISTANT_HAS_OWN_NUMBER")
	originalContainerImage := os.Getenv("CONTAINER_IMAGE")
	
	defer func() {
		os.Setenv("ASSISTANT_NAME", originalAssistantName)
		os.Setenv("ASSISTANT_HAS_OWN_NUMBER", originalHasOwnNumber)
		os.Setenv("CONTAINER_IMAGE", originalContainerImage)
	}()

	// Backup .env if it exists
	dotEnvPath := ".env"
	backupEnvPath := ".env.testbackup"
	if _, err := os.Stat(dotEnvPath); err == nil {
		_ = os.Rename(dotEnvPath, backupEnvPath)
		defer os.Rename(backupEnvPath, dotEnvPath)
	}

	t.Run("Defaults", func(t *testing.T) {
		os.Unsetenv("ASSISTANT_NAME")
		os.Unsetenv("ASSISTANT_HAS_OWN_NUMBER")
		os.Unsetenv("CONTAINER_IMAGE")
		
		Load()

		if AssistantName != "Andy" {
			t.Errorf("Expected AssistantName=Andy, got %s", AssistantName)
		}
		if AssistantHasOwnNumber != false {
			t.Errorf("Expected AssistantHasOwnNumber=false, got %v", AssistantHasOwnNumber)
		}
		if ContainerImage != "nanoclaw-agent:latest" {
			t.Errorf("Expected ContainerImage=nanoclaw-agent:latest, got %s", ContainerImage)
		}
		if ContainerTimeout != 1800000 {
			t.Errorf("Expected ContainerTimeout=1800000, got %d", ContainerTimeout)
		}
		if MaxConcurrentContainers != 5 {
			t.Errorf("Expected MaxConcurrentContainers=5, got %d", MaxConcurrentContainers)
		}
	})

	t.Run("EnvironmentOverrides", func(t *testing.T) {
		os.Setenv("ASSISTANT_NAME", "TestBot")
		os.Setenv("ASSISTANT_HAS_OWN_NUMBER", "true")
		os.Setenv("CONTAINER_IMAGE", "custom-image:v1")
		os.Setenv("MAX_CONCURRENT_CONTAINERS", "10")

		Load()

		if AssistantName != "TestBot" {
			t.Errorf("Expected AssistantName=TestBot, got %s", AssistantName)
		}
		if AssistantHasOwnNumber != true {
			t.Errorf("Expected AssistantHasOwnNumber=true, got %v", AssistantHasOwnNumber)
		}
		if ContainerImage != "custom-image:v1" {
			t.Errorf("Expected ContainerImage=custom-image:v1, got %s", ContainerImage)
		}
		if MaxConcurrentContainers != 10 {
			t.Errorf("Expected MaxConcurrentContainers=10, got %d", MaxConcurrentContainers)
		}
	})

	t.Run("DotEnvOverrides", func(t *testing.T) {
		os.Unsetenv("ASSISTANT_NAME")
		os.Unsetenv("ASSISTANT_HAS_OWN_NUMBER")
		
		err := os.WriteFile(dotEnvPath, []byte("ASSISTANT_NAME=DotEnvBot\nASSISTANT_HAS_OWN_NUMBER=true\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to write .env: %v", err)
		}
		defer os.Remove(dotEnvPath)

		Load()

		if AssistantName != "DotEnvBot" {
			t.Errorf("Expected AssistantName=DotEnvBot, got %s", AssistantName)
		}
		if AssistantHasOwnNumber != true {
			t.Errorf("Expected AssistantHasOwnNumber=true, got %v", AssistantHasOwnNumber)
		}
	})

	t.Run("TriggerPattern", func(t *testing.T) {
		AssistantName = "Andy"
		Load()

		tests := []struct {
			input string
			match bool
		}{
			{"@Andy hello", true},
			{"@andy hello", true},
			{"@ANDY hello", true},
			{"@Andy", true},
			{"@AndyBot", false},
			{"hello @Andy", false},
		}

		for _, tc := range tests {
			if TriggerPattern.MatchString(tc.input) != tc.match {
				t.Errorf("TriggerPattern.MatchString(%q) = %v; want %v", tc.input, !tc.match, tc.match)
			}
		}
	})

	t.Run("Paths", func(t *testing.T) {
		Load()
		
		projectRoot, _ := os.Getwd()
		expectedStoreDir := filepath.Join(projectRoot, "store")
		if StoreDir != expectedStoreDir {
			t.Errorf("Expected StoreDir=%s, got %s", expectedStoreDir, StoreDir)
		}

		if GroupsDir != filepath.Join(projectRoot, "groups") {
			t.Errorf("Expected GroupsDir to match project root groups folder")
		}
	})
}
