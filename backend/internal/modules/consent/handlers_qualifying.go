// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordQualifyingEvent records the assertion under the contact's write authority.
func (h Handlers) RecordQualifyingEvent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	in, ok := decodeQualifyingEvent(w, r)
	if !ok {
		return
	}
	recorded, err := h.store.RecordQualifyingEvent(r.Context(), pathID[ids.ContactKind](id), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	writeQualifyingEventRecord(w, r, recorded)
}

// RecordLeadQualifyingEvent uses the lead's own write authority for the exchange.
func (h Handlers) RecordLeadQualifyingEvent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	in, ok := decodeQualifyingEvent(w, r)
	if !ok {
		return
	}
	recorded, err := h.store.RecordLeadQualifyingEvent(r.Context(), pathID[ids.LeadKind](id), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	writeQualifyingEventRecord(w, r, recorded)
}

func decodeQualifyingEvent(w http.ResponseWriter, r *http.Request) (RecordQualifyingEventInput, bool) {
	var req crmcontracts.RecordQualifyingEventRequest
	if !httperr.Decode(w, r, &req) {
		return RecordQualifyingEventInput{}, false
	}
	in := RecordQualifyingEventInput{
		Kind: string(req.Kind), Note: req.Note, OccurredAt: req.OccurredAt,
		AsksForQuote: req.AsksForQuote != nil && *req.AsksForQuote,
	}
	if deal := idArg[ids.DealKind](req.AboutDeal); deal != nil {
		in.AboutDeal = deal.UUID
	}
	return in, true
}

func writeQualifyingEventRecord(w http.ResponseWriter, r *http.Request, recorded QualifyingEvent) {
	recordedID, err := ids.Parse(recorded.ID)
	if err != nil {
		writeConsentErr(w, r, fmt.Errorf("consent: the recorded exchange has no id: %w", err))
		return
	}
	out := crmcontracts.QualifyingEventRecord{
		Id: openapi_types.UUID(recordedID), Kind: crmcontracts.QualifyingEventRecordKind(recorded.Kind),
		OccurredAt: recorded.OccurredAt, AsksForQuote: recorded.AsksForQuote, Retracted: recorded.Retracted,
	}
	if recorded.Note != "" {
		note := recorded.Note
		out.Note = &note
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// RetractQualifyingEvent keeps retraction under the same contact authority as recording.
func (h Handlers) RetractQualifyingEvent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, eventID openapi_types.UUID) {
	if err := h.store.RetractQualifyingEvent(r.Context(), pathID[ids.ContactKind](id), ids.UUID(eventID)); err != nil {
		writeConsentErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RetractLeadQualifyingEvent keeps retraction under the same lead authority as recording.
func (h Handlers) RetractLeadQualifyingEvent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, eventID openapi_types.UUID) {
	if err := h.store.RetractLeadQualifyingEvent(r.Context(), pathID[ids.LeadKind](id), ids.UUID(eventID)); err != nil {
		writeConsentErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
