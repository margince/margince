// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeContact serves POST /contacts/{id}/merge — merge this contact (A, the path id)
// into target_id (B, the survivor). Returns the survivor. The store owns
// the collision-aware relinking and the restrictive consent rule; this
// handler is wire-only. Agent 🟡 governance is applied by the ADR-0055
// admission gate that wraps this route (same staging as the merge_records
// tool), not by this handler.
func (h Handlers) MergeContact(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.MergeContactParams) {
	var req crmcontracts.MergeContactJSONBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	survivor, err := h.store.MergeContact(r.Context(), pathID[ids.ContactKind](id), ids.From[ids.ContactKind](ids.UUID(req.TargetId)))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, survivor)
}

// uuidArgs widens a repeated uuid query parameter to the store's own shape.
// An absent parameter and an empty list are the same thing here: no filter.
func uuidArgs(in *[]openapi_types.UUID) []ids.UUID {
	if in == nil {
		return nil
	}
	out := make([]ids.UUID, 0, len(*in))
	for _, v := range *in {
		out = append(out, ids.UUID(v))
	}
	return out
}

// ListContacts serves GET /contacts — one keyset page under the caller's row scope.
func (h Handlers) ListContacts(w http.ResponseWriter, r *http.Request, params crmcontracts.ListContactsParams) {
	in := ListContactsInput{
		Cursor:          params.Cursor,
		Limit:           params.Limit,
		Query:           params.Q,
		IncludeArchived: params.IncludeArchived != nil && *params.IncludeArchived,
		CapturedByKind:  capturedByKindArg(params.CapturedByKind),
		AiWritten:       params.AiWritten,
		Sort:            params.Sort,
		CustomFilters:   httperr.CustomFieldFilters(r),
		TagIDs:          uuidArgs(params.TagId),
	}
	mode, err := storekit.ParseTagMode((*string)(params.TagMode))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("tag_mode", "invalid", err.Error()))
		return
	}
	in.TagMode = mode
	in.OwnerID = idArg[ids.UserKind](params.OwnerId)
	in.OwnerTeamID = idArg[ids.TeamKind](params.OwnerTeamId)
	in.Unassigned = params.Unassigned
	in.CompanyID = idArg[ids.CompanyKind](params.CompanyId)

	contacts, page, err := h.store.ListContacts(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ContactListResponse{Data: contacts, Page: pageInfo(page)})
}

// CreateContact serves POST /contacts.
func (h Handlers) CreateContact(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateContactParams) {
	var req crmcontracts.CreateContactRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := contactCreateInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}

	contact, err := h.store.CreateContact(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/contacts/"+contact.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, contact)
}

// ImportVCards serves POST /contacts/vcard-import: the parse, the write, and the
// per-card report.
//
// The upload itself is read here rather than in compose because the whole
// operation is this module's — there is no cross-module assembly to do, and a
// handler split across two packages for the sake of a multipart form would put
// half of one endpoint where nobody looks for it. The ceiling the parse runs
// under is granted to this route in compose.uploadCeilings.
func (h Handlers) ImportVCards(w http.ResponseWriter, r *http.Request) {
	// THE GRANTS, BEFORE THE BYTES, and both of them because the import needs
	// both: it creates the contacts no card matches and updates the ones a card
	// does. The store asks for the pair and keeps asking — but it asks after
	// the parse, so a session holding neither still made the server take a
	// whole address book apart before being refused.
	if err := auth.Require(r.Context(), entityContact, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(r.Context(), entityContact, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// upload:route /v1/contacts/vcard-import — the ceiling this parse runs under
	// is granted to that path in compose.uploadCeilings, and
	// TestEveryMultipartParseNamesItsRoute holds the two together. What is
	// bounded HERE is only how much of the parse stays resident before it
	// spills to disk.
	//nolint:gosec // G120 wants a bound, and the bound is the chassis's own MaxBytesReader on this route: this argument is the spill threshold, deliberately far below the ceiling.
	if err := r.ParseMultipartForm(vcardSpillBytes); err != nil {
		httperr.Write(w, r, httperr.Validation("file", "unreadable",
			"Send the .vcf file as a multipart form field named `file`."))
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httperr.Write(w, r, httperr.Validation("file", "missing", "Attach the .vcf file to import as `file`."))
		return
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded vCard part", "err", cerr)
		}
	}(r.Context())

	entries, err := ParseVCards(file)
	if err != nil {
		// The parser's own words, not a generic refusal: it says which shape it
		// could not read, which is what a reader with a forty-card file needs
		// in order to find the row rather than re-export the file.
		httperr.Write(w, r, httperr.Validation("file", "unreadable", err.Error()))
		return
	}
	results, err := h.store.ImportVCards(r.Context(), entries)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.stageVCardReviews(r.Context(), entries, results)
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.VCardImportReport{
		Results: toContractVCardResults(results),
	})
}

// stageVCardReviews turns each near-match the import refused to create into a
// durable proposal, so the question outlives the upload response instead of
// dying with it. A staging fault does not fail the upload — the cards are
// already written or refused, and failing now would un-tell the reader what
// DID happen — but it is not silent either: the card's own result line says
// the review could not be queued and that re-importing retries, because a 200
// that quietly dropped the promised review is the invisibility this staging
// exists to end.
func (h Handlers) stageVCardReviews(ctx context.Context, entries []VCardEntry, results []VCardResult) {
	if h.stageVCardReview == nil {
		return
	}
	for i := range results {
		if results[i].Outcome != VCardNeedsReview {
			continue
		}
		if err := h.stageVCardReview(ctx, entries[results[i].Index], results[i].ContactID); err != nil {
			slog.ErrorContext(ctx, "contacts: a vCard near-match could not be proposed for review",
				"card_index", results[i].Index, "err", err)
			results[i].Reason = "this card resembles an existing contact, and the review could not be queued; import the card again to retry"
		}
	}
}

func toContractVCardResults(results []VCardResult) []crmcontracts.VCardImportResult {
	out := make([]crmcontracts.VCardImportResult, 0, len(results))
	for _, result := range results {
		item := crmcontracts.VCardImportResult{
			Index:    result.Index,
			FullName: result.FullName,
			Outcome:  crmcontracts.VCardImportResultOutcome(result.Outcome),
		}
		if result.ContactID != nil {
			id := openapi_types.UUID(result.ContactID.UUID)
			item.ContactId = &id
		}
		if result.Reason != "" {
			reason := result.Reason
			item.Reason = &reason
		}
		out = append(out, item)
	}
	return out
}

// QuickCaptureContact serves POST /contacts/quick-capture: the contact, their employer
// and the edge between them in one write. The store owns the transaction; this
// handler is wire-only, and its one decision is that an absent employer is a
// 201 like any other rather than a refusal.
func (h Handlers) QuickCaptureContact(w http.ResponseWriter, r *http.Request, _ crmcontracts.QuickCaptureContactParams) {
	var req crmcontracts.QuickCaptureContactRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := QuickCaptureInput{
		FullName:    req.FullName,
		Title:       req.Title,
		CompanyID:   idArg[ids.CompanyKind](req.CompanyId),
		CompanyName: req.CompanyName,
		Role:        req.Role,
		ProfileURL:  req.ProfileUrl,
		Phone:       req.Phone,
	}
	if req.Email != nil {
		email := string(*req.Email)
		in.Email = &email
	}

	captured, err := h.store.QuickCapture(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.QuickCaptureContactResult{
		Contact:        captured.Contact,
		CompanyCreated: &captured.CompanyCreated,
	}
	if captured.CompanyID != nil {
		companyID := openapi_types.UUID(captured.CompanyID.UUID)
		out.CompanyId = &companyID
	}
	w.Header().Set("Location", "/v1/contacts/"+captured.Contact.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// GetContact serves GET /contacts/{id}, archived rows included — the client
// decides how to render one.
func (h Handlers) GetContact(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	contact, err := h.store.GetContact(r.Context(), pathID[ids.ContactKind](id), storekit.IncludeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, contact)
}

// UpdateContact serves PATCH /contacts/{id} under an If-Match version.
func (h Handlers) UpdateContact(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdateContactParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdateContactRequest
	if !httperr.Decode(w, r, &req) {
		return
	}

	update := contactUpdateInput(req, ifVersion)
	// An explicit null on a nullable field is "clear this", not "leave it": the
	// decoded pointer cannot tell the two apart, and the contract declares these
	// fields nullable, so accepting one and doing nothing is a success the caller
	// cannot trust.
	update.Clear = httperr.ClearedFields(r)
	contact, err := h.store.UpdateContact(r.Context(), pathID[ids.ContactKind](id), update)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, contact)
}

// ArchiveContact serves DELETE — archive, returning the archived entity (200,
// architecture/11 §8 — never a bare 204 for domain rows).
func (h Handlers) ArchiveContact(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchiveContactParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	contact, err := h.store.ArchiveContact(r.Context(), pathID[ids.ContactKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, contact)
}

// PublishCapturedContact shares a contact the caller's own mailbox created.
//
// No If-Match. The version guards a field edit against a concurrent one; this
// write is idempotent in the only direction it goes, and a contact the
// company can already see answers the same 404 as one that was never the
// caller's — so a stale version could not produce a wrong outcome, only a
// confusing refusal.
func (h Handlers) PublishCapturedContact(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.PromoteOwnCapturedContact(r.Context(), pathID[ids.ContactKind](id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}

// DismissRelationshipNudge sets a lapsed contact aside for the calling reader.
//
// No If-Match. The version guards a field edit against a concurrent one; this
// writes a reader's own judgement about their own morning, and two dismissals
// of the same contact by the same reader are one decision restated rather than
// a conflict.
func (h Handlers) DismissRelationshipNudge(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.DismissRelationshipNudgeRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.DismissRelationshipNudge(
		r.Context(), pathID[ids.ContactKind](id), req.Days); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RestoreRelationshipNudge puts a set-aside contact back on the reader's lane.
func (h Handlers) RestoreRelationshipNudge(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if err := h.store.RestoreRelationshipNudge(
		r.Context(), pathID[ids.ContactKind](id)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
