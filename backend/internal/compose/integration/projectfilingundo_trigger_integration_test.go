// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The two retention triggers know exactly one door for undoing a project filing,
// and these cases lean on every other way in. They run as the schema owner
// against the real tables, because what is under test is the database refusing a
// writer that gets the Go side wrong.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	declareUndo       = `SELECT set_config('` + activities.UndoDeclarationSetting + `', $1, true)`
	unlinkProject     = `DELETE FROM activity_link WHERE activity_id = $1 AND entity_type = 'project'`
	deleteFilingProof = `DELETE FROM activity_retention_evidence WHERE activity_id = $1 AND basis = 'project_linked'`
	clearTheClass     = `UPDATE activity SET retention_class = NULL, retention_class_at = NULL WHERE id = $1`
)

type step struct {
	sql  string
	args []any
}

// constraintOf names the constraint a refusal came from. A database error with
// none is not a refusal this suite can read, so it fails the test rather than
// passing for "allowed".
func constraintOf(t *testing.T, sql string, err error) string {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName == "" {
		t.Fatalf("%s: %v is not a constraint refusal", sql, err)
	}
	return pgErr.ConstraintName
}

// refusedBy runs the steps in one owner transaction and names the constraint
// that refused it, or "" when every step passed. The transaction always rolls
// back: a case that is allowed is proved by the service tests, and this one
// must leave the fixture standing for the next.
func refusedBy(t *testing.T, owner *pgx.Conn, steps ...step) string {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back: %v", err)
		}
	}()
	for _, s := range steps {
		if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
			return constraintOf(t, s.sql, err)
		}
	}
	return ""
}

// refusedAtCommit runs the steps and COMMITS, for a refusal a deferred trigger
// raises when the transaction ends. A commit that went through is undone by
// the caller's fixture, not here: the cases that use it expect a refusal.
func refusedAtCommit(t *testing.T, owner *pgx.Conn, steps ...step) string {
	t.Helper()
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range steps {
		if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
			if rollback := tx.Rollback(ctx); rollback != nil {
				t.Errorf("rolling back: %v", rollback)
			}
			return constraintOf(t, s.sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return constraintOf(t, "COMMIT", err)
	}
	return ""
}

// A transaction that declares the undo can still not strip the proof alone: the
// evidence leaves only once the activity is off its project, and a class never
// commits with no evidence behind it. The database also refuses the clear of a
// class by a writer that never declared anything, when nothing else would stop it.
func TestADeclarationAloneDoesNotStripTheProofOrTheClass(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := filedUnderAProject(t, e)
	id := f.email
	declared := step{declareUndo, []any{id.String()}}

	// FIRST, on a connection that has never seen the setting: there it reads as NULL,
	// not as an empty string, and a guard that treats unknown as allowed falls through.
	// With no declaration at all, a class on an activity nothing else holds is
	// still not clearable: an unset setting must read as no declaration, not as
	// an unknown that lets the guard fall through.
	bare := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		retention_class, retention_class_at)
		VALUES ($1, 'email', 'classed', 'body', now(), 'manual', 'human:x', 'commercial_correspondence', now())`, bare)
	if got := refusedBy(t, owner, step{clearTheClass, []any{bare}}); got != "activity_retention_class_monotonic" {
		t.Errorf("clearing the class with no declaration, no evidence and no link was refused by %q", got)
	}

	if got := refusedBy(t, owner, declared, step{deleteFilingProof, []any{id}}); got != "activity_retention_evidence_frozen" {
		t.Errorf("a declared delete while the activity is still filed was refused by %q", got)
	}
	if got := refusedAtCommit(t, owner, declared, step{unlinkProject, []any{id}}, step{deleteFilingProof, []any{id}}); got != "activity_class_needs_evidence" {
		t.Errorf("unlinking and deleting the proof but leaving the class committed (%q), want the deferred check to refuse", got)
	}
	theFilingStands(t, e, f)
}

func TestTheTriggersAdmitOnlyTheDeclaredUndoOfAProjectFiling(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := filedUnderAProject(t, e)
	id := f.email
	declared := step{declareUndo, []any{id.String()}}
	otherActivity := step{declareUndo, []any{ids.NewV7().String()}}

	cases := []struct {
		name  string
		steps []step
		want  string
	}{
		{"clearing the class with no declaration", []step{{unlinkProject, []any{id}}, {clearTheClass, []any{id}}}, "activity_retention_class_monotonic"},
		{"deleting the evidence with no declaration", []step{{deleteFilingProof, []any{id}}}, "activity_retention_evidence_frozen"},
		{"a declaration naming another activity", []step{otherActivity, {unlinkProject, []any{id}}, {deleteFilingProof, []any{id}}}, "activity_retention_evidence_frozen"},
		{"clearing while the evidence remains", []step{declared, {unlinkProject, []any{id}}, {clearTheClass, []any{id}}}, "activity_retention_class_monotonic"},
		{"deleting the proof while the activity is still filed", []step{declared, {deleteFilingProof, []any{id}}, {clearTheClass, []any{id}}}, "activity_retention_evidence_frozen"},
		{"clearing the date and keeping the class", []step{
			declared,
			{unlinkProject, []any{id}},
			{deleteFilingProof, []any{id}},
			{`UPDATE activity SET retention_class_at = NULL WHERE id = $1`, []any{id}},
		}, "activity_retention_class_monotonic"},
		{"clearing the class and keeping the date", []step{
			declared,
			{unlinkProject, []any{id}},
			{deleteFilingProof, []any{id}},
			{`UPDATE activity SET retention_class = NULL WHERE id = $1`, []any{id}},
		}, "activity_retention_class_monotonic"},
		{"moving the date", []step{declared, {`UPDATE activity SET retention_class_at = now() + interval '1 day' WHERE id = $1`, []any{id}}}, "activity_retention_class_monotonic"},
		{"deleting a basis other than the project filing", []step{
			declared,
			{`INSERT INTO activity_retention_evidence (activity_id, basis, qualified_at, decided_by, decided_by_name, reason)
				VALUES ($1, 'controller_pin', now(), $2, 'Rep', 'pinned')`, []any{id, e.Rep1}},
			{`DELETE FROM activity_retention_evidence WHERE activity_id = $1 AND basis = 'controller_pin'`, []any{id}},
		}, "activity_retention_evidence_frozen"},
		{"a declaration on a restricted activity", []step{
			{`UPDATE activity SET restricted_at = now(), restricted_until = now() + interval '6 years', restricted_reason = 'commercial_correspondence', archived_at = now() WHERE id = $1`, []any{id}},
			declared,
			{deleteFilingProof, []any{id}},
		}, "activity_retention_evidence_frozen"},
	}
	for _, tc := range cases {
		if got := refusedBy(t, owner, tc.steps...); got != tc.want {
			t.Errorf("%s: refused by %q, want %q", tc.name, got, tc.want)
		}
	}

	// The declared undo itself passes, so the refusals above are the guards and
	// not a trigger that refuses everything.
	if got := refusedBy(t, owner, declared, step{unlinkProject, []any{id}}, step{deleteFilingProof, []any{id}}, step{clearTheClass, []any{id}}); got != "" {
		t.Errorf("the declared undo was refused by %q", got)
	}
	theFilingStands(t, e, f)
}
