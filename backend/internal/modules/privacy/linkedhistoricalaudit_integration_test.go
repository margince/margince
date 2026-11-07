// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLinkedHistoricalCorrectionUsesTheOriginalBoundaryAndKeepsLaterWork(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, pair, _ := seedDedupePairs(ctx, t, tx)
	original := seedPersonalAudit(ctx, t, tx, "dedupe_candidate", pair)
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	later := seedPersonalAudit(ctx, t, tx, "dedupe_candidate", pair)
	mustExec(ctx, t, tx, `INSERT INTO event_outbox(stream,envelope) VALUES('gw:events:crm:contact',jsonb_build_object('entity',jsonb_build_object('type','contact','id',@subject::uuid::text),'trace',jsonb_build_object('audit_log_id',@audit::uuid::text),'payload',jsonb_build_object('name','Private Contact')))`, pgx.StrictNamedArgs{"subject": subject.contact, "audit": original})
	mustExec(ctx, t, tx, `SET LOCAL ROLE margince_app`)
	run := ids.NewV7()
	page, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, run, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed == 0 {
		t.Fatal("the old linked image was not inspected")
	}
	assertLegacyAuditImage(ctx, t, tx, original, false)
	assertLegacyAuditImage(ctx, t, tx, later, true)
	var events, boundaries int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->'trace'->>'audit_log_id'=@id::uuid::text`, pgx.StrictNamedArgs{"id": original}).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type='dedupe_candidate' AND entity_id=@pair AND action=ANY(@verbs)`, pgx.StrictNamedArgs{"pair": pair, "verbs": storekit.AuditRedactionActions()}).Scan(&boundaries); err != nil {
		t.Fatal(err)
	}
	if events != 0 || boundaries != 0 {
		t.Fatalf("events=%d new domain boundaries=%d", events, boundaries)
	}
	replay, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, run, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Changed != 0 {
		t.Fatalf("replay changed %d entries", replay.Changed)
	}
}

func TestLinkedHistoricalCorrectionKeepsCurrentSubjectHolds(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, pair, _ := seedDedupePairs(ctx, t, tx)
	original := seedPersonalAudit(ctx, t, tx, "dedupe_candidate", pair)
	legacyContactProof(ctx, t, tx, subject.contact.UUID)
	mustExec(ctx, t, tx, `UPDATE contact SET legal_hold=true WHERE id=@subject`, pgx.StrictNamedArgs{"subject": subject.contact})
	page, err := RedactLinkedHistoricalAuditPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Changed != 0 {
		t.Fatalf("changed %d held entries", page.Changed)
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

func TestARestrictionDoesNotCertifyPhysicalAuditErasure(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	original := seedPersonalAudit(ctx, t, tx, "contact", subject)
	proof, err := storekit.AuditWithEvidence(ctx, tx, "restrict", "contact", subject, nil, nil, map[string]any{"cause": "retention"})
	if err != nil {
		t.Fatal(err)
	}
	if err := storekit.RedactAuditHistory(ctx, tx, proof); err == nil {
		t.Fatal("a restriction authorized physical erasure")
	}
	assertLegacyAuditImage(ctx, t, tx, original, true)
}

func legacyContactProof(ctx context.Context, t *testing.T, tx pgx.Tx, subject ids.UUID) ids.UUID {
	t.Helper()
	// Upgrade fixtures retain the old images because earlier erasures wrote only a certificate.
	proof, err := storekit.AuditWithEvidence(ctx, tx, "erase", "contact", subject, nil, nil, map[string]any{"cause": "contact_erasure"})
	if err != nil {
		t.Fatal(err)
	}
	return proof
}

func assertLegacyAuditImage(ctx context.Context, t *testing.T, tx pgx.Tx, id ids.UUID, kept bool) {
	t.Helper()
	var personal bool
	if err := tx.QueryRow(ctx, `SELECT coalesce(after->>'full_name'='Private Contact',false) FROM audit_log WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&personal); err != nil {
		t.Fatal(err)
	}
	if personal != kept {
		t.Fatalf("audit %s personal values retained=%v, want %v", id, personal, kept)
	}
}

func TestPhysicalAuditActionsMatchTheDatabase(t *testing.T) {
	ctx := context.Background()
	tx := subjectColumnsTx(ctx, t)
	var stored []string
	if err := tx.QueryRow(ctx, `SELECT audit_redaction_actions()`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	expected := storekit.AuditRedactionActions()
	slices.Sort(stored)
	slices.Sort(expected)
	if !slices.Equal(stored, expected) {
		t.Fatalf("database actions=%q application actions=%q", stored, expected)
	}
}
