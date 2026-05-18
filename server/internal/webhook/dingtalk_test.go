package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestSendMarkdown(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	client := NewDingTalkClient(srv.URL, "[Test]")
	err := client.SendMarkdown(context.Background(), "Test Title", "Test Body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received["msgtype"] != "markdown" {
		t.Errorf("expected msgtype=markdown, got %v", received["msgtype"])
	}
	md, ok := received["markdown"].(map[string]any)
	if !ok {
		t.Fatal("missing markdown field")
	}
	if md["title"] != "Test Title" {
		t.Errorf("unexpected title: %v", md["title"])
	}
	if md["text"] != "Test Body" {
		t.Errorf("unexpected text: %v", md["text"])
	}
}

func TestSendMarkdownTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	client := NewDingTalkClient(srv.URL, "[Test]")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.SendMarkdown(ctx, "Title", "Body")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

func TestSendMarkdownNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := NewDingTalkClient(srv.URL, "[Test]")
	err := client.SendMarkdown(context.Background(), "Title", "Body")
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}
