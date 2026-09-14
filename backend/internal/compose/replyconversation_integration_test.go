// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func logReplyMessage(ctx context.Context, t *testing.T, e *integration.Env, thread, subject, body string, offset time.Duration) crmcontracts.Activity {
	t.Helper()
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC).Add(offset)
	direction := "outbound"
	row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, ThreadKey: thread,
		Direction: &direction, OccurredAt: &at, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestReplyDraftUsesTheSelectedMessageAndReadableThreadContext(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	anchor := logReplyMessage(ctx, t, e, "pricing", "Re: RE: Pricing", "Selected proposal", 0)
	logReplyMessage(ctx, t, e, "pricing", "Delivery", "Agreed delivery window", time.Hour)
	logReplyMessage(ctx, t, e, "unrelated", "Unrelated", "Other customer secret", 2*time.Hour)
	colleague := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	held := logReplyMessage(colleague, t, e, "pricing", "Private planning", "Hidden colleague note", 2*time.Hour)
	_, err := e.Activities.SetAudience(colleague, ids.From[ids.ActivityKind](ids.UUID(held.Id)), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Wrong target","body":"Could we confirm the proposal?"}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	result, err := drafter.DraftEmailWithProvenance(ctx, ids.UUID(anchor.Id), "Ask for confirmation")
	if err != nil {
		t.Fatal(err)
	}
	if result.Subject != "Re: Pricing" {
		t.Fatalf("reply subject = %q", result.Subject)
	}
	if len(brain.request.Messages) != 1 {
		t.Fatalf("model messages = %d", len(brain.request.Messages))
	}
	content := brain.request.Messages[0].Content
	if !strings.Contains(content, "Selected proposal") || !strings.Contains(content, "Agreed delivery window") ||
		strings.Contains(content, "Other customer secret") || strings.Contains(content, "Hidden colleague note") ||
		!strings.Contains(content, "later than the selected message") {
		t.Fatalf("selected message and thread isolation lost: %s", content)
	}
	if strings.Count(content, "Selected proposal") != 1 || strings.Contains(content, `"thread":"inbound_mail"`) {
		t.Fatalf("outbound anchor duplicated or treated as inbound: %s", content)
	}
}

// A text-only adapter is still governed by the HTTP surface's selected subject.
type plainReplyStub struct{}

func (plainReplyStub) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "Another subject", "An editable draft", nil
}

func TestReplyEndpointNormalizesTheSubjectForEveryDraftAdapter(t *testing.T) {
	e := integration.Setup(t)
	anchor := logReplyMessage(e.Admin(), t, e, "", "Re: Pricing", "Selected proposal", 0)
	for name, drafter := range map[string]activities.EmailDrafter{
		"deterministic": nil,
		"plain":         plainReplyStub{},
		"provenance":    newReplyDrafter(e.Pool, &replyBrainStub{response: model.Response{Text: `{"subject":"Another subject","body":"Please confirm."}`}}, slog.New(slog.DiscardHandler)),
	} {
		t.Run(name, func(t *testing.T) {
			handler := activities.NewHandlers(e.DB()).WithEmailDrafter(drafter)
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/draft-email", nil).WithContext(e.Admin())
			handler.DraftEmail(response, request, anchor.Id)
			var draft crmcontracts.EmailDraft
			if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || draft.Subject != "Re: Pricing" || draft.Body == "" {
				t.Fatalf("reply endpoint: status %d, draft %+v", response.Code, draft)
			}
		})
	}
}
