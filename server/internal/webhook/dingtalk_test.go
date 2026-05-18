package webhook

import (
	"strings"
	"testing"
)

func TestEmojiForType(t *testing.T) {
	tests := []struct {
		notifType string
		want      string
	}{
		{"issue_assigned", "📋"},
		{"unassigned", "📋"},
		{"assignee_changed", "📋"},
		{"mentioned", "💬"},
		{"new_comment", "💭"},
		{"status_changed", "🔄"},
		{"task_failed", "🚫"},
		{"agent_blocked", "🚫"},
		{"task_completed", "✅"},
		{"agent_completed", "✅"},
		{"priority_changed", "⚠️"},
		{"due_date_changed", "⚠️"},
		{"reaction_added", "👍"},
		{"unknown_type", "🔔"},
	}
	for _, tt := range tests {
		if got := emojiForType(tt.notifType); got != tt.want {
			t.Errorf("emojiForType(%q) = %q, want %q", tt.notifType, got, tt.want)
		}
	}
}

func TestFormatMarkdown(t *testing.T) {
	title, text := FormatMarkdown("[Multica]", "issue_assigned", "Alice assigned MUL-42 to you", "Fix the login timeout")
	if title != "[Multica] 📋 Alice assigned MUL-42 to you" {
		t.Errorf("unexpected title: %s", title)
	}
	if !strings.Contains(text, "[Multica] 📋 **Alice assigned MUL-42 to you**") {
		t.Errorf("text missing formatted title: %s", text)
	}
	if !strings.Contains(text, "Fix the login timeout") {
		t.Errorf("text missing body: %s", text)
	}
	if !strings.Contains(text, "⏰") {
		t.Errorf("text missing timestamp: %s", text)
	}
}

func TestFormatMarkdownEmptyBody(t *testing.T) {
	_, text := FormatMarkdown("[Test]", "new_comment", "New comment on MUL-1", "")
	if strings.Contains(text, "\n\n\n") {
		t.Errorf("text has extra blank lines when body is empty: %q", text)
	}
}
