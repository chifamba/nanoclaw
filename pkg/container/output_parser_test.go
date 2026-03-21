package container

import (
	"reflect"
	"testing"
)

func TestParseChunk(t *testing.T) {
	var outputs []ContainerOutput
	activityCount := 0

	parser := &OutputParser{
		OnOutput: func(out ContainerOutput) {
			outputs = append(outputs, out)
		},
		OnActivityDetected: func() {
			activityCount++
		},
	}

	// Chunk 1: Partial start
	parser.ParseChunk("some random logs\n---NANOCLAW_OUTPUT_")
	if len(outputs) != 0 {
		t.Error("expected no outputs yet")
	}

	// Chunk 2: Finish start and partial JSON
	parser.ParseChunk("START---\n{\"status\": \"success\", \"result\": \"hello")
	if len(outputs) != 0 {
		t.Error("expected no outputs yet")
	}

	// Chunk 3: Finish JSON and end marker
	parser.ParseChunk(" world\"}\n---NANOCLAW_OUTPUT_END---")
	if len(outputs) != 1 {
		t.Errorf("expected 1 output, got %d", len(outputs))
	} else if outputs[0].Result != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", outputs[0].Result)
	}
	if activityCount != 1 {
		t.Errorf("expected 1 activity detection, got %d", activityCount)
	}

	// Chunk 4: Multiple outputs in one chunk
	parser.ParseChunk("\n---NANOCLAW_OUTPUT_START---\n{\"status\": \"success\", \"result\": \"one\"}\n---NANOCLAW_OUTPUT_END---\n---NANOCLAW_OUTPUT_START---\n{\"status\": \"success\", \"result\": \"two\"}\n---NANOCLAW_OUTPUT_END---\n")
	if len(outputs) != 3 {
		t.Errorf("expected 3 outputs total, got %d", len(outputs))
	}
}

func TestGetLastOutput(t *testing.T) {
	stdout := "log line 1\n---NANOCLAW_OUTPUT_START---\n{\"status\": \"success\", \"result\": \"first\"}\n---NANOCLAW_OUTPUT_END---\nlog line 2\n---NANOCLAW_OUTPUT_START---\n{\"status\": \"success\", \"result\": \"last\"}\n---NANOCLAW_OUTPUT_END---\n"
	
	expected := ContainerOutput{
		Status: "success",
		Result: "last",
	}

	out, err := GetLastOutput(stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(out, expected) {
		t.Errorf("expected %+v, got %+v", expected, out)
	}

	// Fallback case: last non-empty line is JSON
	stdoutFallback := "random logs\n{\"status\": \"error\", \"error\": \"fallback\"}"
	expectedFallback := ContainerOutput{
		Status: "error",
		Error:  "fallback",
	}
	outFallback, err := GetLastOutput(stdoutFallback)
	if err != nil {
		t.Fatalf("unexpected error in fallback: %v", err)
	}
	if !reflect.DeepEqual(outFallback, expectedFallback) {
		t.Errorf("expected %+v, got %+v", expectedFallback, outFallback)
	}
}
