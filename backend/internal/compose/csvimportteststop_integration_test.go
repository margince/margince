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

func TestATestRunStoppedByItsRequestDeadlineIsStillRecordedAsFailed(t *testing.T) {
	e := integration.Setup(t)
	runs := migration.NewRunStore(e.DB())
	run, err := runs.CreateStagedRun(importAdmin(e), migration.CreateStagedRunInput{
		Connector: migration.ConnectorCSV, SourceRef: "ws/import/x", Source: importSourceProvenance,
		Mapping: migration.RunMapping{Object: migration.ObjectLead, Fields: map[string]string{"Email": "email"}, SourceKey: "Email"},
	})
	if err != nil {
		t.Fatalf("staging a run: %v", err)
	}

	expired, release := context.WithDeadline(importAdmin(e), time.Unix(0, 0))
	defer release()
	failValidation(expired, runs, run.ID, context.DeadlineExceeded)

	if status := statusOf(t, e, run.ID); status != migration.StatusFailed {
		t.Fatalf("status = %q, want failed: a run left validating shows as checking for good", status)
	}
}

func TestATestRunPastItsBudgetIsRefusedByNameAndRecordedAsFailed(t *testing.T) {
	e := integration.Setup(t)
	h := importHandlers{db: e.DB(), blobs: blobstore.NewMemory(), uploadLimit: 1 << 20, testRunBudget: time.Nanosecond}
	profile, err := h.profileAndStore(importAdmin(e), migration.ObjectLead, []byte("Email,Full Name\nada@lovelace.example,Ada\n"))
	if err != nil {
		t.Fatalf("storing the source: %v", err)
	}
	object := crmcontracts.ImportObject(migration.ObjectLead)
	mapping := map[string]string{"Email": "email", "Full Name": "full_name"}

	_, err = h.stageRun(importAdmin(e), crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object, SourceRef: profile.SourceRef, Mapping: &mapping,
	})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "test_run_too_long" {
		t.Fatalf("err = %v, want the test_run_too_long refusal", err)
	}

	listed, _, err := migration.NewRunStore(e.DB()).ListStaged(importAdmin(e), "", 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("runs = %+v (%v), want the one this attempt opened", listed, err)
	}
	if listed[0].Status != migration.StatusFailed {
		t.Fatalf("status = %q, want failed", listed[0].Status)
	}
}

func TestAFileOverTheTestRunsRowCapIsRefusedAndRecordedAsFailed(t *testing.T) {
	e := integration.Setup(t)
	h := importHandlers{db: e.DB(), blobs: blobstore.NewMemory(), uploadLimit: 10_000_000}
	var file strings.Builder
	file.WriteString("Email,Full Name\n")
	for i := range importTestRunRowCap + 1 {
		fmt.Fprintf(&file, "p%d@cap.example,P %d\n", i, i)
	}
	profile, err := h.profileAndStore(importAdmin(e), migration.ObjectLead, []byte(file.String()))
	if err != nil {
		t.Fatalf("storing the source: %v", err)
	}
	object := crmcontracts.ImportObject(migration.ObjectLead)
	mapping := map[string]string{"Email": "email", "Full Name": "full_name"}

	_, err = h.stageRun(importAdmin(e), crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnectorCsv, Object: &object, SourceRef: profile.SourceRef, Mapping: &mapping,
	})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "too_many_rows_for_test_run" ||
		!strings.Contains(refusal.Detail, fmt.Sprint(importTestRunRowCap+1)) {
		t.Fatalf("err = %v, want the row-cap refusal naming the file's row count", err)
	}
	if listed, _, err := migration.NewRunStore(e.DB()).ListStaged(importAdmin(e), "", 10); err != nil ||
		len(listed) != 1 || listed[0].Status != migration.StatusFailed {
		t.Fatalf("runs = %+v (%v), want the one run, failed", listed, err)
	}
}
