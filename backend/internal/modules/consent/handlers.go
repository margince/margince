// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the module's transport slice; compose embeds it so the
// generated consent stubs are shadowed by real code.
type Handlers struct {
	store  *Store
	eraser Eraser
	// The Art. 15 assembler, injected the same way and for the same reason as
	// the eraser: answering an access request means producing the package, not
	// marking a row done.
	assembler SubjectAccessAssembler
}

// Eraser is the erase-path seam (compose injects the real one): DSR
// fulfillment of an erasure request EXECUTES the erasure, it never just
// marks a row done.
type Eraser interface {
	ErasePerson(ctx context.Context, personID ids.UUID, reason string) error
}

// SubjectAccessAssembler is the Art. 15 read seam (compose injects the real
// one). It is a privileged read that deliberately crosses the caller's row
// scope, because Art. 15 owes the subject everything held rather than the slice
// one rep may see — privacy.AssembleSAR carries the checks that make that safe,
// and this interface exists so consent can reach it without importing a sibling.
//
// It answers the serialized package rather than a struct, because the shape is
// privacy's to own: the package gains a section every time a new table starts
// holding subject data, and a type mirrored here would be a second declaration
// of what Art. 15 owes, drifting one release behind the one the gate checks.
type SubjectAccessAssembler interface {
	AssemblePackage(ctx context.Context, personID ids.UUID) ([]byte, error)
}

// NewHandlers wires the transport over the installation-bound pool.
func NewHandlers(db *database.DB) Handlers {
	return Handlers{store: NewStore(db)}
}

// WithInstallationName injects the label the public preference page shows
// as the sender of the mail it is answering. Compose supplies identity's
// reader; a module never imports a sibling.
func (h Handlers) WithInstallationName(r InstallationNameReader) Handlers {
	h.store = h.store.WithInstallationName(r)
	return h
}

// WithEraser returns a copy wired to the erase path.
func (h Handlers) WithEraser(e Eraser) Handlers {
	h.eraser = e
	return h
}

// WithSubjectAccessAssembler returns a copy wired to the Art. 15 export.
func (h Handlers) WithSubjectAccessAssembler(a SubjectAccessAssembler) Handlers {
	h.assembler = a
	return h
}

func (h Handlers) ListConsentPurposes(w http.ResponseWriter, r *http.Request, _ crmcontracts.ListConsentPurposesParams) {
	purposes, err := h.store.ListPurposes(r.Context())
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	data := make([]crmcontracts.ConsentPurpose, 0, len(purposes))
	for _, p := range purposes {
		data = append(data, wirePurpose(p))
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"page": crmcontracts.PageInfo{HasMore: false},
	})
}

func (h Handlers) CreateConsentPurpose(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.CreateConsentPurposeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	requiresDOI := req.RequiresDoubleOptIn != nil && *req.RequiresDoubleOptIn
	purpose, err := h.store.CreatePurpose(r.Context(), req.Key, req.Label, requiresDOI)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wirePurpose(purpose))
}

func (h Handlers) GetPersonConsent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	states, events, err := h.store.PersonConsent(r.Context(), pathID[ids.PersonKind](id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	wireStates := make([]crmcontracts.PersonConsentState, 0, len(states))
	for _, st := range states {
		wireStates = append(wireStates, wireState(st))
	}
	wireEvents := make([]crmcontracts.ConsentEvent, 0, len(events))
	for _, ev := range events {
		wireEvents = append(wireEvents, wireEvent(ev))
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"state": wireStates, "events": wireEvents})
}

func (h Handlers) RecordConsent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.RecordConsentParams) {
	var req crmcontracts.RecordConsentRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	state, err := h.store.Record(r.Context(), RecordInput{
		PersonID:    pathID[ids.PersonKind](id),
		PurposeID:   pathID[ids.PurposeKind](req.PurposeId),
		NewState:    string(req.NewState),
		LawfulBasis: req.LawfulBasis,
		Source:      req.Source,
	})
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireState(state))
}

// IssueDoubleOptIn implements (POST /people/{id}/consent/double-opt-in) by
// refusing, and mints nothing.
//
// A double opt-in is evidence only because the data subject completed it from
// their own mailbox. This endpoint returned the plaintext token to the
// authenticated operator, who could paste it back into RecordConsent — so the
// round trip the proof stands on could be closed without the subject's mailbox
// ever taking part, and the resulting consent_event recorded a confirmation
// that had not happened. Nothing here delivered the token either: the deliver
// flag reached an audit payload and no mailer.
//
// It refuses rather than being deleted because the operation is in the public
// contract and a caller deserves an answer that says why. It returns when the
// confirmation mail has a durable path to the subject; until then marketing
// opt-in is captured through the confirm-details link, which mails a single-use
// link to the person's own live primary address.
func (h Handlers) IssueDoubleOptIn(w http.ResponseWriter, r *http.Request, _ crmcontracts.Id) {
	var req crmcontracts.IssueDoubleOptInJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	// The body id is still probed before the refusal. A caller who omitted the
	// purpose has a malformed request whichever way this endpoint answers, and
	// requiredbodyids holds every required id to a probe — an endpoint that
	// refuses for its own reasons must not become the one place a missing id
	// goes unnamed.
	if err := httperr.RequireBodyID(purposeIDField, ids.UUID(req.PurposeId)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.Write(w, r, fmt.Errorf(
		"a double opt-in link can only be completed by the data subject, and this installation "+
			"cannot yet mail one: %w", apperrors.ErrConflict))
}

// SuppressPerson serves POST /people/{id}/consent/suppress: a person recording
// that the subject asked us to stop writing to them.
//
// Wire-only. The store owns which kinds a seat may write, whose authority the
// row carries and whether this caller may write about this subject at all —
// the last of which must not be decided here, because a handler that probed
// visibility itself would be a second row-scope gate beside the one the store
// already runs.
func (h Handlers) SuppressPerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.SuppressPersonJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := SuppressInput{
		PersonID: ids.From[ids.PersonKind](ids.UUID(id)),
		Kind:     string(req.Kind),
	}
	if req.Reason != nil {
		in.Reason = *req.Reason
	}
	if err := h.store.Suppress(r.Context(), in); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// 204: the row is the whole result, and echoing it back would invite a
	// caller to read a suppression list from the write door rather than from
	// the person's own consent view.
	w.WriteHeader(http.StatusNoContent)
}

// RecordQualifyingEvent serves POST /people/{id}/consent/qualifying-events: the
// one lawful basis nothing can derive, written down by the person who was
// there. The store owns the rules; this is wire-only.
func (h Handlers) RecordQualifyingEvent(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.RecordQualifyingEventRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	recorded, err := h.store.RecordQualifyingEvent(r.Context(), pathID[ids.PersonKind](id), RecordQualifyingEventInput{
		Kind:       string(req.Kind),
		Note:       req.Note,
		OccurredAt: req.OccurredAt,
	})
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	out := crmcontracts.QualifyingEventRecord{
		Kind:       crmcontracts.QualifyingEventRecordKind(recorded.Kind),
		OccurredAt: recorded.OccurredAt,
	}
	if recorded.Note != "" {
		note := recorded.Note
		out.Note = &note
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

func writeConsentErr(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *ValidationError
	if errors.As(err, &invalid) {
		httperr.Write(w, r, httperr.Validation(invalid.Field, "invalid", invalid.Reason))
		return
	}
	var badEvent *InvalidQualifyingEventError
	if errors.As(err, &badEvent) {
		field, code, message := badEvent.FieldFault()
		httperr.Write(w, r, httperr.Validation(field, code, message))
		return
	}
	httperr.Write(w, r, err)
}

func wirePurpose(p Purpose) crmcontracts.ConsentPurpose {
	return crmcontracts.ConsentPurpose{
		Id:                  openapi_types.UUID(p.ID.UUID),
		Key:                 p.Key,
		Label:               p.Label,
		RequiresDoubleOptIn: &p.RequiresDoubleOptIn,
		CreatedAt:           p.CreatedAt,
	}
}

func wireState(st State) crmcontracts.PersonConsentState {
	out := crmcontracts.PersonConsentState{
		PurposeId:              openapi_types.UUID(st.PurposeID.UUID),
		State:                  crmcontracts.PersonConsentStateState(st.State),
		LawfulBasis:            st.LawfulBasis,
		DoubleOptInConfirmedAt: st.DoubleOptInConfirmedAt,
		UpdatedAt:              st.UpdatedAt,
	}
	if st.PurposeKey != "" {
		out.PurposeKey = &st.PurposeKey
	}
	return out
}

func wireEvent(ev ProofEvent) crmcontracts.ConsentEvent {
	actorType, actorID := splitActor(ev.CapturedBy)
	wireActorType := crmcontracts.ConsentEventActorType(actorType)
	return crmcontracts.ConsentEvent{
		Id:          openapi_types.UUID(ev.ID),
		PurposeId:   openapi_types.UUID(ev.PurposeID.UUID),
		NewState:    crmcontracts.ConsentEventNewState(ev.NewState),
		LawfulBasis: ev.LawfulBasis,
		Source:      ev.Source,
		ActorType:   &wireActorType,
		ActorId:     &actorID,
		OccurredAt:  ev.OccurredAt,
	}
}

// splitActor maps the stored captured_by ("human:<id>" | "agent:<id>" |
// "connector:<name>" | "system") onto the contract's actor pair.
func splitActor(capturedBy string) (actorType, actorID string) {
	for _, prefix := range []string{"human", "agent", "connector", "system"} {
		if capturedBy == prefix {
			return prefix, ""
		}
		if len(capturedBy) > len(prefix)+1 && capturedBy[:len(prefix)+1] == prefix+":" {
			return prefix, capturedBy[len(prefix)+1:]
		}
	}
	return "system", capturedBy
}
