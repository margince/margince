// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importAdmin is the harness admin holding the whole import_run grant, which
// the harness admin does not.
func importAdmin(e *integration.Env) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects[migration.ImportRunObject] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	return e.As(e.AdminUser, nil, perms)
}

// statusOf reads a run back on a live context.
func statusOf(t *testing.T, e *integration.Env, id migration.RunID) string {
	t.Helper()
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), id)
	if err != nil {
		t.Fatalf("reading run %s: %v", id, err)
	}
	return run.Status
}

// leadSourceHandlers stores a small lead file the way an upload does, opening
// its draft run. Its test runs queue on the real inserter.
func leadSourceHandlers(t *testing.T, e *integration.Env) (importHandlers, crmcontracts.ImportSourceProfile) {
	t.Helper()
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("building the job inserter: %v", err)
	}
	h := importHandlers{db: e.DB(), blobs: blobstore.NewMemory(), uploadLimit: 1 << 20, queue: &importEnqueuer{runner: inserter}}
	profile, err := h.profileAndStore(importAdmin(e), migration.ObjectLead, "leads.csv",
		[]byte("Email,Full Name\nada@lovelace.example,Ada\n"))
	if err != nil {
		t.Fatalf("storing the source: %v", err)
	}
	return h, profile
}

// queuedLeadTest starts the test run of the lead file as the handler does, and
// answers the job's args.
func queuedLeadTest(t *testing.T, e *integration.Env, h importHandlers, profile crmcontracts.ImportSourceProfile) ImportValidateArgs {
	t.Helper()
	object := crmcontracts.ImportObject(migration.ObjectLead)
	mapping := map[string]string{"Email": "email", "Full Name": "full_name"}
	queued, err := h.stageRun(importAdmin(e), crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object, SourceRef: profile.SourceRef, Mapping: &mapping,
	})
	if err != nil || queued.Status != crmcontracts.ImportRunStatusValidating {
		t.Fatalf("starting the test run: %s (%v), want validating", queued.Status, err)
	}
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), migration.RunID(profile.RunId))
	if err != nil {
		t.Fatalf("reading the queued run: %v", err)
	}
	return ImportValidateArgs{Workspace: e.WS, RunID: run.ID, Attempt: run.JobAttempt}
}

func validateWorkerFor(e *integration.Env, h importHandlers) *importValidateWorker {
	return &importValidateWorker{
		slicer: newImportSlicer(e.Pool, slog.New(slog.DiscardHandler)), blobs: h.blobs, uploadCap: h.uploadLimit,
	}
}

func TestARefusedTestRunReturnsToDraftWithItsReason(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	h, profile := leadSourceHandlers(t, e)
	args := queuedLeadTest(t, e, h, profile)
	if err := h.blobs.Delete(context.Background(), profile.SourceRef); err != nil {
		t.Fatalf("losing the stored file: %v", err)
	}

	if err := validateWorkerFor(e, h).Work(context.Background(), &river.Job[ImportValidateArgs]{Args: args}); err != nil {
		t.Fatalf("the test run answered %v, want nil once its run says why it stopped", err)
	}
	run, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), args.RunID)
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	want, _ := httperr.Classify(importProblem(blobstore.ErrNotFound))
	if run.Status != migration.StatusPending || run.Error != want.Detail || run.Progress != nil {
		t.Fatalf("run = %s with %q and progress %+v, want pending with the refusal's reason %q",
			run.Status, run.Error, run.Progress, want.Detail)
	}
}

func TestUploadCreatesADraftAndRefusesOversizeWithoutARun(t *testing.T) {
	e := integration.Setup(t)
	h, profile := leadSourceHandlers(t, e)
	draft, err := migration.NewRunStore(e.DB()).GetStaged(importAdmin(e), migration.RunID(profile.RunId))
	if err != nil || draft.Status != migration.StatusPending || draft.SourceRef != profile.SourceRef || len(draft.Profile) == 0 {
		t.Fatalf("draft = %+v (%v), want a pending run holding the upload and its profile", draft, err)
	}
	if profile.RowCount != 1 {
		t.Errorf("row_count = %d, want 1", profile.RowCount)
	}

	var file strings.Builder
	file.WriteString("Email,Full Name\n")
	for i := range importRowCap + 1 {
		fmt.Fprintf(&file, "p%d@cap.example,P %d\n", i, i)
	}
	h.uploadLimit = 10_000_000
	_, err = h.profileAndStore(importAdmin(e), migration.ObjectLead, "", []byte(file.String()))
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "too_many_rows" ||
		!strings.Contains(refusal.Detail, fmt.Sprint(importRowCap+1)) {
		t.Fatalf("err = %v, want the row-cap refusal naming the file's row count", err)
	}
	if listed, _, err := migration.NewRunStore(e.DB()).ListStaged(importAdmin(e), "", 10, migration.ViewAll); err != nil || len(listed) != 1 {
		t.Fatalf("runs = %+v (%v), want only the first upload's draft", listed, err)
	}
}

func TestRunListViewsSeparateDraftsFromHistory(t *testing.T) {
	e := integration.Setup(t)
	leadSourceHandlers(t, e)
	store := migration.NewRunStore(e.DB())
	for view, want := range map[migration.RunView]int{
		migration.ViewAll: 1, migration.ViewDrafts: 1, migration.ViewHistory: 0,
	} {
		listed, _, err := store.ListStaged(importAdmin(e), "", 10, view)
		if err != nil || len(listed) != want {
			t.Errorf("view %q listed %d runs (%v), want %d", view, len(listed), err, want)
		}
	}
}
