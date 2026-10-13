// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
// its draft run.
func leadSourceHandlers(t *testing.T, e *integration.Env, budget time.Duration) (importHandlers, crmcontracts.ImportSourceProfile) {
	t.Helper()
	h := importHandlers{db: e.DB(), blobs: blobstore.NewMemory(), uploadLimit: 1 << 20, testRunBudget: budget}
	profile, err := h.profileAndStore(importAdmin(e), migration.ObjectLead, "leads.csv",
		[]byte("Email,Full Name\nada@lovelace.example,Ada\n"))
	if err != nil {
		t.Fatalf("storing the source: %v", err)
	}
	return h, profile
}

func TestATestRunStoppedByItsRequestDeadlineStillReturnsToItsDraft(t *testing.T) {
	e := integration.Setup(t)
	_, profile := leadSourceHandlers(t, e, 0)
	runs := migration.NewRunStore(e.DB())
	id := migration.RunID(profile.RunId)
	if _, _, err := runs.QueueValidation(importAdmin(e), id, 1, testsInRequest); err != nil {
		t.Fatalf("starting the test run: %v", err)
	}

	expired, release := context.WithDeadline(importAdmin(e), time.Unix(0, 0))
	defer release()
	returnToDraft(expired, runs, id, context.DeadlineExceeded)

	run, err := runs.GetStaged(importAdmin(e), id)
	if err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	if run.Status != migration.StatusPending || run.Error != migration.CauseTestRunStopped {
		t.Fatalf("run = %s with %q, want pending with the fixed sentence: a run left validating shows as checking for good",
			run.Status, run.Error)
	}
}

func TestARefusedTestRunReturnsToDraftWithItsReason(t *testing.T) {
	e := integration.Setup(t)
	h, profile := leadSourceHandlers(t, e, time.Nanosecond)
	object := crmcontracts.ImportObject(migration.ObjectLead)
	mapping := map[string]string{"Email": "email", "Full Name": "full_name"}

	_, err := h.stageRun(importAdmin(e), crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object, SourceRef: profile.SourceRef, Mapping: &mapping,
	})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "test_run_too_long" {
		t.Fatalf("err = %v, want the test_run_too_long refusal", err)
	}

	listed, _, err := migration.NewRunStore(e.DB()).ListStaged(importAdmin(e), "", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != migration.RunID(profile.RunId) {
		t.Fatalf("runs = %+v (%v), want only the upload's draft", listed, err)
	}
	if listed[0].Status != migration.StatusPending || listed[0].Error != refusal.Detail {
		t.Fatalf("run = %s with %q, want pending with the refusal's reason %q", listed[0].Status, listed[0].Error, refusal.Detail)
	}
}

func TestUploadCreatesADraftAndRefusesOversizeWithoutARun(t *testing.T) {
	e := integration.Setup(t)
	h, profile := leadSourceHandlers(t, e, 0)
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
	if listed, _, err := migration.NewRunStore(e.DB()).ListStaged(importAdmin(e), "", 10); err != nil || len(listed) != 1 {
		t.Fatalf("runs = %+v (%v), want only the first upload's draft", listed, err)
	}
}
