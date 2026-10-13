// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The migrate-in surface (IEM-WIRE-3/4/5/6 and the IEM-WIRE-8 upload):
// upload a file, read what its columns hold, map them, dry-run, approve.
//
// It lives in compose rather than in modules/migration for the same reason the
// flip does: driving the engine means constructing a Writers over contacts's
// stores, and a module may never import a sibling. The engine, the run record
// and the identity map are all the module's; this file is the door.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// importSpillBytes is how much of the upload is held in memory before the
	// rest goes to a temp file. The request cap itself is CAP-BODY, owned by the
	// rate-limit chapter and set by the deployment (OPS-CFG-12); enforced with a
	// MaxBytesReader so an oversized file is a distinct refusal, never a
	// truncated read that imports half a customer's estate and reports success.
	importSpillBytes = 1 << 20
	// importBlobKind namespaces uploaded sources inside the workspace's blob
	// prefix, beside attachments and logos.
	importBlobKind = "import"
	// importSourceProvenance is the `source` every run row carries: this
	// surface, not the connector, which lives in its own column.
	importSourceProvenance = "import_api"
	// importPredictPage bounds one prediction read; the same page size the
	// engine walks with, so the two make the same number of round trips.
	importPredictPage = 200
)

type importHandlers struct {
	db    *database.DB
	blobs blobstore.Store
	// uploadLimit is the deployment's ceiling for this route (OPS-CFG-12).
	// Zero refuses every upload, which is the honest reading of "nobody has
	// said" for a bound.
	uploadLimit int64
	// queue enqueues an attempt's job; nil is a role that queues no import work.
	queue *importEnqueuer
	// previewWait, when set, replaces how long preview_import waits for its
	// test's report. Only the test harness sets it.
	previewWait agents.RegistryOption
}

// UploadImportSource stores a file, describes it (IEM-WIRE-8) and opens its
// draft run. Nothing is imported or validated against the estate here. The
// response is the evidence a human makes the mapping on.
func (h importHandlers) UploadImportSource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(ctx, importRunGrant, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.blobs == nil {
		httperr.Write(w, r, fmt.Errorf("this role stores no objects, so it cannot accept an import: %w", apperrors.ErrConflict))
		return
	}

	if h.uploadLimit <= 0 {
		// Our fault, not the caller's — the same guard the other two upload
		// routes carry, and for the same reason: a zero bound refuses a
		// perfectly good file and tells its sender it "exceeds the 0 MB limit",
		// which sends them off to shrink something that was never too large.
		httperr.Write(w, r, errUploadLimitUnset)
		return
	}

	upload, err := readImportUpload(w, r, h.uploadLimit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var out crmcontracts.ImportSourceProfile
	if migration.IsZip(upload.Body) {
		out, err = h.profileBundle(ctx, upload)
	} else {
		out, err = h.profileAndStore(ctx, upload.Object, upload.FileName, upload.Body)
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// discardSource ends the draft a stored import source opened, which releases
// the file to the reaper. A source no run holds is deleted at once. Keyed the
// same way it was written, so a ref from another workspace names a key this
// one never wrote.
func (h importHandlers) discardSource(ctx context.Context, ref string) error {
	if h.blobs == nil {
		return fmt.Errorf("this role stores no objects, so it holds no import source: %w", apperrors.ErrConflict)
	}
	// The SAME check staging makes, not a prefix of it. A prefix admits a key
	// with extra segments under this workspace's import namespace, and answers
	// forbidden — which tells a caller that a reference they were never given
	// belongs to somebody. ownsSource matches the whole minted key and answers
	// not-found, and a delete is not the place to start disagreeing with the
	// read about what a source ref is.
	if err := h.ownsSource(ctx, ref); err != nil {
		return err
	}
	err := migration.NewRunStore(h.db).DiscardOwnDraft(ctx, ref)
	if errors.Is(err, apperrors.ErrNotFound) {
		return h.blobs.Delete(ctx, ref)
	}
	return err
}

// profileAndStore is everything the upload does once it has the bytes. It reads
// the header and counts the rows, proposes a mapping, stores the file and opens
// its run. fileName is empty for pasted text.
//
// Two doors share it. The REST one reads a multipart file; the tool surface
// takes CSV as text, since an assistant cannot perform a file upload.
func (h importHandlers) profileAndStore(
	ctx context.Context, object, fileName string, body []byte,
) (crmcontracts.ImportSourceProfile, error) {
	if err := migration.CheckUTF8(body); err != nil {
		return crmcontracts.ImportSourceProfile{}, importProblem(err)
	}
	profile, err := migration.ProfileCSV(bytes.NewReader(body), migration.ProfileRowLimit)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, importProblem(err)
	}
	rows, err := migration.CountRecords(bytes.NewReader(body))
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, importProblem(err)
	}
	if err := refuseTooManyRows(rows); err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	cat, err := readImportCatalog(ctx, h.db, object)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	targets, err := importTargets(object, cat)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	key, err := h.storeSource(ctx, body, "text/csv")
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	suggested := suggestImportMapping(profile, targets, cat)
	out := crmcontracts.ImportSourceProfile{
		SourceRef:        key,
		Object:           new(crmcontracts.ImportObject(object)),
		FileName:         sentText(fileName),
		Columns:          toContractColumns(profile),
		RowsProfiled:     profile.RowsProfiled,
		RowCount:         rows,
		SuggestedMapping: suggested,
		Targets:          targets,
	}
	return h.openDraft(ctx, out, migration.RunMapping{Object: object, Fields: suggested, FileName: fileName})
}

// CreateImportRun queues the test of a mapped file against the estate and
// writes nothing (IEM-WIRE-3, AC-M5). The worker moves the run on to the report.
//
// Open to an agent: this call writes NO domain rows, by construction (AC-M5),
// so the worst an ungranted-but-authenticated caller could do is produce a
// report. What commits is approveImportRun, which stays confirm-first on the
// tool surface — a contact sees the report and says yes.
func (h importHandlers) CreateImportRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The grant is taken BEFORE the body is read: an ungranted caller must not
	// be able to tell a rejected mapping from an accepted one, which is what a
	// 422 arriving ahead of the 403 would tell them.
	if err := auth.Require(ctx, importRunGrant, principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.blobs == nil {
		httperr.Write(w, r, errNoObjectStore)
		return
	}
	var req crmcontracts.CreateImportRunRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	staged, err := h.stageRun(ctx, req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, staged)
}

// stageRun is everything createImportRun does once it HAS the request. It
// checks the source is this workspace's, then tests the mapping on its run.
//
// It is separate from the transport because the tool surface arrives here too,
// and the dry run is the part that must be identical on both doors — G stays
// product law, so nothing may commit an import that has not produced a report
// through exactly this path.
func (h importHandlers) stageRun(
	ctx context.Context, req crmcontracts.CreateImportRunRequest,
) (crmcontracts.ImportRun, error) {
	if err := h.ownsSource(ctx, req.SourceRef); err != nil {
		return crmcontracts.ImportRun{}, err
	}
	run, err := h.draftOfSource(ctx, req)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	queued, _, err := h.testRun(ctx, run, run.Version, req)
	return queued, err
}

// GetImportRun reports the lifecycle (IEM-WIRE-6): a failed run carries its
// checkpoint, which is what makes it resumable rather than a dead end.
func (h importHandlers) GetImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	run, err := h.staged(r, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractImportRun(run))
}

// GetImportRunReport reads what the run will do, or did (IEM-WIRE-4) — one
// shape for both, so a human comparing them compares like with like.
func (h importHandlers) GetImportRunReport(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	run, err := h.reported(r, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractImportReport(run))
}

// ApproveImportRun commits a validated run (IEM-WIRE-5). It answers with the
// run as it stands once its commit is queued. The body is optional.
func (h importHandlers) ApproveImportRun(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var req crmcontracts.ApproveImportRunRequest
	if err := httperr.DecodeOrRefusal(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		httperr.Write(w, r, err)
		return
	}
	version := 0
	if req.Version != nil {
		version = *req.Version
	}
	out, err := h.commitRun(r.Context(), id, version)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, out)
}

// commitRun is the approval and the queueing of its write, without the
// transport. A version other than zero must be the run's.
//
// Shared with the tool surface for the reason the whole import seam is shared:
// this is the call that writes a customer's estate, and two paths that both
// "commit an approved run" would be two chances to disagree about what an
// approved run is.
func (h importHandlers) commitRun(
	ctx context.Context, id openapi_types.UUID, version int,
) (crmcontracts.ImportRun, error) {
	run, err := h.stagedFor(ctx, id)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}
	if run.Mapping == nil {
		return crmcontracts.ImportRun{}, fmt.Errorf(
			"import run %s carries no mapping, so it is not an approvable import: %w",
			run.ID, apperrors.ErrConflict)
	}
	if h.blobs == nil {
		return crmcontracts.ImportRun{}, errNoObjectStore
	}
	if err := refuseAuthorFromNonImporter(ctx, *run.Mapping); err != nil {
		return crmcontracts.ImportRun{}, err
	}
	queue, ws, err := h.jobQueue(ctx)
	if err != nil {
		return crmcontracts.ImportRun{}, err
	}

	// Built before the run is approved, so a run whose source or writers
	// cannot be built is refused while it can still be approved again.
	source, _, err := bundleSourceFor(h.blobs, *run.Mapping, run.SourceRef, h.uploadLimit)
	if err != nil {
		return crmcontracts.ImportRun{}, importProblem(err)
	}
	if _, err := newBundleWriters(ctx, h.db, run.ID, run.Mapping, source); err != nil {
		return crmcontracts.ImportRun{}, importProblem(err)
	}

	approved, err := h.startOrResume(ctx, migration.NewRunStore(h.db), run, version, queue.commits(ws))
	if err != nil {
		return crmcontracts.ImportRun{}, h.sessionRefusal(ctx, err)
	}
	return toContractImportRun(approved), nil
}

// startOrResume begins an approved run, or continues one that failed part-way.
//
// A failed run is resumable by contract (IEM-WIRE-6), and approve is the only
// door onto the engine. Pressing it again on a failure continues from the
// checkpoint, unless nobody ever approved the run. Every other state falls
// through to Approve, which refuses anything but awaiting_approval.
func (h importHandlers) startOrResume(
	ctx context.Context, runs *migration.RunStore, run migration.Run, version int, enqueue migration.Enqueue,
) (migration.Run, error) {
	if run.Status == migration.StatusFailed {
		return runs.ResumeApproved(ctx, run.ID, version, enqueue)
	}
	return runs.Approve(ctx, run.ID, version, enqueue)
}

// jobQueue answers the queue an attempt's job goes on, and the workspace the
// job names: the one this handler's database is bound to.
func (h importHandlers) jobQueue(ctx context.Context) (*importEnqueuer, ids.UUID, error) {
	if h.queue == nil {
		return nil, ids.UUID{}, errNoImportQueue
	}
	ws, err := h.db.Workspace(ctx)
	if err != nil {
		return nil, ids.UUID{}, err
	}
	return h.queue, ws.UUID, nil
}

// staged is the read every id-bearing operation starts from: the store's own
// gate answers not-found for a run outside the caller's scope rather than
// disclosing that it exists.
func (h importHandlers) staged(r *http.Request, id openapi_types.UUID) (migration.Run, error) {
	return h.stagedFor(r.Context(), id)
}

// stagedFor is staged without a request, so the tool surface reaches the same
// read the REST one does.
//
// THE HUMAN-ONLY CHECK THAT SAT HERE IS GONE, not moved. Reading an import run
// and its report is open to an agent now — a migration an assistant is helping
// with is unreadable to it otherwise, which was the whole gap. What bounds it
// is the object grant, auth.Require(migration.ImportRunObject, …), taken by
// every caller of this and unchanged: an agent with no import grant still gets
// nothing, and the run store still answers not-found for another workspace's
// run.
//
// Two operations keep RequireHuman, each for its own reason. uploadImportSource
// is multipart and has no agent-shaped door at all. undoImportRun reverses a
// committed estate-wide write, and reversing is not something a caller should
// reach without a contact present.
func (h importHandlers) stagedFor(ctx context.Context, id openapi_types.UUID) (migration.Run, error) {
	run, err := migration.NewRunStore(h.db).GetStaged(ctx, migration.RunID(id))
	if err != nil {
		return migration.Run{}, err
	}
	// The flip writes its own runs to the same table, and they carry no
	// mapping — no object, no columns, nothing this surface can describe. They
	// answer not-found here rather than being served as an import with an
	// empty `object` the contract's own enum forbids.
	if run.Connector != migration.ConnectorCSV {
		return migration.Run{}, fmt.Errorf("import run %s: %w", id, apperrors.ErrNotFound)
	}
	return run, nil
}

// errUploadLimitUnset reports that this composition never told the import
// handler its ceiling. A wiring fault, not a request fault, so it answers 500
// rather than refusing the caller's file for a size nobody set — the same guard
// the attachment and LinkedIn routes carry.
var errUploadLimitUnset = errors.New("compose: no upload ceiling configured for the import route")

// errNoObjectStore refuses an import on a process role that stores no objects.
// A conflict rather than a 500: the installation is configured this way, and a
// nil store reached later would be a panic inside a run that already exists.
var errNoObjectStore = fmt.Errorf("this role stores no objects, so it cannot run an import: %w", apperrors.ErrConflict)

// ownsSource refuses a source reference minted for another installation.
//
// The reference is a blobstore key and the blobstore treats keys as opaque
// bytes — it enforces no tenant boundary of its own, by design (the key IS the
// boundary, see blobstore.WorkspaceKey). So the only thing standing between a
// caller and another workspace's uploaded file is this check: without it, a
// reference obtained anywhere could be dry-run and approved here, importing
// somebody else's estate into this one.
func (h importHandlers) ownsSource(ctx context.Context, sourceRef string) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return fmt.Errorf("no workspace is bound to this request: %w", apperrors.ErrPermissionDenied)
	}
	if sourceRef != blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](ws), importBlobKind, path.Base(sourceRef)) {
		// Not-found, not forbidden: a caller may not learn whether a reference
		// they were never given exists.
		return fmt.Errorf("import source %q: %w", sourceRef, apperrors.ErrNotFound)
	}
	return nil
}
