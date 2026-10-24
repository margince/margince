// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedPreRuleDuplicates writes one company pair and one contact pair through
// the create path, then removes the pairs those creates filed: the state of
// records written before the rule that files them.
func seedPreRuleDuplicates(t *testing.T, e *integration.Env) {
	t.Helper()
	for _, name := range []string{"Example Consulting GmbH", "Example Consulting", "Northwind Traders"} {
		if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	for name, email := range map[string]string{"Bjoern Example": "bjoern@first.example", "Björn Example": "bjorn@second.example"} {
		if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: name, Source: "manual",
			Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	e.WsExec(t, `DELETE FROM dedupe_candidate`)
}

func scannedPairs(t *testing.T, e *integration.Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE source = 'dedupe_scan' AND disposition = 'open'`)
}

func TestTheDuplicateScanTaskFilesPairsWrittenBeforeTheRuleAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	seedPreRuleDuplicates(t, e)

	task := dedupeScanDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	runMachineAddressTask(ctx, t, e, task)
	if got := scannedPairs(t, e); got != 2 {
		t.Fatalf("the task filed %d pairs, want the company pair and the contact pair", got)
	}

	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'dedupe_candidate'`)
	for _, step := range dedupeScanDataTask().Steps {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
			if err == nil && out.Changed != 0 {
				t.Errorf("a replay of %s from a nil cursor filed %d pairs", step.Name, out.Changed)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'dedupe_candidate'`); again != audits {
		t.Errorf("a replay wrote audit rows: %d -> %d", audits, again)
	}

	count, more, err := dedupeScanDataTask().Steps[0].Estimate(ctx, e.Pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !more {
		t.Errorf("the estimate capped at 1 reads (%d, more=%v), want (1, true) over three companies", count, more)
	}
}

func TestTheDuplicateScanTaskRollsBackWithItsBatch(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	seedPreRuleDuplicates(t, e)
	rollback := errors.New("roll back after the write")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := dedupeScanDataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Errorf("the batch filed %d pairs, want the company pair", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate`); got != 0 {
		t.Errorf("a rolled-back batch left %d pairs on the queue", got)
	}
}

func TestTheDuplicateScanTaskRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := dedupeScanDataTask().Steps[1].Batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"not an id"`), Limit: 10})
		return err
	})
	if err == nil {
		t.Fatal("a batch accepted a cursor that names no record")
	}
}

// The nightly job files the pairs of records changed since its last pass, and
// only after the open queue has been re-checked.
func TestTheNightlyDuplicateJobFilesThePairsOfChangedRecords(t *testing.T) {
	e := integration.Setup(t)
	seedPreRuleDuplicates(t, e)
	later, err := time.Parse(time.RFC3339Nano, e.WsScalar(t, `SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC' + interval '1 hour', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`))
	if err != nil {
		t.Fatal(err)
	}
	worker := &dedupeRecheckWorker{pool: e.Pool, now: func() time.Time { return later }}
	if err := worker.recheckWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the nightly pass: %v", err)
	}
	if got := scannedPairs(t, e); got != 2 {
		t.Errorf("the nightly pass filed %d pairs, want the company pair and the contact pair", got)
	}
}
