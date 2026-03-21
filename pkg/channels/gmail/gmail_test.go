package gmail

import (
	"encoding/base64"
	"testing"

	"google.golang.org/api/gmail/v1"
)

func TestExtractEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Doe <john@example.com>", "john@example.com"},
		{"<jane@example.com>", "jane@example.com"},
		{"only@example.com", "only@example.com"},
		{"Invalid Email", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractEmail(tt.input)
		if result != tt.expected {
			t.Errorf("extractEmail(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractBody(t *testing.T) {
	tests := []struct {
		name     string
		payload  *gmail.MessagePart
		expected string
	}{
		{
			name: "Simple text body",
			payload: &gmail.MessagePart{
				Body: &gmail.MessagePartBody{
					Data: base64.URLEncoding.EncodeToString([]byte("Hello world")),
				},
			},
			expected: "Hello world",
		},
		{
			name: "Multipart text/plain",
			payload: &gmail.MessagePart{
				MimeType: "multipart/alternative",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "text/html",
						Body: &gmail.MessagePartBody{
							Data: base64.URLEncoding.EncodeToString([]byte("<html><body>Hello</body></html>")),
						},
					},
					{
						MimeType: "text/plain",
						Body: &gmail.MessagePartBody{
							Data: base64.URLEncoding.EncodeToString([]byte("Hello plain")),
						},
					},
				},
			},
			expected: "Hello plain",
		},
		{
			name: "Nested multipart",
			payload: &gmail.MessagePart{
				MimeType: "multipart/mixed",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "multipart/alternative",
						Parts: []*gmail.MessagePart{
							{
								MimeType: "text/plain",
								Body: &gmail.MessagePartBody{
									Data: base64.URLEncoding.EncodeToString([]byte("Nested hello")),
								},
							},
						},
					},
				},
			},
			expected: "Nested hello",
		},
		{
			name: "Empty body",
			payload: &gmail.MessagePart{
				Body: &gmail.MessagePartBody{},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBody(tt.payload)
			if result != tt.expected {
				t.Errorf("extractBody() = %q; want %q", result, tt.expected)
			}
		})
	}
}

func TestOwnsJID(t *testing.T) {
	c := &GmailChannel{}
	tests := []struct {
		jid      string
		expected bool
	}{
		{"gmail:test@example.com", true},
		{"gmail:", true},
		{"whatsapp:1234567890", false},
		{"test@example.com", false},
		{"", false},
	}

	for _, tt := range tests {
		result := c.OwnsJID(tt.jid)
		if result != tt.expected {
			t.Errorf("OwnsJID(%q) = %v; want %v", tt.jid, result, tt.expected)
		}
	}
}
