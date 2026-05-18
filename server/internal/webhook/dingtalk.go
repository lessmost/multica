package webhook

import (
	"fmt"
	"time"
)

func emojiForType(notifType string) string {
	switch notifType {
	case "issue_assigned", "unassigned", "assignee_changed":
		return "📋"
	case "mentioned":
		return "💬"
	case "new_comment":
		return "💭"
	case "status_changed":
		return "🔄"
	case "task_failed", "agent_blocked":
		return "🚫"
	case "task_completed", "agent_completed":
		return "✅"
	case "priority_changed", "due_date_changed":
		return "⚠️"
	case "reaction_added":
		return "👍"
	default:
		return "🔔"
	}
}

// FormatMarkdown builds a DingTalk markdown title and text from notification fields.
func FormatMarkdown(prefix, notifType, title, body string) (mdTitle, mdText string) {
	emoji := emojiForType(notifType)
	mdTitle = fmt.Sprintf("%s %s %s", prefix, emoji, title)

	timestamp := time.Now().Format("2006-01-02 15:04")
	if body != "" {
		mdText = fmt.Sprintf("%s %s **%s**\n\n%s\n\n---\n⏰ %s", prefix, emoji, title, body, timestamp)
	} else {
		mdText = fmt.Sprintf("%s %s **%s**\n\n---\n⏰ %s", prefix, emoji, title, timestamp)
	}
	return mdTitle, mdText
}
