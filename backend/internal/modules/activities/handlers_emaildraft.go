// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The draft half of the email surface: the seams a compose path plugs into,
// the provenance a served draft carries, and the endpoint that returns one.
// Sending lives in handlers_email.go — drafting only proposes text and the
// send endpoint remains a separate consent-gated operation.

import (
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithEmailDrafter returns handlers whose draft endpoint uses the injected
// compose path.
func (h Handlers) WithEmailDrafter(drafter EmailDrafter) Handlers {
	h.emailDrafter = drafter
	return h
}

// DraftResult is one prepared draft with its provenance: whether a model
// produced it (Art. 50 disclosure) and which Voice DNA version styled it.
type DraftResult struct {
	Subject             string
	Body                string
	AIGenerated         bool
	AIDisclosure        *string
	VoiceProfileVersion *int
	// DraftRef identifies this served voice draft for learning feedback
	// (rejectVoiceDraft); nil when no voice profile styled it.
	DraftRef *string
	// VoiceDegraded says the sender's voice could not even be looked up, so
	// nobody knows whether a profile should have styled this draft. It is
	// distinct from a nil VoiceProfileVersion, which also covers the ordinary
	// no-profile case, and it is the one state the sender cannot detect by
	// reading the text.
	VoiceDegraded bool
}

// FirstEmailDrafter drafts a message that OPENS a conversation, where
// EmailDrafter answers one.
//
// It is a second method rather than an anchor that may be zero, because the two
// take different evidence and a nil anchor would let a caller ask the reply
// path a question it has no answer for. A reply reads the activity it answers —
// its subject, its body, how long ago it arrived, who sent it. A first message
// has none of that in existence: the caller's intent is the only subject
// material there is, and everything else the draft is written from (the
// language, the clock, who is signing) is server-derived either way.
//
// A drafter that implements only EmailDrafter leaves the first-message path on
// the deterministic floor, which is what a deployment running no model gets on
// both paths.
type FirstEmailDrafter interface {
	DraftFirstEmail(ctx context.Context, intent string) (string, string, error)
}

// ProvenanceEmailDrafter is the richer drafting seam: same draft, plus the
// provenance the HTTP response stamps. A drafter that implements it is
// preferred over the plain EmailDrafter shape; the plain seam stays for
// consumers (agents, automation) whose surfaces carry text only.
type ProvenanceEmailDrafter interface {
	DraftEmailWithProvenance(ctx context.Context, anchor ids.UUID, intent string) (DraftResult, error)
}

func (h Handlers) DraftEmail(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req struct {
		Intent *string `json:"intent"`
	}
	if r.ContentLength > 0 && !httperr.Decode(w, r, &req) {
		return
	}
	intent := ""
	if req.Intent != nil {
		intent = *req.Intent
	}
	ctx := r.Context()
	result, err := h.prepareEmailDraft(ctx, ids.UUID(id), intent)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}

	replyTo := openapi_types.UUID(ids.UUID(id))
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.EmailDraft{
		Subject:             result.Subject,
		Body:                result.Body,
		To:                  h.replyAddresses(ctx, ids.From[ids.ActivityKind](ids.UUID(id))),
		InReplyToActivityId: &replyTo,
		AiGenerated:         &result.AIGenerated,
		AiDisclosure:        result.AIDisclosure,
		VoiceProfileVersion: result.VoiceProfileVersion,
		DraftRef:            result.DraftRef,
		VoiceDegraded:       &result.VoiceDegraded,
	})
}

func (h Handlers) prepareEmailDraft(ctx context.Context, anchor ids.UUID, intent string) (DraftResult, error) {
	if provenance, ok := h.emailDrafter.(ProvenanceEmailDrafter); ok {
		return provenance.DraftEmailWithProvenance(ctx, anchor, intent)
	}
	if h.emailDrafter != nil {
		subject, body, err := h.emailDrafter.DraftEmail(ctx, anchor, intent)
		return DraftResult{Subject: subject, Body: body}, err
	}
	activity, err := h.store.GetActivityContent(ctx, ids.From[ids.ActivityKind](anchor), storekit.LiveOnly)
	if err != nil {
		return DraftResult{}, err
	}
	answering := DraftContext{
		Band:      convstate.BandFresh,
		Threaded:  IsMailThread(activity.Kind, activity.Direction),
		Recipient: h.store.GreetingName(ctx, ids.From[ids.ActivityKind](anchor)),
	}
	if activity.Subject != nil {
		answering.Topic = *activity.Subject
	}
	if activity.Body != nil {
		answering.Body = *activity.Body
	}
	subject, body := DeterministicEmailDraft(answering, intent)
	return DraftResult{Subject: subject, Body: body}, nil
}
