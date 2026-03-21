package env

import (
	"os"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	// Backup original .env
	originalEnv, err := os.ReadFile(".env")
	if err != nil {
		originalEnv = []byte{}
	}
	// Restore after test
	defer func() {
		_ = os.WriteFile(".env", originalEnv, 0644)
	}()

	// Test 1: .env file not found
	_ = os.Remove(".env")
	result := ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if len(result) != 0 {
		t.Errorf("Expected empty map when .env not found, got %v", result)
	}

	// Test 2: .env file with simple key=value
	_ = os.WriteFile(".env", []byte("ASSISTANT_NAME=Andy\nPOLL_INTERVAL=2000\n"), 0644)
	result = ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if result["ASSISTANT_NAME"] != "Andy" {
		t.Errorf("Expected ASSISTANT_NAME=Andy, got %s", result["ASSISTANT_NAME"])
	}
	if result["POLL_INTERVAL"] != "2000" {
		t.Errorf("Expected POLL_INTERVAL=2000, got %s", result["POLL_INTERVAL"])
	}

	// Test 3: .env file with quoted values
	_ = os.WriteFile(".env", []byte("ASSISTANT_NAME=\"Andy Bot\"\nPOLL_INTERVAL='3000'\n"), 0644)
	result = ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if result["ASSISTANT_NAME"] != "Andy Bot" {
		t.Errorf("Expected ASSISTANT_NAME=Andy Bot, got %s", result["ASSISTANT_NAME"])
	}
	if result["POLL_INTERVAL"] != "3000" {
		t.Errorf("Expected POLL_INTERVAL=3000, got %s", result["POLL_INTERVAL"])
	}

	// Test 4: .env file with comments and blank lines
	_ = os.WriteFile(".env", []byte("# Comment\n\nASSISTANT_NAME=Andy\n# Another comment\nPOLL_INTERVAL=2000\n"), 0644)
	result = ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if result["ASSISTANT_NAME"] != "Andy" {
		t.Errorf("Expected ASSISTANT_NAME=Andy, got %s", result["ASSISTANT_NAME"])
	}
	if result["POLL_INTERVAL"] != "2000" {
		t.Errorf("Expected POLL_INTERVAL=2000, got %s", result["POLL_INTERVAL"])
	}

	// Test 5: Requested key not in .env
	_ = os.WriteFile(".env", []byte("ASSISTANT_NAME=Andy\n"), 0644)
	result = ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if result["ASSISTANT_NAME"] != "Andy" {
		t.Errorf("Expected ASSISTANT_NAME=Andy, got %s", result["ASSISTANT_NAME"])
	}
	if _, exists := result["POLL_INTERVAL"]; exists {
		t.Errorf("Expected POLL_INTERVAL not to be in result")
	}

	// Test 6: Empty value
	_ = os.WriteFile(".env", []byte("ASSISTANT_NAME=\nPOLL_INTERVAL=2000\n"), 0644)
	result = ReadEnvFile([]string{"ASSISTANT_NAME", "POLL_INTERVAL"})
	if result["ASSISTANT_NAME"] != "" {
		t.Errorf("Expected ASSISTANT_NAME empty, got %s", result["ASSISTANT_NAME"])
	}
	if result["POLL_INTERVAL"] != "2000" {
		t.Errorf("Expected POLL_INTERVAL=2000, got %s", result["POLL_INTERVAL"])
	}
}