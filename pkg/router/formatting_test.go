package router

import (
	"strings"
	"testing"
)

func TestEscapeXml(t *testing.T) {
	if EscapeXml("a & b") != "a &amp; b" {
		t.Errorf("Expected 'a &amp; b', got '%s'", EscapeXml("a & b"))
	}
	if EscapeXml("<tag>") != "&lt;tag&gt;" {
		t.Errorf("Expected '&lt;tag&gt;', got '%s'", EscapeXml("<tag>"))
	}
	if EscapeXml(`"quoted"`) != "&quot;quoted&quot;" {
		t.Errorf("Expected '&quot;quoted&quot;', got '%s'", EscapeXml(`"quoted"`))
	}
}

func TestFormatLocalTime(t *testing.T) {
	result := formatLocalTime("2026-02-04T18:30:00.000Z", "America/New_York")
	if !strings.Contains(result, "1:30") || !strings.Contains(result, "PM") || !strings.Contains(result, "Feb") || !strings.Contains(result, "2026") {
		t.Errorf("Expected result to contain 1:30 PM, Feb, 2026, got: %s", result)
	}

	utc := "2026-06-15T12:00:00.000Z"
	ny := formatLocalTime(utc, "America/New_York")
	tokyo := formatLocalTime(utc, "Asia/Tokyo")

	if !strings.Contains(ny, "8:00") {
		t.Errorf("Expected NY to contain 8:00, got: %s", ny)
	}
	if !strings.Contains(tokyo, "9:00") {
		t.Errorf("Expected Tokyo to contain 9:00, got: %s", tokyo)
	}
}

func TestFormatOutbound(t *testing.T) {
	if StripInternalTags("<internal>hidden</internal>") != "" {
		t.Errorf("Expected empty string")
	}

	if FormatOutbound("hello world") != "hello world" {
		t.Errorf("Expected 'hello world'")
	}

	if FormatOutbound("<internal>hidden</internal>") != "" {
		t.Errorf("Expected empty string")
	}

	if FormatOutbound("<internal>thinking</internal>The answer is 42") != "The answer is 42" {
		t.Errorf("Expected 'The answer is 42'")
	}
}

func TestStripInternalTags(t *testing.T) {
	if StripInternalTags("<internal>only this</internal>") != "" {
		t.Errorf("Expected empty string")
	}
}
