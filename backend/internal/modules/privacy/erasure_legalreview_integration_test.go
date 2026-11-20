// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// A legal review's note and comments are colleagues' free text about a deal.
// Erasing or anonymizing a stakeholder clears them, and the Art. 15 export hands
// them to the same stakeholder.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	reviewNote     = "Please check the clause Sara Subject asked for"
	reviewComments = "Sara Subject's change is acceptable"
)

// seedReviewedDeal opens a deal with one stakeholder and one answered legal
// review carrying both free-text columns.
func seedReviewedDeal(ctx context.Context, t *testing.T, tx execer, stakeholder ids.UUID) (deal, review ids.UUID) {
	t.Helper()
	deal, review = ids.NewV7(), ids.NewV7()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Legal text "+pipeline.String())
	mustExec(ctx, t, tx, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Review', 1)`, stage, pipeline)
	mustExec(ctx, t, tx, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Reviewed deal', 'open', $2, $3, 'seed', 'system')`, deal, pipeline, stage)
	mustExec(ctx, t, tx, `INSERT INTO relationship (kind, contact_id, deal_id, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'manual', 'test')`, stakeholder, deal)
	mustExec(ctx, t, tx, `INSERT INTO legal_review (id, deal_id, status, note, comments, packet, decided_at, captured_by)
		VALUES ($1, $2, 'changes_needed', $3, $4, '{}'::jsonb, now(), 'test')`,
		review, deal, reviewNote, reviewComments)
	return deal, review
}

func reviewText(ctx context.Context, t *testing.T, tx pgx.Tx, review ids.UUID) (note, comments *string, status string) {
	t.Helper()
	if err := tx.QueryRow(ctx, `SELECT note, comments, status FROM legal_review WHERE id = $1`, review).
		Scan(&note, &comments, &status); err != nil {
		t.Fatal(err)
	}
	return note, comments, status
}

func redactFor(ctx context.Context, t *testing.T, tx pgx.Tx, subject ids.UUID) {
	t.Helper()
	if err := redactSubjectLegalReviewText(ctx, tx, subject, nil, "Sara Subject", "retention", causeRetention); err != nil {
		t.Fatalf("redacting the subject from legal review text: %v", err)
	}
}

func TestRedactingAStakeholderKeepsTheRestOfTheLegalReviewText(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, other := ids.NewV7(), ids.NewV7()
	for _, c := range []ids.UUID{subject, other} {
		mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Sara Subject', 'manual', 'test')`, c)
	}
	deal, theirs := seedReviewedDeal(ctx, t, tx, subject)
	mustExec(ctx, t, tx, `INSERT INTO relationship (kind, contact_id, deal_id, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'manual', 'test')`, other, deal)
	_, unrelated := seedReviewedDeal(ctx, t, tx, other)

	redactFor(ctx, t, tx, subject)

	note, comments, status := reviewText(ctx, t, tx, theirs)
	if note == nil || *note != "Please check the clause Erased asked for" {
		t.Errorf("note = %v, want the sentence kept with the name redacted", note)
	}
	if comments == nil || *comments != "Erased's change is acceptable" {
		t.Errorf("comments = %v, want the sentence kept with the name redacted", comments)
	}
	if status != "changes_needed" {
		t.Errorf("status = %q, want the outcome kept for the stage gate", status)
	}
	if note, _, _ := reviewText(ctx, t, tx, unrelated); note == nil || *note != reviewNote {
		t.Errorf("a review on a deal the subject has no part in changed: %v", note)
	}
	var tombstones int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'legal_review' AND entity_id = $1 AND action = 'erase'`,
		theirs).Scan(&tombstones); err != nil || tombstones != 1 {
		t.Errorf("tombstones for the changed review = %d, %v; want 1", tombstones, err)
	}
}

func TestRedactingAStakeholderReachesADealWhoseLinkWasArchived(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Sara Subject', 'manual', 'test')`, subject)
	deal, review := seedReviewedDeal(ctx, t, tx, subject)
	mustExec(ctx, t, tx, `UPDATE relationship SET archived_at = now() WHERE deal_id = $1`, deal)

	redactFor(ctx, t, tx, subject)

	if note, _, _ := reviewText(ctx, t, tx, review); note == nil || *note != "Please check the clause Erased asked for" {
		t.Errorf("note = %v, want the archived stakeholder redacted too", note)
	}
}

func TestTheExportCarriesTheLegalReviewTextOfTheSubjectsDeals(t *testing.T) {
	e := setupSARIdentifiers(t)
	_, review := seedReviewedDeal(e.ctx, t, e.owner, e.contact.UUID)

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.LegalReviews) != 1 {
		t.Fatalf("legal reviews in the export = %d, want the subject's one (review %s)", len(pkg.LegalReviews), review)
	}
	row := pkg.LegalReviews[0]
	if row["note"] != reviewNote || row["comments"] != reviewComments || row["status"] != "changes_needed" {
		t.Errorf("exported review = %v, want its status, note and comments", row)
	}
}

func TestTheExportLeavesOutTheLegalReviewOfADealWhoseStakeholderLinkWasArchived(t *testing.T) {
	e := setupSARIdentifiers(t)
	_, live := seedReviewedDeal(e.ctx, t, e.owner, e.contact.UUID)
	retiredDeal, retired := seedReviewedDeal(e.ctx, t, e.owner, e.contact.UUID)
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE relationship SET archived_at = now() WHERE deal_id = $1`, retiredDeal); err != nil {
		t.Fatal(err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	if len(pkg.LegalReviews) != 1 {
		t.Fatalf("legal reviews in the export = %d, want only the live link's (live %s, archived %s)",
			len(pkg.LegalReviews), live, retired)
	}
	if len(pkg.Deals) != 1 {
		t.Errorf("deals in the export = %d, want the same one deal", len(pkg.Deals))
	}
}

func TestASubjectWithNoUsablePatternHasTheLegalReviewTextCleared(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Cher', 'manual', 'test')`, subject)
	_, review := seedReviewedDeal(ctx, t, tx, subject)

	if err := redactSubjectLegalReviewText(ctx, tx, subject, nil, "Cher", "retention", causeRetention); err != nil {
		t.Fatal(err)
	}

	note, comments, status := reviewText(ctx, t, tx, review)
	if note != nil || comments != nil || status != "changes_needed" {
		t.Errorf("note=%v comments=%v status=%q, want the text cleared and the outcome kept", note, comments, status)
	}
	var tombstones int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'legal_review' AND entity_id = $1 AND action = 'erase'`,
		review).Scan(&tombstones); err != nil || tombstones != 1 {
		t.Errorf("tombstones = %d, %v; want 1", tombstones, err)
	}
}

func TestAnAddressInLegalReviewTextIsRedactedLikeAnUnlinkedNote(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject := ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Cher', 'manual', 'test')`, subject)
	_, review := seedReviewedDeal(ctx, t, tx, subject)
	mustExec(ctx, t, tx, `UPDATE legal_review SET note = 'Ask cher@example.test about clause nine' WHERE id = $1`, review)

	if err := redactSubjectLegalReviewText(ctx, tx, subject, []string{"cher@example.test"}, "Cher", "retention", causeRetention); err != nil {
		t.Fatal(err)
	}

	if note, _, _ := reviewText(ctx, t, tx, review); note == nil || *note != "Ask Erased about clause nine" {
		t.Errorf("note = %v, want the address redacted and the rest kept", note)
	}
}
