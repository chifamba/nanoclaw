package setup

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestEmitStatus(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fields := map[string]interface{}{
		"KEY1": "VALUE1",
		"KEY2": 123,
		"KEY3": true,
	}
	EmitStatus("TEST_STEP", fields)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = old

	output := buf.String()

	expectedHeader := "=== NANOCLAW SETUP: TEST_STEP ==="
	if !strings.Contains(output, expectedHeader) {
		t.Errorf("expected header %q, not found in output", expectedHeader)
	}

	expectedFields := []string{
		"KEY1: VALUE1",
		"KEY2: 123",
		"KEY3: true",
	}
	for _, f := range expectedFields {
		if !strings.Contains(output, f) {
			t.Errorf("expected field %q, not found in output", f)
		}
	}

	expectedEnd := "=== END ==="
	if !strings.Contains(output, expectedEnd) {
		t.Errorf("expected end %q, not found in output", expectedEnd)
	}
}
