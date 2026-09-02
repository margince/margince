// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (h Handlers) ListDataSubjectRequests(w http.ResponseWriter, r *http.Request, params crmcontracts.ListDataSubjectRequestsParams) {
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	status := ""
	if params.Status != nil {
		if !params.Status.Valid() {
			writeConsentErr(w, r, &ValidationError{Field: fieldStatus, Reason: "not a queue state"})
			return
		}
		status = string(*params.Status)
	}
	requests, page, err := h.store.ListDSRs(r.Context(), params.Limit, cursor, status)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	data := make([]crmcontracts.DataSubjectRequest, 0, len(requests))
	for _, d := range requests {
		data = append(data, wireDSR(d))
	}
	info := crmcontracts.PageInfo{HasMore: page.HasMore}
	if page.NextCursor != "" {
		info.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"data": data, "page": info})
}

func (h Handlers) CreateDataSubjectRequest(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateDataSubjectRequestParams) {
	var req crmcontracts.CreateDataSubjectRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := CreateDSRInput{
		Kind:       string(req.Kind),
		SubjectRef: req.SubjectRef,
		DueAt:      req.DueAt,
		AssigneeID: idArg[ids.UserKind](req.AssigneeId),
	}
	created, err := h.store.CreateDSR(r.Context(), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wireDSR(created))
}

func (h Handlers) UpdateDataSubjectRequest(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.UpdateDataSubjectRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := UpdateDSRInput{Resolution: req.Resolution, AssigneeID: idArg[ids.UserKind](req.AssigneeId)}
	if req.Status != nil {
		status := string(*req.Status)
		in.Status = &status
	}
	// Fulfilling an erasure request EXECUTES the irreversible scrub, so it
	// cannot ride the plain UpdateDSR path: the erase and the status flip must
	// be serialized against every other officer touching this row, or a
	// concurrent reject/fulfil could interleave and leave a subject erased on a
	// request the queue still shows open. FulfilErasure owns that serialization
	// — it locks the request FOR UPDATE and holds the lock across the erase,
	// refuses a subject_ref that names no person, and only then finalizes.
	if in.Status != nil && *in.Status == "fulfilled" {
		current, err := h.store.GetDSR(r.Context(), ids.UUID(id))
		if err != nil {
			writeConsentErr(w, r, err)
			return
		}
		if current.Kind == dsrKindErasure {
			if h.eraser == nil {
				// Fail closed: fulfilling an erasure on a surface with no
				// erase path wired would certify a deletion that never
				// happened.
				writeConsentErr(w, r, errors.New("consent: erasure fulfillment has no erase path wired"))
				return
			}
			updated, err := h.store.FulfilErasure(r.Context(), ids.UUID(id), in, h.eraser.ErasePerson)
			if err != nil {
				writeConsentErr(w, r, err)
				return
			}
			httperr.WriteJSON(w, http.StatusOK, wireDSR(updated))
			return
		}
	}
	updated, err := h.store.UpdateDSR(r.Context(), ids.UUID(id), in)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireDSR(updated))
}

// DownloadDataSubjectPackage answers an Art. 15 access request with the package
// it asks for.
//
// The queue could record an access request and mark it fulfilled with no product
// path that produced anything, so "fulfilled" meant an officer had said so. This
// is the path: it assembles what the installation holds about the subject and
// hands it back, as the file an officer forwards.
//
// It does NOT change the request's status. Producing the export and deciding the
// request is answered are two acts by the same person, and collapsing them would
// close a request on a download that might have failed to reach anybody — the
// officer marks it fulfilled through the existing PATCH once they have sent it.
func (h Handlers) DownloadDataSubjectPackage(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.assembler == nil {
		// Fail closed rather than answer an empty package: a role composed
		// without the assembler cannot produce an Art. 15 export, and an empty
		// one would read as "we hold nothing about this person".
		httperr.ServiceUnavailable(w, r,
			"this installation cannot assemble a subject-access package here")
		return
	}
	// GetDSR takes the queue's own gate — admin, human, person.read — so
	// reaching the request is already what reaching any request takes.
	//
	// That gate is the narrow one and it has to stay in front. AssembleSAR
	// admits any unbounded human holding person.delete, which the seeded
	// defaults give to ops and management as well as admin, so calling it
	// without reading the request first would make the export reachable by
	// roles the queue that owns this workflow refuses.
	request, err := h.store.GetDSR(r.Context(), ids.UUID(id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	// An access request, not an erasure or a rectification. The other two kinds
	// have their own paths and answering them with a package would export a
	// subject's whole record to close a request that asked for something else.
	if request.Kind != dsrKindAccess {
		writeConsentErr(w, r, &ValidationError{
			Field:  fieldKind,
			Reason: "only an access request has a package to download",
		})
		return
	}
	// The same refusal the erasure path gives, for the same reason: subject_ref
	// is free text until somebody resolves it to a person, and a request naming
	// nobody has nothing to assemble.
	personID, parseErr := ids.Parse(request.SubjectRef)
	if parseErr != nil {
		writeConsentErr(w, r, &ValidationError{
			Field:  fieldSubjectRef,
			Reason: "an access request must name a person id before its package can be assembled",
		})
		return
	}
	body, err := h.assembler.AssemblePackage(r.Context(), personID)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	// Served as a download rather than an inline body: this is a file an officer
	// forwards to the subject, and naming it after the request is what ties the
	// copy they sent to the row that recorded it. Through httperr.Download, which
	// is the one spelling of a download's headers.
	httperr.Download{
		ContentType: "application/json",
		Filename:    "subject-access-" + request.ID.String() + ".json",
		Size:        int64(len(body)),
	}.WriteHeaders(w)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		// The status and headers are already on the wire, so this cannot become
		// an error response. Logged rather than swallowed: a truncated export an
		// officer forwards as complete is the failure worth knowing about.
		slog.ErrorContext(r.Context(), "subject-access package was not fully written",
			"request", request.ID.String(), "error", err)
	}
}
