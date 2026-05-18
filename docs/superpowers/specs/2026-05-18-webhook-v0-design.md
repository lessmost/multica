# V0 Outbound Webhook (DingTalk) — Design Spec

**Status:** Approved  
**Date:** 2026-05-18  
**Scope:** Simple env-var-driven DingTalk webhook for inbox notifications

---

## 1. Goal

Send a DingTalk markdown message to a configured webhook URL whenever an inbox notification is created. This gives teams real-time visibility in their group chat without polling or building custom integrations.

Non-goals for V0: per-workspace config, DB-backed endpoints, retry/circuit-breaker, delivery logs, HMAC signing, admin UI.

---

## 2. Architecture

```
notification_listeners → creates inbox_item → publishes inbox:new
                                                      │
                      ┌───────────────────────────────┼──────────────────┐
                      ▼                               ▼                  ▼
            WS listener (existing)         Webhook listener (new)     ...
            SendToUser(recipientID)         POST to WEBHOOK_URL
```

The webhook listener subscribes to `inbox:new` on the event bus — the same event that drives WebSocket delivery. It fires for every inbox item regardless of recipient (team-wide channel notification).

If `WEBHOOK_URL` is not configured, the listener is never registered — zero runtime overhead.

---

## 3. Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `WEBHOOK_URL` | No | `""` (disabled) | DingTalk custom robot webhook URL |
| `WEBHOOK_MSG_PREFIX` | No | `[Multica]` | Message prefix for DingTalk keyword security filter |

Both are read from the server environment at startup. No hot-reload — restart required to change.

---

## 4. Message Format

DingTalk markdown POST body:

```json
{
  "msgtype": "markdown",
  "markdown": {
    "title": "[Multica] 📋 Alice assigned MUL-42 to you",
    "text": "[Multica] 📋 **Alice assigned MUL-42 to you**\n\nFix the login timeout issue\n\n---\n⏰ 2026-05-18 10:30"
  }
}
```

### 4.1 Title

`{prefix} {emoji} {inbox_item.title}`

The `title` field appears in the notification preview on mobile. Keep it short.

### 4.2 Text (body)

```
{prefix} {emoji} **{inbox_item.title}**

{inbox_item.body}

---
⏰ {timestamp}
```

If `inbox_item.body` is empty, the body line is omitted.

### 4.3 Emoji Mapping

| Notification Type | Emoji |
|-------------------|-------|
| `issue_assigned`, `unassigned`, `assignee_changed` | 📋 |
| `mentioned` | 💬 |
| `new_comment` | 💭 |
| `status_changed` | 🔄 |
| `task_failed`, `agent_blocked` | 🚫 |
| `task_completed`, `agent_completed` | ✅ |
| `priority_changed`, `due_date_changed` | ⚠️ |
| `reaction_added` | 👍 |
| (unknown/other) | 🔔 |

---

## 5. Delivery Semantics

- **Non-blocking:** Dispatched in a separate goroutine. The event bus handler returns immediately.
- **Fire-and-forget:** No retry on failure. A single attempt per event.
- **Timeout:** Connect timeout 2s, total request timeout 5s.
- **Failure handling:** Log at WARN level with HTTP status and response body. Never panic, never block.
- **Concurrency:** No explicit limit for V0. Each `inbox:new` event spawns one goroutine. At realistic notification volumes (< 100/min) this is fine. V1 can add a worker pool if needed.

---

## 6. Event Payload Access

The `inbox:new` event payload shape (from `notifySubscribers` / `notifyDirect`):

```go
e.Payload = map[string]any{
    "item": map[string]any{
        "id":             "uuid",
        "workspace_id":   "uuid",
        "recipient_type": "member",
        "recipient_id":   "uuid",
        "type":           "issue_assigned",    // notification type
        "severity":       "attention",
        "issue_id":       "uuid",
        "title":          "Alice assigned MUL-42 to you",
        "body":           "Fix the login timeout issue",
        "actor_type":     "member",
        "actor_id":       "uuid",
        "details":        {},                  // JSON object
        "created_at":     "2026-05-18T10:30:00Z",
    },
}
```

The webhook formatter extracts `type`, `title`, `body`, and `created_at` from the item map.

---

## 7. Implementation

### 7.1 New Files

| Path | Purpose |
|------|---------|
| `server/internal/webhook/dingtalk.go` | DingTalk HTTP client + markdown message builder |
| `server/cmd/server/webhook_listeners.go` | `registerWebhookListeners(bus, cfg)` — subscribes to `inbox:new` |

### 7.2 Modified Files

| Path | Change |
|------|--------|
| `server/cmd/server/main.go` | Call `registerWebhookListeners` if `WEBHOOK_URL` is non-empty |

### 7.3 Package Design

```go
// server/internal/webhook/dingtalk.go

package webhook

type DingTalkClient struct {
    url       string
    prefix    string
    client    *http.Client
}

func NewDingTalkClient(url, prefix string) *DingTalkClient

// SendMarkdown posts a markdown message. Non-blocking caller is responsible
// for running this in a goroutine. Returns error for logging only.
func (d *DingTalkClient) SendMarkdown(ctx context.Context, title, text string) error
```

```go
// server/cmd/server/webhook_listeners.go

package main

func registerWebhookListeners(bus *events.Bus, client *webhook.DingTalkClient)
```

The listener extracts the inbox item from the event payload, maps the notification type to an emoji, formats title + text, and calls `client.SendMarkdown` in a goroutine.

---

## 8. Testing

- **Unit test** for `DingTalkClient.SendMarkdown`: mock HTTP server, verify request body shape.
- **Unit test** for emoji mapping and message formatting logic.
- **Integration:** Manual — configure a real DingTalk webhook URL in `.env`, trigger a notification, verify message appears in group chat.

---

## 9. Future Evolution (V1)

When the full spec (`docs/outbound-webhook-spec.md`) is needed:

- Replace env-var config with DB-backed `webhook_endpoint` table
- Add per-workspace CRUD API
- Move from fire-and-forget to a delivery worker with retry
- Add HMAC signing, circuit breaker, delivery logs
- Subscribe to domain events directly for richer payloads

The V0 `webhook` package provides a foundation — `DingTalkClient` becomes one of several formatters behind a `Sender` interface.
