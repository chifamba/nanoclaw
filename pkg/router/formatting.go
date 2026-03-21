package router

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nanoclaw/nanoclaw/pkg/types"
)

// EscapeXml escapes special characters into XML-safe entities.
// Matches the logic in src/router.ts.
func EscapeXml(s string) string {
	if s == "" {
		return ""
	}
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}

// formatLocalTime converts a UTC ISO timestamp to a localized display string.
// Matches the logic in src/timezone.ts.
func formatLocalTime(utcIso string, timezone string) string {
	// Try parsing RFC3339 first (includes Z or offset)
	t, err := time.Parse(time.RFC3339, utcIso)
	if err != nil {
		// Fallback to simpler format if ISO with milliseconds etc. fails
		// time.RFC3339 covers most, but maybe not all variations of JS's date.toISOString()
		// which could have more fractional seconds than 3.
		const layout = "2006-01-02T15:04:05.000Z"
		t, err = time.Parse(layout, utcIso)
		if err != nil {
			return utcIso // Fallback if all else fails
		}
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	localT := t.In(loc)
	// Match JS toLocaleString 'en-US' options:
	// year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', hour12: true
	// Result: "Jan 1, 2024, 1:30 PM"
	return localT.Format("Jan 2, 2006, 3:04 PM")
}

// FormatMessages formats a list of NewMessage objects into an XML context.
// Matches the logic in src/router.ts.
func FormatMessages(messages []types.NewMessage, timezone string) string {
	var lines []string
	for _, m := range messages {
		displayTime := formatLocalTime(m.Timestamp, timezone)
		line := fmt.Sprintf(`<message sender="%s" time="%s">%s</message>`,
			EscapeXml(m.SenderName),
			EscapeXml(displayTime),
			EscapeXml(m.Content))
		lines = append(lines, line)
	}

	header := fmt.Sprintf(`<context timezone="%s" />`, EscapeXml(timezone))
	messagesBody := strings.Join(lines, "\n")
	
	return fmt.Sprintf("%s\n<messages>\n%s\n</messages>", header, messagesBody)
}

var internalTagsRegex = regexp.MustCompile(`(?s)<internal>.*?</internal>`)

// StripInternalTags removes all <internal> tag blocks from the text.
// Matches the logic in src/router.ts.
func StripInternalTags(text string) string {
	stripped := internalTagsRegex.ReplaceAllString(text, "")
	return strings.TrimSpace(stripped)
}

// FormatOutbound prepares text for sending by stripping internal tags.
// Matches the logic in src/router.ts.
func FormatOutbound(rawText string) string {
	text := StripInternalTags(rawText)
	if text == "" {
		return ""
	}
	return text
}
