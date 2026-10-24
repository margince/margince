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
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithEmailDrafter returns handlers whose draft endpoint uses the injected
// compose path.
func (h Handlers) WithEmailDrafter(drafter EmailDrafter) Handlers {
	h.emailDrafter = drafter
	return h
}

// DraftResult is one prepared draft with its provenance: whether a model
// produced it (the AI provenance notice) and which Voice DNA version styled it.
type DraftResult struct {
	Subject             string
	Body                string
	AIGenerated         bool
	AIProvenanceNotice  *string
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

// ProvenanceEmailDrafter is the richer drafting seam: same draft, plus the
// provenance the HTTP response stamps. A drafter that implements it is
// preferred over the plain EmailDrafter shape; the plain seam stays for
// consumers (agents, automation) whose surfaces carry text only.
type ProvenanceEmailDrafter interface {
	DraftEmailWithProvenance(ctx context.Context, anchor ids.UUID, steering draftfloor.Steering) (DraftResult, error)
}

// DraftEmail serves a reply draft for the anchored activity, with its
// provenance: the AI provenance notice, the voice version that styled it, and
// whether the sender's voice was lost on the way (voice_degraded). Drafting
// never sends; the send endpoint stays a separate consent-gated operation.
func (h Handlers) DraftEmail(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.DraftEmailJSONRequestBody
	if r.ContentLength > 0 && !httperr.Decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	result, err := h.prepareEmailDraft(ctx, ids.UUID(id), draftfloor.NewSteering(req.Intent, req.RewriteOf))
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
		AiDisclosure:        result.AIProvenanceNotice,
		VoiceProfileVersion: result.VoiceProfileVersion,
		DraftRef:            result.DraftRef,
		VoiceDegraded:       &result.VoiceDegraded,
	})
}

func (h Handlers) prepareEmailDraft(ctx context.Context, anchor ids.UUID, steering draftfloor.Steering) (DraftResult, error) {
	activity, err := h.store.GetActivityContent(ctx, ids.From[ids.ActivityKind](anchor), storekit.LiveOnly)
	if err != nil {
		return DraftResult{}, err
	}
	result, err := h.draftForActivity(ctx, anchor, activity, steering)
	if err != nil {
		return DraftResult{}, err
	}
	topic := ""
	if activity.Subject != nil {
		topic = *activity.Subject
	}
	result.Subject = ReplySubject(activity.Kind, topic, result.Subject)
	return result, nil
}

func (h Handlers) draftForActivity(ctx context.Context, anchor ids.UUID, activity crmcontracts.Activity, steering draftfloor.Steering) (DraftResult, error) {
	if provenance, ok := h.emailDrafter.(ProvenanceEmailDrafter); ok {
		return provenance.DraftEmailWithProvenance(ctx, anchor, steering)
	}
	if h.emailDrafter != nil {
		subject, body, err := h.emailDrafter.DraftEmail(ctx, anchor, steering.Intent)
		return DraftResult{Subject: subject, Body: body}, err
	}
	subject, body := h.store.FloorDraft(ctx, anchor, activity)
	return DraftResult{Subject: subject, Body: body}, nil
}
