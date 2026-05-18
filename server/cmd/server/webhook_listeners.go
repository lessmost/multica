package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/webhook"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// webhookStatusLabels maps DB status values to human-readable labels for webhook messages.
var webhookStatusLabels = map[string]string{
	"backlog":     "Backlog",
	"todo":        "Todo",
	"in_progress": "In Progress",
	"in_review":   "In Review",
	"done":        "Done",
	"blocked":     "Blocked",
	"cancelled":   "Cancelled",
}

// webhookPriorityLabels maps DB priority values to human-readable labels for webhook messages.
var webhookPriorityLabels = map[string]string{
	"urgent": "Urgent",
	"high":   "High",
	"medium": "Medium",
	"low":    "Low",
	"none":   "No priority",
}

// enrichBodyFromDetails extracts structured transition info from the inbox
// item's details field and returns a human-readable body string.
func enrichBodyFromDetails(notifType string, details json.RawMessage) string {
	if len(details) == 0 || string(details) == "{}" {
		return ""
	}

	var m map[string]string
	if err := json.Unmarshal(details, &m); err != nil {
		return ""
	}

	from, to := m["from"], m["to"]
	if from == "" && to == "" {
		return ""
	}

	switch notifType {
	case "status_changed":
		fromLabel := webhookStatusLabels[from]
		if fromLabel == "" {
			fromLabel = from
		}
		toLabel := webhookStatusLabels[to]
		if toLabel == "" {
			toLabel = to
		}
		return fmt.Sprintf("%s → %s", fromLabel, toLabel)

	case "priority_changed":
		fromLabel := webhookPriorityLabels[from]
		if fromLabel == "" {
			fromLabel = from
		}
		toLabel := webhookPriorityLabels[to]
		if toLabel == "" {
			toLabel = to
		}
		return fmt.Sprintf("%s → %s", fromLabel, toLabel)

	case "due_date_changed":
		if from == "" {
			return fmt.Sprintf("Due: %s", to)
		}
		if to == "" {
			return fmt.Sprintf("Due date removed (was %s)", from)
		}
		return fmt.Sprintf("%s → %s", from, to)
	}

	return ""
}

// resolveDisplayName looks up the display name for a member (user) or agent by UUID.
func resolveDisplayName(ctx context.Context, queries *db.Queries, entityType, entityID string) string {
	if queries == nil || entityID == "" {
		return ""
	}
	uuid, err := util.ParseUUID(entityID)
	if err != nil {
		return ""
	}

	switch entityType {
	case "member":
		user, err := queries.GetUser(ctx, uuid)
		if err != nil {
			return ""
		}
		return user.Name
	case "agent":
		agent, err := queries.GetAgent(ctx, uuid)
		if err != nil {
			return ""
		}
		return agent.Name
	}
	return ""
}

// enrichAssignmentBody constructs a human-readable body for assignment notifications
// by resolving actor and assignee names from the database.
func enrichAssignmentBody(ctx context.Context, queries *db.Queries, notifType string, item map[string]any) string {
	details, ok := item["details"].(json.RawMessage)
	if !ok || len(details) == 0 || string(details) == "{}" {
		return ""
	}

	var m map[string]string
	if err := json.Unmarshal(details, &m); err != nil {
		return ""
	}

	newType, newID := m["new_assignee_type"], m["new_assignee_id"]
	prevType, prevID := m["prev_assignee_type"], m["prev_assignee_id"]

	newName := resolveDisplayName(ctx, queries, newType, newID)
	prevName := resolveDisplayName(ctx, queries, prevType, prevID)

	switch notifType {
	case "issue_assigned":
		if newName != "" {
			return fmt.Sprintf("Assigned to %s", newName)
		}
	case "unassigned":
		if prevName != "" {
			return fmt.Sprintf("%s unassigned", prevName)
		}
	case "assignee_changed":
		if prevName != "" && newName != "" {
			return fmt.Sprintf("%s → %s", prevName, newName)
		}
		if newName != "" {
			return fmt.Sprintf("Assigned to %s", newName)
		}
		if prevName != "" {
			return fmt.Sprintf("%s unassigned", prevName)
		}
	}
	return ""
}

func registerWebhookListeners(bus *events.Bus, queries *db.Queries, client *webhook.DingTalkClient) {
	bus.Subscribe(protocol.EventInboxNew, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		item, ok := payload["item"].(map[string]any)
		if !ok {
			return
		}

		notifType, _ := item["type"].(string)
		title, _ := item["title"].(string)
		var body string
		if bp, ok := item["body"].(*string); ok && bp != nil {
			body = *bp
		}

		// Enrich body from structured details when body is empty.
		if body == "" {
			switch notifType {
			case "issue_assigned", "unassigned", "assignee_changed":
				body = enrichAssignmentBody(context.Background(), queries, notifType, item)
			default:
				if details, ok := item["details"].(json.RawMessage); ok {
					body = enrichBodyFromDetails(notifType, details)
				}
			}
		}

		if title == "" {
			return
		}

		mdTitle, mdText := webhook.FormatMarkdown(client.Prefix(), notifType, title, body)

		go func() {
			if err := client.SendMarkdown(context.Background(), mdTitle, mdText); err != nil {
				slog.Warn("webhook delivery failed",
					"error", err,
					"notif_type", notifType,
				)
			}
		}()
	})
}
