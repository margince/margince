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

func TestAnonymizingAStakeholderClearsTheLegalReviewTextOfTheirDeals(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, bystander := ids.NewV7(), ids.NewV7()
	for _, c := range []ids.UUID{subject, bystander} {
		mustExec(ctx, t, tx, `INSERT INTO contact (id, full_name, source, captured_by)
			VALUES ($1, 'Review Subject', 'manual', 'test')`, c)
	}
	_, theirs := seedReviewedDeal(ctx, t, tx, subject)
	_, others := seedReviewedDeal(ctx, t, tx, bystander)

	if err := purgeAnonymizedContactJudgments(ctx, tx, subject, nil); err != nil {
		t.Fatalf("purging the anonymized contact's judgments: %v", err)
	}

	note, comments, status := reviewText(ctx, t, tx, theirs)
	if note != nil || comments != nil {
		t.Errorf("the subject's review keeps note=%v comments=%v, want both cleared", note, comments)
	}
	if status != "changes_needed" {
		t.Errorf("the subject's review status = %q, want the outcome kept for the stage gate", status)
	}
	if note, comments, _ := reviewText(ctx, t, tx, others); note == nil || comments == nil {
		t.Errorf("another contact's review lost its text: note=%v comments=%v", note, comments)
	}
	var tombstones int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'legal_review' AND entity_id = $1 AND action = 'erase'`,
		theirs).Scan(&tombstones); err != nil || tombstones != 1 {
		t.Errorf("tombstones for the cleared review = %d, %v; want 1", tombstones, err)
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
