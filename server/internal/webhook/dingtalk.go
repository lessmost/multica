package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// DingTalkClient sends markdown messages to a DingTalk webhook endpoint.
type DingTalkClient struct {
	url    string
	prefix string
	client *http.Client
}

// NewDingTalkClient creates a client with a 5-second timeout.
func NewDingTalkClient(url, prefix string) *DingTalkClient {
	return &DingTalkClient{
		url:    url,
		prefix: prefix,
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: 2 * time.Second,
			},
		},
	}
}

// Prefix returns the configured message prefix.
func (d *DingTalkClient) Prefix() string {
	return d.prefix
}

// SendMarkdown posts a markdown message to the DingTalk webhook.
func (d *DingTalkClient) SendMarkdown(ctx context.Context, title, text string) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  text,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
