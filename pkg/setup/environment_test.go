package setup

import (
	"os"
	"testing"
)

func TestCheckEnvironment(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	// Ensure we don't panic
	CheckEnvironment()

	w.Close()
	os.Stdout = old
}
