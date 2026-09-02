// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergePerson: POST /people/{id}/merge — merge this person (A, the path id)
// into target_id (B, the survivor). Returns the survivor. The store owns
// the collision-aware relinking and the restrictive consent rule; this
// handler is wire-only. Agent 🟡 governance is applied by the ADR-0055
// admission gate that wraps this route (same staging as the merge_records
// tool), not by this handler.
func (h Handlers) MergePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.MergePersonParams) {
	var req crmcontracts.MergePersonJSONBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	survivor, err := h.store.MergePerson(r.Context(), pathID[ids.PersonKind](id), ids.From[ids.PersonKind](ids.UUID(req.TargetId)))
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

func (h Handlers) ListPeople(w http.ResponseWriter, r *http.Request, params crmcontracts.ListPeopleParams) {
	in := ListPeopleInput{
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
	in.OrganizationID = idArg[ids.OrganizationKind](params.OrganizationId)

	people, page, err := h.store.ListPeople(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.PersonListResponse{Data: people, Page: pageInfo(page)})
}

func (h Handlers) CreatePerson(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreatePersonParams) {
	var req crmcontracts.CreatePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in, err := personCreateInput(req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}

	person, err := h.store.CreatePerson(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/people/"+person.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, person)
}

// ImportVCards serves POST /people/vcard-import: the parse, the write, and the
// per-card report.
//
// The upload itself is read here rather than in compose because the whole
// operation is this module's — there is no cross-module assembly to do, and a
// handler split across two packages for the sake of a multipart form would put
// half of one endpoint where nobody looks for it. The ceiling the parse runs
// under is granted to this route in compose.uploadCeilings.
func (h Handlers) ImportVCards(w http.ResponseWriter, r *http.Request) {
	// upload:route /v1/people/vcard-import — the ceiling this parse runs under
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
		if err := h.stageVCardReview(ctx, entries[results[i].Index], results[i].PersonID); err != nil {
			slog.ErrorContext(ctx, "people: a vCard near-match could not be proposed for review",
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
		if result.PersonID != nil {
			id := openapi_types.UUID(result.PersonID.UUID)
			item.PersonId = &id
		}
		if result.Reason != "" {
			reason := result.Reason
			item.Reason = &reason
		}
		out = append(out, item)
	}
	return out
}

// QuickCapturePerson serves POST /people/quick-capture: the person, their employer
// and the edge between them in one write. The store owns the transaction; this
// handler is wire-only, and its one decision is that an absent employer is a
// 201 like any other rather than a refusal.
func (h Handlers) QuickCapturePerson(w http.ResponseWriter, r *http.Request, _ crmcontracts.QuickCapturePersonParams) {
	var req crmcontracts.QuickCapturePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	in := QuickCaptureInput{
		FullName:         req.FullName,
		Title:            req.Title,
		OrganizationID:   idArg[ids.OrganizationKind](req.OrganizationId),
		OrganizationName: req.OrganizationName,
		Role:             req.Role,
		ProfileURL:       req.ProfileUrl,
		Phone:            req.Phone,
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
	out := crmcontracts.QuickCapturePersonResult{
		Person:              captured.Person,
		OrganizationCreated: &captured.OrganizationCreated,
	}
	if captured.OrganizationID != nil {
		orgID := openapi_types.UUID(captured.OrganizationID.UUID)
		out.OrganizationId = &orgID
	}
	w.Header().Set("Location", "/v1/people/"+captured.Person.Id.String())
	httperr.WriteJSON(w, http.StatusCreated, out)
}

func (h Handlers) GetPerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	person, err := h.store.GetPerson(r.Context(), pathID[ids.PersonKind](id), storekit.IncludeArchived)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

func (h Handlers) UpdatePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.UpdatePersonParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var req crmcontracts.UpdatePersonRequest
	if !httperr.Decode(w, r, &req) {
		return
	}

	update := personUpdateInput(req, ifVersion)
	// An explicit null on a nullable field is "clear this", not "leave it": the
	// decoded pointer cannot tell the two apart, and the contract declares these
	// fields nullable, so accepting one and doing nothing is a success the caller
	// cannot trust.
	update.Clear = httperr.ClearedFields(r)
	person, err := h.store.UpdatePerson(r.Context(), pathID[ids.PersonKind](id), update)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

// ArchivePerson: DELETE = archive, returning the archived entity (200,
// architecture/11 §8 — never a bare 204 for domain rows).
func (h Handlers) ArchivePerson(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.ArchivePersonParams) {
	ifVersion, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	person, err := h.store.ArchivePerson(r.Context(), pathID[ids.PersonKind](id), ifVersion)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, person)
}

func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}
