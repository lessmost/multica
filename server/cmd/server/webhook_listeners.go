package main

import (
	"context"
	"log/slog"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/webhook"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func registerWebhookListeners(bus *events.Bus, client *webhook.DingTalkClient) {
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
