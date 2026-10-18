// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge is told on the surviving record as well as on the archived one. The
// survivor's history is where a reader finds why it holds a value nobody typed.

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mergeRowsOn returns the after-images of the merge audit rows written for one
// entity, oldest first.
func (e *dedupeEnv) mergeRowsOn(ctx context.Context, t *testing.T, entityType string, id ids.UUID) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT after FROM audit_log
			 WHERE entity_type = $1 AND entity_id = $2 AND action = 'merge'
			 ORDER BY occurred_at, id`, entityType, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var after map[string]any
			if err := json.Unmarshal(raw, &after); err != nil {
				return err
			}
			out = append(out, after)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the %s merge rows: %v", entityType, err)
	}
	return out
}

func (e *dedupeEnv) requireMergedFrom(t *testing.T, rows []map[string]any, source ids.UUID) {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("the survivor holds %d merge rows, want 1: %v", len(rows), rows)
	}
	if got := rows[0][auditKeyMergedFrom]; got != source.String() {
		t.Errorf("the survivor's merge row names %v as the record folded in, want %s", got, source)
	}
}

func TestAContactMergeIsToldOnTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	title := "Chief Merge Officer"
	survivor := e.seedContact(ctx, t, "Ann Smith", []string{"ann@newco.test"}, nil)
	folded, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "A. Smith", Title: &title, Source: "manual",
		Emails: []ContactEmailInput{{Email: "a.smith@newco.test", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatalf("seed the contact to fold in: %v", err)
	}
	merged := ids.From[ids.ContactKind](ids.UUID(folded.Id))

	if _, err := e.store.MergeContact(ctx, merged, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	rows := e.mergeRowsOn(ctx, t, "contact", survivor.UUID)
	e.requireMergedFrom(t, rows, merged.UUID)
	if rows[0]["title"] != title {
		t.Errorf("the survivor's row holds title %v, want the filled %q as its own key", rows[0]["title"], title)
	}
	if rows[0][auditKeyRelinkedEmails] != float64(1) {
		t.Errorf("the survivor's row counts %v emails moved, want 1", rows[0][auditKeyRelinkedEmails])
	}
	if _, nested := rows[0][auditKeyFilled]; nested {
		t.Errorf("the survivor's row nests the filled values under %q: %v", auditKeyFilled, rows[0])
	}

	archived := e.mergeRowsOn(ctx, t, "contact", merged.UUID)
	if len(archived) != 1 || archived[0][auditKeyMergedInto] != survivor.UUID.String() {
		t.Fatalf("the archived contact's row = %v, want it to name %s", archived, survivor.UUID)
	}
	if got := archived[0][auditKeyFilled]; !reflect.DeepEqual(got, []any{"title"}) {
		t.Errorf("the archived contact's row names filled %v, want [title] and no values", got)
	}
	if _, leaked := archived[0]["title"]; leaked {
		t.Errorf("the archived contact's row carries the filled value: %v", archived[0])
	}
}

func TestACompanyMergeIsToldOnTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	source := e.seedCompanyOnDomain(ctx, t, "Newco", "newco.test")
	target := e.seedCompanyOnDomain(ctx, t, "Newco Ltd", "")

	if _, err := e.store.MergeCompany(ctx, source, target, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	rows := e.mergeRowsOn(ctx, t, "company", target.UUID)
	e.requireMergedFrom(t, rows, source.UUID)
	if _, nested := rows[0][auditKeyFilled]; nested {
		t.Errorf("the survivor's row nests the filled values under %q: %v", auditKeyFilled, rows[0])
	}
}

func TestALeadMergeIsToldOnTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor := e.createLead(ctx, t, "Karin Vogt", "karin@vogt.test", "Vogt KG")
	loser := e.createLead(ctx, t, "Karin Voigt", "", "Vogt KG")

	if _, err := e.store.MergeLead(ctx, loser, survivor); err != nil {
		t.Fatalf("merge: %v", err)
	}
	rows := e.mergeRowsOn(ctx, t, entityLead, survivor.UUID)
	e.requireMergedFrom(t, rows, loser.UUID)
	if _, nested := rows[0][auditKeyFilled]; nested {
		t.Errorf("the survivor's row nests the filled values under %q: %v", auditKeyFilled, rows[0])
	}
}

// A colleague folding a record into an imported contact is a human act on it.
// Undoing the import therefore leaves the contact alone.
func TestAMergeIntoAContactCountsAsAHumanActOnIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.editorCtx()
	survivor := e.seedContact(ctx, t, "Ann Smith", []string{"ann@newco.test"}, nil)
	merged := e.seedContact(ctx, t, "A. Smith", []string{"a.smith@newco.test"}, nil)
	cutoff := time.Now().UTC()
	acted := func() bool {
		var touched bool
		if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
			touched, err = storekit.HumanActedSince(ctx, tx, "contact", survivor.UUID, cutoff)
			return err
		}); err != nil {
			t.Fatalf("asking whether a human acted: %v", err)
		}
		return touched
	}
	if acted() {
		t.Fatal("a human is counted as acting on the survivor before the merge")
	}

	if _, err := e.store.MergeContact(ctx, merged, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !acted() {
		t.Error("the merge into the survivor is not counted as a human act on it")
	}
}
