// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// WithScheduleTimer returns handlers that can defer a send. Compose calls this;
// without it a scheduling request refuses rather than accept a moment nothing
// will wake at.
func (h Handlers) WithScheduleTimer(timer ScheduleTimer) Handlers {
	h.timer = timer
	return h
}

// WithHeldNotifier returns handlers whose store raises the inbox card when a
// message is stopped. The notifier lives on the STORE because that is where a
// hold is written, and the handlers carry their own store instance.
func (h Handlers) WithHeldNotifier(notifier HeldNotifier) Handlers {
	h.store = h.store.WithHeldNotifier(notifier)
	return h
}

// ListScheduledSends answers with the caller's own waiting messages.
func (h Handlers) ListScheduledSends(w http.ResponseWriter, r *http.Request, params crmcontracts.ListScheduledSendsParams) {
	status := ""
	if params.Status != nil {
		status = string(*params.Status)
	}
	rows, err := h.store.ListScheduledSends(r.Context(), status)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := make([]crmcontracts.ScheduledSend, 0, len(rows))
	for _, row := range rows {
		out = append(out, scheduledSendResponse(row))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetScheduledSend answers with one of the caller's waiting messages.
func (h Handlers) GetScheduledSend(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	row, err := h.store.GetScheduledSend(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scheduledSendResponse(row))
}

// RescheduleScheduledSend moves a waiting message to a different moment.
func (h Handlers) RescheduleScheduledSend(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RescheduleScheduledSendParams) {
	var req crmcontracts.RescheduleSendRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if version == nil {
		// Required rather than optional: two surfaces moving one message must
		// not silently resolve to whichever wrote last.
		httperr.Write(w, r, httperr.Validation("If-Match",
			"missing_if_match", "moving a scheduled send needs the version you last read"))
		return
	}
	row, err := h.store.RescheduleScheduledSend(r.Context(), ids.UUID(id), SendSchedule{
		At: req.ScheduledAt,
		TZ: req.ScheduledTz,
	}, *version, h.timer)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, scheduledSendResponse(row))
}

// CancelScheduledSend withdraws a waiting message.
func (h Handlers) CancelScheduledSend(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.CancelScheduledSend(r.Context(), ids.UUID(id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scheduledSendResponse renders one row onto the wire.
func scheduledSendResponse(row ScheduledSend) crmcontracts.ScheduledSend {
	out := crmcontracts.ScheduledSend{
		Id:          openapi_types.UUID(row.ID),
		Status:      crmcontracts.ScheduledSendStatus(row.Status),
		ScheduledAt: row.ScheduledAt,
		ScheduledTz: row.ScheduledTZ,
		Subject:     row.Subject,
		// The To LINE, derived the way the send derives it. row.Recipients is
		// the merged consent superset — every To, Cc AND Bcc — so rendering it
		// raw would put blind copies in a field the rep reads as visible.
		To:        emailList(toRecipients(row.Recipients, row.Cc, row.Bcc)),
		Version:   row.Version,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if cc := emailList(row.Cc); len(cc) > 0 {
		out.Cc = &cc
	}
	if bcc := emailList(row.Bcc); len(bcc) > 0 {
		out.Bcc = &bcc
	}
	if row.Body != "" {
		body := row.Body
		out.Body = &body
	}
	if row.Anchor.UUID != (ids.UUID{}) {
		anchor := openapi_types.UUID(row.Anchor.UUID)
		out.AnchorActivityId = &anchor
	}
	// The caller's own frozen input, echoed to its author and nobody else: the
	// read is scoped to scheduled_by, and every door that ACTS on these links —
	// the preview, the fire — re-probes each one through the caller's row scope
	// before the engine sees it. A record the scheduler can no longer read is
	// refused there, not hidden here, because a list that silently dropped one
	// would have the preview ask a narrower question than the fire.
	if len(row.Links) > 0 {
		links := linksOnWire(row.Links)
		out.Links = &links
	}
	// Only a category the wire's enum publishes. The store type also names the
	// five controller-only categories, which the door refuses and the fire
	// refuses again; a row that carried one anyway must not reach a client as
	// a value outside its own contract.
	if row.Context.Valid() && !row.Context.ServesTheSubject() {
		claimed := crmcontracts.CommunicationContext(row.Context)
		out.CommunicationContext = &claimed
	}
	if row.MarketingPurpose != "" {
		purpose := row.MarketingPurpose
		out.MarketingPurpose = &purpose
	}
	if row.ConsentPurpose != "" {
		purpose := row.ConsentPurpose
		out.ConsentPurpose = &purpose
	}
	if evidence, named := evidenceOnWire(row.Evidence); named {
		out.Evidence = &evidence
	}
	if row.ActivityID != (ids.UUID{}) {
		activityID := openapi_types.UUID(row.ActivityID)
		out.ActivityId = &activityID
	}
	if row.HeldReason != "" {
		reason := crmcontracts.ScheduledSendHeldReason(row.HeldReason)
		out.HeldReason = &reason
	}
	return out
}

// linksOnWire is linkInputsOf read backwards: the frozen records a scheduled
// message names, in the shape a caller supplied them.
func linksOnWire(links []ActivityLinkInput) []crmcontracts.ActivityLinkInput {
	out := make([]crmcontracts.ActivityLinkInput, 0, len(links))
	for _, l := range links {
		out = append(out, crmcontracts.ActivityLinkInput{
			EntityType: crmcontracts.ActivityLinkInputEntityType(l.EntityType),
			EntityId:   openapi_types.UUID(l.EntityID),
		})
	}
	return out
}

// evidenceOnWire is evidenceFrom read backwards. The second value says whether
// anything was named, so a message with no evidence omits the block rather than
// sending six nulls a client would have to read as "none".
func evidenceOnWire(e commsauthz.Evidence) (crmcontracts.CommunicationEvidence, bool) {
	var out crmcontracts.CommunicationEvidence
	named := false
	name := func(into **openapi_types.UUID, id ids.UUID) {
		if id == (ids.UUID{}) {
			return
		}
		wire := openapi_types.UUID(id)
		*into = &wire
		named = true
	}
	name(&out.ActivityId, e.ActivityID)
	name(&out.DealId, e.DealID)
	name(&out.InvoiceId, e.InvoiceID)
	name(&out.ContractId, e.ContractID)
	name(&out.ConsentEventId, e.ConsentEventID)
	name(&out.BasisId, e.BasisID)
	return out, named
}

// emailList renders stored addresses onto the wire's email type.
func emailList(addresses []string) []openapi_types.Email {
	out := make([]openapi_types.Email, 0, len(addresses))
	for _, addr := range addresses {
		out = append(out, openapi_types.Email(addr))
	}
	return out
}

// scheduleFrom reads the optional scheduling fields both send requests carry.
// Nil means send now, which is what every caller before ADR-0104 meant.
//
// One spelling for both surfaces, for the same reason sendInputFrom is: a
// second copy would eventually validate one of them differently, and "send
// later" would mean something subtly different depending on which button the
// rep pressed.
func scheduleFrom(at *time.Time, tz *string) (*SendSchedule, error) {
	if at == nil && tz == nil {
		// Neither field sent: send now, which is what every caller before
		// ADR-0104 meant and what a composer that never shows the control
		// still means. A nil schedule is the ANSWER here, not a missing one.
		return nil, nil //nolint:nilnil // "send now" IS the answer for an optional schedule, not a missing value.
	}
	if at == nil {
		return nil, &InvalidScheduleError{Field: FieldScheduledAt, Reason: "is required when a zone is given"}
	}
	if tz == nil {
		return nil, &InvalidScheduleError{Field: FieldScheduledTZ, Reason: "is required when a moment is given"}
	}
	return &SendSchedule{At: *at, TZ: *tz}, nil
}
