package router

import (
	"strings"
	"testing"

	"github.com/nanoclaw/nanoclaw/pkg/types"
)

func makeMsg(overrides ...func(*types.NewMessage)) types.NewMessage {
	msg := types.NewMessage{
		ID:         "1",
		ChatJID:    "group@g.us",
		Sender:     "123@s.whatsapp.net",
		SenderName: "Alice",
		Content:    "hello",
		Timestamp:  "2024-01-01T00:00:00.000Z",
	}
	for _, override := range overrides {
		override(&msg)
	}
	return msg
}

func TestEscapeXml(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a & b", "a &amp; b"},
		{"a < b", "a &lt; b"},
		{"a > b", "a &gt; b"},
		{"\"hello\"", "&quot;hello&quot;"},
		{"a & b < c > d \"e\"", "a &amp; b &lt; c &gt; d &quot;e&quot;"},
		{"hello world", "hello world"},
		{"", ""},
	}

	for _, test := range tests {
		if got := EscapeXml(test.input); got != test.expected {
			t.Errorf("EscapeXml(%q) = %q; want %q", test.input, got, test.expected)
		}
	}
}

func TestFormatMessages(t *testing.T) {
	tz := "UTC"

	t.Run("formats a single message", func(t *testing.T) {
		msgs := []types.NewMessage{makeMsg()}
		result := FormatMessages(msgs, tz)

		if !strings.Contains(result, `<context timezone="UTC" />`) {
			t.Errorf("Result missing context header: %s", result)
		}
		if !strings.Contains(result, `sender="Alice"`) {
			t.Errorf("Result missing sender: %s", result)
		}
		if !strings.Contains(result, `>hello</message>`) {
			t.Errorf("Result missing message content: %s", result)
		}
		// Go's format for Jan 1, 2024, 12:00 AM might vary from JS, 
		// but it should contain "Jan 1, 2024"
		if !strings.Contains(result, "Jan 1, 2024") {
			t.Errorf("Result missing date: %s", result)
		}
	})

	t.Run("formats multiple messages", func(t *testing.T) {
		msgs := []types.NewMessage{
			makeMsg(func(m *types.NewMessage) {
				m.ID = "1"
				m.SenderName = "Alice"
				m.Content = "hi"
			}),
			makeMsg(func(m *types.NewMessage) {
				m.ID = "2"
				m.SenderName = "Bob"
				m.Content = "hey"
				m.Timestamp = "2024-01-01T01:00:00.000Z"
			}),
		}
		result := FormatMessages(msgs, tz)

		if !strings.Contains(result, `sender="Alice"`) {
			t.Errorf("Result missing Alice: %s", result)
		}
		if !strings.Contains(result, `sender="Bob"`) {
			t.Errorf("Result missing Bob: %s", result)
		}
		if !strings.Contains(result, `>hi</message>`) {
			t.Errorf("Result missing Alice content: %s", result)
		}
		if !strings.Contains(result, `>hey</message>`) {
			t.Errorf("Result missing Bob content: %s", result)
		}
	})

	t.Run("escapes special characters", func(t *testing.T) {
		msgs := []types.NewMessage{
			makeMsg(func(m *types.NewMessage) {
				m.SenderName = "A & B <Co>"
				m.Content = "<script>alert(\"xss\")</script>"
			}),
		}
		result := FormatMessages(msgs, tz)

		if !strings.Contains(result, `sender="A &amp; B &lt;Co&gt;"`) {
			t.Errorf("Sender not escaped correctly: %s", result)
		}
		if !strings.Contains(result, `&lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;`) {
			t.Errorf("Content not escaped correctly: %s", result)
		}
	})

	t.Run("handles empty array", func(t *testing.T) {
		result := FormatMessages([]types.NewMessage{}, tz)
		if !strings.Contains(result, `<context timezone="UTC" />`) {
			t.Errorf("Result missing context header: %s", result)
		}
		if !strings.Contains(result, "<messages>\n\n</messages>") {
			t.Errorf("Result missing empty messages tag: %q", result)
		}
	})

	t.Run("converts timestamps to local time", func(t *testing.T) {
		// 2024-01-01T18:30:00Z in America/New_York (EST) = 1:30 PM
		msgs := []types.NewMessage{
			makeMsg(func(m *types.NewMessage) {
				m.Timestamp = "2024-01-01T18:30:00.000Z"
			}),
		}
		result := FormatMessages(msgs, "America/New_York")

		if !strings.Contains(result, "1:30") {
			t.Errorf("Result missing local time 1:30: %s", result)
		}
		if !strings.Contains(result, "PM") {
			t.Errorf("Result missing PM: %s", result)
		}
		if !strings.Contains(result, `<context timezone="America/New_York" />`) {
			t.Errorf("Result missing correct timezone in header: %s", result)
		}
	})
}

func TestStripInternalTags(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello <internal>secret</internal> world", "hello  world"},
		{"hello <internal>\nsecret\nstuff\n</internal> world", "hello  world"},
		{"<internal>a</internal>hello<internal>b</internal>", "hello"},
		{"<internal>only this</internal>", ""},
	}

	for _, test := range tests {
		if got := StripInternalTags(test.input); got != test.expected {
			t.Errorf("StripInternalTags(%q) = %q; want %q", test.input, got, test.expected)
		}
	}
}

func TestFormatOutbound(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello world", "hello world"},
		{"<internal>hidden</internal>", ""},
		{"<internal>thinking</internal>The answer is 42", "The answer is 42"},
	}

	for _, test := range tests {
		if got := FormatOutbound(test.input); got != test.expected {
			t.Errorf("FormatOutbound(%q) = %q; want %q", test.input, got, test.expected)
		}
	}
}
