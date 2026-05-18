package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEnrichBodyFromDetails(t *testing.T) {
	tests := []struct {
		name      string
		notifType string
		details   string
		want      string
	}{
		{
			name:      "status_changed with known statuses",
			notifType: "status_changed",
			details:   `{"from":"todo","to":"in_progress"}`,
			want:      "Todo → In Progress",
		},
		{
			name:      "status_changed with unknown status falls back to raw",
			notifType: "status_changed",
			details:   `{"from":"custom_status","to":"done"}`,
			want:      "custom_status → Done",
		},
		{
			name:      "priority_changed",
			notifType: "priority_changed",
			details:   `{"from":"low","to":"urgent"}`,
			want:      "Low → Urgent",
		},
		{
			name:      "due_date_changed with both dates",
			notifType: "due_date_changed",
			details:   `{"from":"2026-05-01","to":"2026-05-20"}`,
			want:      "2026-05-01 → 2026-05-20",
		},
		{
			name:      "due_date_changed set from empty",
			notifType: "due_date_changed",
			details:   `{"from":"","to":"2026-06-01"}`,
			want:      "Due: 2026-06-01",
		},
		{
			name:      "due_date_changed removed",
			notifType: "due_date_changed",
			details:   `{"from":"2026-05-01","to":""}`,
			want:      "Due date removed (was 2026-05-01)",
		},
		{
			name:      "empty details object",
			notifType: "status_changed",
			details:   `{}`,
			want:      "",
		},
		{
			name:      "unhandled notif type returns empty",
			notifType: "issue_assigned",
			details:   `{"from":"x","to":"y"}`,
			want:      "",
		},
		{
			name:      "invalid JSON returns empty",
			notifType: "status_changed",
			details:   `not json`,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enrichBodyFromDetails(tt.notifType, json.RawMessage(tt.details))
			if got != tt.want {
				t.Errorf("enrichBodyFromDetails(%q, %s) = %q, want %q",
					tt.notifType, tt.details, got, tt.want)
			}
		})
	}
}

func TestEnrichAssignmentBodyNoQueries(t *testing.T) {
	// Without a real DB, enrichAssignmentBody should gracefully return empty
	// when it can't resolve names. We test the parsing/logic path here.
	tests := []struct {
		name      string
		notifType string
		details   string
		wantEmpty bool
	}{
		{
			name:      "empty details",
			notifType: "issue_assigned",
			details:   `{}`,
			wantEmpty: true,
		},
		{
			name:      "invalid JSON",
			notifType: "issue_assigned",
			details:   `bad`,
			wantEmpty: true,
		},
		{
			name:      "missing assignee IDs",
			notifType: "issue_assigned",
			details:   `{"new_assignee_type":"member"}`,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := map[string]any{
				"details": json.RawMessage(tt.details),
			}
			// Pass nil queries — all lookups will fail gracefully.
			got := enrichAssignmentBody(context.Background(), nil, tt.notifType, item)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty body, got %q", got)
			}
		})
	}
}
