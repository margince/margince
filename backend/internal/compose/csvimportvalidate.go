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

// ValidateImportRun queues the test of a draft's saved choices on the same run.
// "Import anyway" is then a save and a second test, never a second run. A run
// already being tested at that version is answered 200, as it stands.
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
	queued, moved, err := h.testRun(ctx, run, req.Version, requestOfRun(run))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := http.StatusAccepted
	if !moved {
		status = http.StatusOK
	}
	httperr.WriteJSON(w, status, queued)
}

// testRun saves req's choices on run at version and queues their test on that
// run, in the transaction that moves it. Both doors arrive here, so the choices
// a report describes are the ones the run keeps for its commit. The worker
// reads the rows; the request checks only what the choices must satisfy. A run
// already being tested at that version is answered unmoved, which makes a
// double click start one test.
func (h importHandlers) testRun(
	ctx context.Context, run migration.Run, version int, req crmcontracts.CreateImportRunRequest,
) (crmcontracts.ImportRun, bool, error) {
	queue, ws, err := h.jobQueue(ctx)
	if err != nil {
		return crmcontracts.ImportRun{}, false, err
	}
	mapping, err := h.checkedMapping(ctx, req)
	if err != nil {
		return crmcontracts.ImportRun{}, false, err
	}
	source, _, err := bundleSourceFor(h.blobs, mapping, run.SourceRef, h.uploadLimit)
	if err != nil {
		return crmcontracts.ImportRun{}, false, importProblem(err)
	}
	if err := checkEveryPartsColumns(ctx, source, mapping); err != nil {
		return crmcontracts.ImportRun{}, false, err
	}

	runs := migration.NewRunStore(h.db)
	saved, err := runs.UpdateDraft(ctx, run.ID, version, draftPatchOf(mapping))
	if err != nil {
		return crmcontracts.ImportRun{}, false, err
	}
	queued, moved, err := runs.QueueValidation(ctx, saved.ID, saved.Version, queue.validates(ws))
	if err != nil {
		return crmcontracts.ImportRun{}, false, h.sessionRefusal(ctx, err)
	}
	return toContractImportRun(queued), moved, nil
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

// sessionRefusal answers a refusal that points at another run. That is the run
// holding the source name, the one that replaced this run, or the agent's own
// test still going. It is named only when the caller may read it.
func (h importHandlers) sessionRefusal(ctx context.Context, err error) error {
	var other migration.RunID
	refusal := &httperr.DetailedError{Status: http.StatusConflict}
	if agent, ok := errors.AsType[*migration.AgentBusyError](err); ok {
		other, refusal.Code = agent.RunID, httperr.AgentBusyCode
		refusal.Detail = "this agent's test run of another import is still running; read that run until it has its report, then try again"
	} else if busy, ok := errors.AsType[*migration.SourceBusyError](err); ok {
		other, refusal.Code = busy.RunID, httperr.SourceBusyCode
		refusal.Detail = "another import of this source name is in progress; let it finish or cancel it, then try again"
	} else if replaced, ok := errors.AsType[*migration.SupersededError](err); ok {
		other, refusal.Code = replaced.By, "conflict"
		refusal.Detail = "a newer test run of this source name replaced this one; go on with that run"
	} else {
		return err
	}
	if _, readErr := h.stagedFor(ctx, openapi_types.UUID(other)); readErr == nil {
		refusal.Details = map[string]any{"run_id": other.String()}
	}
	return refusal
}
