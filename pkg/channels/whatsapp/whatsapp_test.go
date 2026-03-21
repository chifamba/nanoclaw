package whatsapp

import "testing"

func TestOwnsJID(t *testing.T) {
	c := &WhatsAppChannel{}
	tests := []struct {
		jid      string
		expected bool
	}{
		{"1234567890@s.whatsapp.net", true},
		{"1234567890@g.us", true},
		{"1234567890@c.us", false},
		{"gmail:test@example.com", false},
		{"random string", false},
		{"", false},
	}

	for _, tt := range tests {
		result := c.OwnsJID(tt.jid)
		if result != tt.expected {
			t.Errorf("OwnsJID(%q) = %v; want %v", tt.jid, result, tt.expected)
		}
	}
}
