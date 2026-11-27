// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refusingMeetingBrain labels every message it is asked about "meeting" and
// judges every one a refusal, so a test can see which verdicts the pass keeps.
type refusingMeetingBrain struct{ prompts []string }

func (b *refusingMeetingBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	b.prompts = append(b.prompts, req.Messages[0].Content)
	marker, ok := promptfence.MarkerIn(req.System)
	if !ok {
		return model.Response{}, fmt.Errorf("classify prompt declared no data boundary: %q", req.System)
	}
	var results []map[string]any
	for _, span := range strings.Split(req.Messages[0].Content, "<"+marker+` source_id="`)[1:] {
		ref, _, _ := strings.Cut(span, `"`)
		results = append(results, map[string]any{"id": ref, "label": "meeting", "confidence": 0.95, "reply": "negative"})
	}
	payload, err := json.Marshal(map[string]any{"results": results})
	return model.Response{Text: string(payload)}, err
}

// A chat message a connector captured is labelled like mail, so a chat asking
// for a time reaches the worklist as a meeting. It takes no reply verdict: one
// chat line read without the line it answers cannot carry a refusal that may
// close the deal as lost.
func TestAConnectorCapturedChatMessageIsLabelledAndTakesNoReplyVerdict(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	chat := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, channel_provider, direction, body, occurred_at, source, source_system, source_id,
		                      captured_by, thread_key)
		VALUES ($1, 'message', 'telegram', 'inbound', 'Are you free at 8:00 PM Monday?', now(), 'dispact', 'dispact', $2,
		        'connector:telegram:'||$3::text, 'telegram:chat-1')`,
		chat, "chat-"+chat.String(), e.Rep1)
	// The control: the same refusal on an inbound mail is stored. So the chat's
	// missing verdict comes from the chat rule.
	mail := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, direction, subject, body, occurred_at, source, source_system, source_id, captured_by)
		VALUES ($1, 'email', 'inbound', 'Re: the offer', 'We will not go ahead.', now(), 'gmail', 'gmail', $2,
		        'connector:gmail')`,
		mail, "mail-"+mail.String())

	// A filed chat screenshot, as ChatCaptureFile writes it: a message on the
	// chat's channel, captured by the attachment reader and not a connector.
	screenshot := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, channel_provider, body, occurred_at, source, source_system, source_id, captured_by)
		VALUES ($1, 'message', 'whatsapp', 'Lena: are we still on for Thursday?', now(), $2, $2, $3,
		        'agent:attachment-extractor')`,
		screenshot, activities.ChatCaptureSourceSystem, screenshot.String())

	brain := &refusingMeetingBrain{}
	classifier := compose.NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler))
	// The system principal the capture_classify job binds (capturejobs.go).
	// Without it a reply verdict write fails, and the test could not see one.
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:capture_classify",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("running the pass: %v", err)
	}
	if len(brain.prompts) == 0 || !strings.Contains(brain.prompts[0], "channel: telegram\ninbound: yes\n") {
		t.Fatalf("the chat was not offered to the model with its channel line: %q", brain.prompts)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity
		 WHERE id = $1 AND capture_label = 'meeting' AND reply_verdict IS NULL`, chat); n != 1 {
		t.Error("the chat message is not labelled meeting with no reply verdict")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND capture_label IS NULL`, screenshot); n != 1 {
		t.Error("a filed chat screenshot was labelled, but the pass reads only connector capture")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND reply_verdict = 'negative'`, mail); n != 1 {
		t.Error("the inbound mail took no reply verdict, so this pass stores none at all")
	}
}
