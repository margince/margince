// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ValidateImportRun tests a draft's saved choices on the same run, so "Import
// anyway" is a save and a second test, never a second run.
func (h importHandlers) ValidateImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	ctx := r.Context()
	if err := auth.Require(ctx, importRunGrant, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.blobs == nil {
		httperr.Write(w, r, errNoObjectStore)
		return
	}
	var req crmcontracts.ImportRunVersion
	if !httperr.Decode(w, r, &req) {
		return
	}
	run, err := h.stagedFor(ctx, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	tested, err := h.testRun(ctx, run, req.Version, requestOfRun(run))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, tested)
}

// testRun saves req's choices on run at version and tests them on that run.
// Both doors arrive here, so the choices a report describes are the ones the
// run keeps for its commit. A run another request is already testing at that
// version is answered as it stands, which makes a double click start one test.
func (h importHandlers) testRun(
	ctx context.Context, run migration.Run, version int, req crmcontracts.CreateImportRunRequest,
) (crmcontracts.ImportRun, error) {
	mapping, err := h.checkedMapping(ctx, req)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	source, skipped, err := bundleSourceFor(h.blobs, mapping, run.SourceRef, h.uploadLimit)
	if err != nil {
		return crmcontracts.ImportRun{}, importProblem(err)
	}
	if err := checkEveryPartsColumns(ctx, source, mapping); err != nil {
		return crmcontracts.ImportRun{}, err
	}

	runs := migration.NewRunStore(h.db)
	saved, err := runs.UpdateDraft(ctx, run.ID, version, draftPatchOf(mapping))
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	queued, moved, err := runs.QueueValidation(ctx, saved.ID, saved.Version)
	if err != nil {
		return crmcontracts.ImportRun{}, h.sourceBusy(ctx, err)
	}
	if !moved {
		return toContractImportRun(queued), nil
	}
	report, err := h.dryRun(ctx, runs, saved.ID, source, skipped, &mapping)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	if err := runs.AwaitApproval(ctx, saved.ID, report); err != nil {
		// A run that cannot be parked for a human goes back to its draft
		// rather than sitting in `validating`, which nothing else moves.
		returnToDraft(ctx, runs, saved.ID, err)
		return crmcontracts.ImportRun{}, err
	}
	staged, err := runs.GetStaged(ctx, saved.ID)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	return toContractImportRun(staged), nil
}

// checkedMapping holds a request to what a run's choices must satisfy before
// a test reads a row. That is the object's fields, the grants and the tag.
func (h importHandlers) checkedMapping(
	ctx context.Context, req crmcontracts.CreateImportRunRequest,
) (migration.RunMapping, error) {
	catalogs, err := readImportCatalogs(ctx, h.db, requestedObjects(req))
	if err != nil {
		return migration.RunMapping{}, err
	}
	mapping, err := stagedMappingFrom(req, catalogs)
	if err != nil {
		return migration.RunMapping{}, err
	}
	if err := refuseAuthorFromNonImporter(ctx, mapping); err != nil {
		return migration.RunMapping{}, err
	}
	if err := h.contextTagIsApplicable(ctx, mapping.ContextTag); err != nil {
		return migration.RunMapping{}, err
	}
	return mapping, nil
}

// draftPatchOf is a whole checked mapping as a save. The file name is the
// upload's and is never changed by a choice.
func draftPatchOf(m migration.RunMapping) migration.DraftPatch {
	return migration.DraftPatch{
		Parts: &m.Parts, Object: &m.Object, Fields: &m.Fields, SourceKey: &m.SourceKey,
		SourceName: &m.SourceName, OnDuplicate: &m.OnDuplicate, OnBrokenLink: &m.OnBrokenLink,
		ContextTag: &m.ContextTag,
	}
}

// requestOfRun spells a run's saved choices as a create request. A saved
// draft is then checked as a new one is.
func requestOfRun(run migration.Run) crmcontracts.CreateImportRunRequest {
	req := crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnectorCsv, SourceRef: run.SourceRef,
	}
	m := run.Mapping
	if m == nil {
		return req
	}
	if len(m.Parts) > 0 {
		parts := make([]crmcontracts.ImportPartRequest, 0, len(m.Parts))
		for _, p := range m.Parts {
			parts = append(parts, crmcontracts.ImportPartRequest{
				File: p.File, Object: crmcontracts.ImportObject(p.Object), Mapping: p.Fields, SourceKey: sentText(p.SourceKey),
			})
		}
		req.Parts = &parts
	} else {
		req.Object = new(crmcontracts.ImportObject(m.Object))
		req.Mapping = &m.Fields
		req.SourceKey = sentText(m.SourceKey)
	}
	req.SourceName, req.FileName = sentText(m.SourceName), sentText(m.FileName)
	if m.OnDuplicate != "" {
		req.OnDuplicate = new(crmcontracts.ImportOnDuplicate(m.OnDuplicate))
	}
	if m.OnBrokenLink != "" {
		req.OnBrokenLink = new(crmcontracts.ImportOnBrokenLink(m.OnBrokenLink))
	}
	if tag := parseContextTag(m.ContextTag); !tag.IsZero() {
		req.ContextTagId = new(openapi_types.UUID(tag.UUID))
	}
	return req
}

// sentText is an optional request field: absent when empty.
func sentText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// sourceBusy names the writing run that holds the source name, so the caller
// can look at it. A run the caller may not read is not named.
func (h importHandlers) sourceBusy(ctx context.Context, err error) error {
	busy, ok := errors.AsType[*migration.SourceBusyError](err)
	if !ok {
		return err
	}
	refusal := &httperr.DetailedError{
		Status: http.StatusConflict, Code: httperr.SourceBusyCode,
		Detail: "another import of this source name is still writing; wait for it to finish, then test again",
	}
	if _, readErr := h.stagedFor(ctx, openapi_types.UUID(busy.RunID)); readErr == nil {
		refusal.Details = map[string]any{"run_id": busy.RunID.String()}
	}
	return refusal
}
