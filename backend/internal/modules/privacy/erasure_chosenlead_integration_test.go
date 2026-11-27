// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedChosenOriginal files a chat message on the given records through
// activity_link, with an original whose payload names none of them.
func seedChosenOriginal(t *testing.T, env *sarIdentifierEnv, key string, leads, contacts []ids.UUID) {
	t.Helper()
	ctx := context.Background()
	raw, activity := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, env.owner, `INSERT INTO raw_capture (id, source_system, source_id, payload) VALUES ($1, 'ext:unit:chat', $2, '{"id":"x"}')`, raw, key)
	mustExec(ctx, t, env.owner, `INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by, audience, raw_capture_id)
		VALUES ($1, 'note', 's', 'b', 'inbound', 'ext:unit:chat', $2, 'ext:unit:chat', 'connector:ext:unit', 'workspace', $3)`, activity, key, raw)
	for _, lead := range leads {
		mustExec(ctx, t, env.owner, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, activity, lead)
	}
	for _, contact := range contacts {
		mustExec(ctx, t, env.owner, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, activity, contact)
	}
}

// chosenKey names one run's original, so a second run on the same database
// files a new one instead of colliding with the first.
func chosenKey(name string) string {
	return "chosen-" + name + "-" + ids.NewV7().String()
}

func originalsHeld(t *testing.T, env *sarIdentifierEnv, key string) int {
	t.Helper()
	var n int
	if err := env.owner.QueryRow(context.Background(), `SELECT count(*) FROM raw_capture WHERE source_id = $1`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedPlainLead(t *testing.T, env *sarIdentifierEnv, name string) ids.UUID {
	t.Helper()
	lead := ids.NewV7()
	mustExec(context.Background(), t, env.owner, `INSERT INTO lead (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'system:test')`, lead, name)
	return lead
}

func TestAnonymizingALeadRemovesTheOriginalsFiledOnItByChoice(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	lead := seedPlainLead(t, env, "Aged")
	key := chosenKey("anon")
	seedChosenOriginal(t, env, key, []ids.UUID{lead}, nil)

	retention := &RetentionService{eraser: NewEraser(env.db)}
	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error { return retention.anonymizeLead(env.ctx, tx, lead) }); err != nil {
		t.Fatal(err)
	}
	if n := originalsHeld(t, env, key); n != 0 {
		t.Fatalf("%d originals survive the anonymization of the lead they were filed on", n)
	}
}

func TestDeletingAnUntoldLeadRemovesTheOriginalsFiledOnItByChoice(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	lead := seedPlainLead(t, env, "Untold")
	mustExec(context.Background(), t, env.owner,
		`INSERT INTO lead_privacy_notice (lead_id, template_key, template_version, state, failure, failed_at) VALUES ($1, 'site_lead', 1, 'failed', 'refused', now())`, lead)
	key := chosenKey("untold")
	seedChosenOriginal(t, env, key, []ids.UUID{lead}, nil)

	err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		return EraseUntoldLeadTx(env.ctx, tx, UntoldLead{LeadID: ids.From[ids.LeadKind](lead)})
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := originalsHeld(t, env, key); n != 0 {
		t.Fatalf("%d originals survive the deletion of the lead they were filed on", n)
	}
}

// A message filed on a lead and a live contact keeps its original when only
// the lead is erased.
func TestAnOriginalFiledOnALeadAndAContactSurvivesTheLeadsErasure(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	lead := seedPlainLead(t, env, "Shared")
	contact := ids.NewV7()
	mustExec(context.Background(), t, env.owner, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Kept', 'manual', 'system:test')`, contact)
	key := chosenKey("both")
	seedChosenOriginal(t, env, key, []ids.UUID{lead}, []ids.UUID{contact})

	retention := &RetentionService{eraser: NewEraser(env.db)}
	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error { return retention.anonymizeLead(env.ctx, tx, lead) }); err != nil {
		t.Fatal(err)
	}
	if n := originalsHeld(t, env, key); n != 1 {
		t.Fatalf("%d originals after erasing only the lead, want the one the contact is still owed", n)
	}
}
