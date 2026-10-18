// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A merge is told on the surviving record as well as on the archived one: the
// survivor's history is where a person reads why it now holds a value nobody
// typed there.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

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
	survivor := e.seedContact(ctx, t, "Ann Smith", []string{"ann@newco.test"}, nil)
	merged := e.seedContact(ctx, t, "A. Smith", []string{"a.smith@newco.test"}, nil)

	if _, err := e.store.MergeContact(ctx, merged, survivor, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	rows := e.mergeRowsOn(ctx, t, "contact", survivor.UUID)
	e.requireMergedFrom(t, rows, merged.UUID)
	if _, counted := rows[0]["relinked"]; !counted {
		t.Errorf("the survivor's row does not say what it took in: %v", rows[0])
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
	e.requireMergedFrom(t, e.mergeRowsOn(ctx, t, "company", target.UUID), source.UUID)
}

func TestALeadMergeIsToldOnTheSurvivor(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor := e.createLead(ctx, t, "Karin Vogt", "karin@vogt.test", "Vogt KG")
	loser := e.createLead(ctx, t, "Karin Voigt", "", "Vogt KG")

	if _, err := e.store.MergeLead(ctx, loser, survivor); err != nil {
		t.Fatalf("merge: %v", err)
	}
	e.requireMergedFrom(t, e.mergeRowsOn(ctx, t, entityLead, survivor.UUID), loser.UUID)
}
