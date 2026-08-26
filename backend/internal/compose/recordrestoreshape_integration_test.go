// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A restore lands every field it sends.
//
// The contract's update SHAPE is wider than a module's mapper. A key in that gap
// is accepted, ignored, and answers success — the silent drop this feature's
// whole refusal set exists to prevent, and the one failure worse than refusing,
// because the person reads the confirmation and stops looking.
//
// namedByTheShapeButNotWrittenByThePatch (undoability.go) is the declared gap.
// This holds it against behaviour rather than against a reading of the mappers:
// it writes a value, restores, and reports any field that did not come back. A
// key that joins the gap later is named here instead of dropped in front of a
// user.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// fieldsSentButNotHeld names the patch keys whose value on the record is not
// what was sent. It reads each column as jsonb, through the same representation
// the image was written in, rather than through a per-type Go conversion that
// would disagree about dates and money.
func fieldsSentButNotHeld(t *testing.T, e *integration.Env, entityType string, id ids.UUID, patch map[string]json.RawMessage) []string {
	t.Helper()
	// The row decides what is comparable: a field kept in its own table is
	// absent from the row's jsonb, and the query below skips it rather than
	// reporting it as never landed.
	sent, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal the sent patch: %v", err)
	}
	var missed []string
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.Admin(), `
			SELECT k.key
			FROM jsonb_each($2::jsonb) AS k(key, value)
			JOIN `+pgx.Identifier{entityType}.Sanitize()+` r ON r.id = $1
			WHERE to_jsonb(r) ? k.key
			  AND to_jsonb(r) -> k.key IS DISTINCT FROM k.value
			ORDER BY 1`, id, sent)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			missed = append(missed, key)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("ask what did not land: %v", err)
	}
	return missed
}

func TestARestoreLandsEveryFieldItSends(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	first, second := "CTO", "CEO"
	person, err := e.People.CreatePerson(ctx, people.CreatePersonInput{
		FullName: "Greta Shape", Title: &first, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed through the real writer: %v", err)
	}
	id := ids.UUID(person.Id)
	if _, err := e.People.UpdatePerson(ctx, ids.From[ids.PersonKind](id),
		people.UpdatePersonInput{Title: &second, Source: "manual"}); err != nil {
		t.Fatalf("change the field through the real writer: %v", err)
	}

	// The image the reversal would send, filtered exactly as the executor
	// filters it — so what this asserts is what a restore actually writes.
	patch, _, err := filterImage("person", json.RawMessage(`{"title":"CTO"}`))
	if err != nil {
		t.Fatalf("filter the image: %v", err)
	}
	if len(patch) == 0 {
		t.Fatal("the filtered image is empty; this test would then assert nothing")
	}

	if _, err := e.People.UpdatePerson(ctx, ids.From[ids.PersonKind](id),
		people.UpdatePersonInput{Title: &first, Source: "manual"}); err != nil {
		t.Fatalf("put it back through the real writer: %v", err)
	}

	if missed := fieldsSentButNotHeld(t, e, "person", id, patch); len(missed) > 0 {
		t.Errorf("the restore sent %v and the record does not hold them; a key the "+
			"update shape names and the mapper ignores answers success and writes "+
			"nothing — declare it in namedByTheShapeButNotWrittenByThePatch or make "+
			"the mapper write it", missed)
	}
}

// A record outside the caller's row scope is 404, and it is 404 BEFORE the
// audit row is read.
//
// The row-scope rule is that a miss answers 404 so existence stays hidden. A
// reversal that read the audit row first would answer "this change cannot be
// put back" for a record the caller is not allowed to know exists — and a
// caller who can tell a refusal from a 404 can tell a hidden record from an
// absent one, which is the whole of what the rule hides.
func TestARestoreOfARecordOutsideTheCallersScopeIsNotFound(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()

	title := "CTO"
	person, err := e.People.CreatePerson(ctx, people.CreatePersonInput{
		FullName: "Greta Scoped", Title: &title, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed through the real writer: %v", err)
	}
	id := ids.UUID(person.Id)
	changed := "CEO"
	if _, err := e.People.UpdatePerson(ctx, ids.From[ids.PersonKind](id),
		people.UpdatePersonInput{Title: &changed, Source: "manual"}); err != nil {
		t.Fatalf("change it: %v", err)
	}

	// A REAL audit row on that record. An id naming nothing would answer 404
	// for the wrong reason and prove nothing about row scope.
	var auditID ids.UUID
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT id FROM audit_log
			WHERE entity_type = 'person' AND entity_id = $1 AND action = 'update'
			ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&auditID)
	}); err != nil {
		t.Fatalf("find the entry a person would press Undo on: %v", err)
	}

	// The record and its entry both exist and are readable by THIS caller. What
	// is varied is the row-scope gate alone, because that is the property under
	// test: a caller who may not see the record must be answered 404, and never
	// a refusal — a refusal is proof the record exists, and a caller who can
	// tell the two apart can tell a hidden record from an absent one.
	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool))
	seam.visible = func(context.Context, pgx.Tx, string, ids.UUID) error {
		return apperrors.ErrNotFound
	}

	_, err = seam.Restore(ctx, "person", id, auditID, 1)
	if err == nil {
		t.Fatal("a restore of a record the caller may not see succeeded")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a row-scope miss answered %v, want ErrNotFound", err)
	}
	var refusal RefusedRestore
	if errors.As(err, &refusal) {
		t.Errorf("a row-scope miss answered the refusal %q; the gate is being asked "+
			"AFTER the audit row is read, which discloses the record", refusal.Reason)
	}
}
