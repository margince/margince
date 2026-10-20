// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/feedback"
)

func TestMergePreviewMatchesHumanDecisionSurvivorship(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			seed := e.SeedContact
			if kind == "company" {
				seed = e.SeedCompany
			}
			source, target := seed(t, "Original record", &e.AdminUser), seed(t, "Surviving record", &e.AdminUser)
			store := ai.NewFeedbackStore(e.DB())
			for _, input := range []ai.RecordInput{
				{SubjectID: source, ClaimPath: "first", Verdict: ai.VerdictSuppressed},
				{SubjectID: source, ClaimPath: "second", Verdict: ai.VerdictConfirmed},
				{SubjectID: target, ClaimPath: "first", Verdict: ai.VerdictCorrected, CorrectedValue: new("Human correction")},
				{SubjectID: target, ClaimPath: "unrelated", Verdict: ai.VerdictConfirmed},
			} {
				input.SubjectType, input.ClaimKind = kind, ai.ClaimSignal
				input.ValueShown, input.Note = new("Original assertion"), new("Decision context")
				if err := store.Record(ctx, input); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Feedback) != 2 {
				t.Fatalf("affected decisions: %+v", preview.Feedback)
			}
			expected := map[ids.UUID]feedback.StoredDecision{}
			for _, effect := range preview.Feedback {
				if effect.Source.Version < 1 || effect.Source.UpdatedAt.IsZero() || effect.Source.CapturedBy == "" {
					t.Fatalf("missing revision or author: %+v", effect)
				}
				kept := effect.Source
				if effect.Source.ClaimKey == ai.ClaimKey("first") {
					if effect.Target == nil || effect.Target.Verdict != ai.VerdictCorrected || effect.Target.CorrectedValue == nil || *effect.Target.CorrectedValue != "Human correction" {
						t.Fatalf("wrong collision: %+v", effect)
					}
					kept = *effect.Target
				} else if effect.Target != nil {
					t.Fatalf("unexpected collision: %+v", effect)
				}
				expected[kept.ID] = kept
			}
			checkMergePreviewRefusals(t, e, kind, source, target, preview, "description")
			e.Contacts.WithFeedbackCarryReader(nil)
			incomplete, refusal := e.Contacts.PreviewMergeSurvivorship(ctx, kind, source, target, nil, nil)
			if !errors.Is(refusal, apperrors.ErrInvalidArgument) || incomplete.Feedback != nil || incomplete.Fields != nil {
				t.Fatalf("unwired reader exposed a partial preview: %+v, %v", incomplete, refusal)
			}
			e.Contacts.WithFeedbackCarryReader(store)
			if kind == "contact" {
				_, err = e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](target), &preview.SourceVersion)
			} else {
				_, err = e.Contacts.MergeCompany(ctx, ids.From[ids.CompanyKind](source), ids.From[ids.CompanyKind](target), &preview.SourceVersion)
			}
			if err != nil {
				t.Fatal(err)
			}
			checkCarriedFeedback(t, e, ctx, kind, source, target, expected)
		})
	}
}

func checkCarriedFeedback(t *testing.T, e *integration.Env, ctx context.Context, kind string, source, target ids.UUID, expected map[ids.UUID]feedback.StoredDecision) {
	t.Helper()
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var retired, kept int
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE subject_id = @source), count(*) FILTER (WHERE subject_id = @target)
 FROM ai_feedback WHERE subject_type = @kind AND subject_id IN (@source, @target)`, pgx.StrictNamedArgs{"kind": kind, "source": source, "target": target}).Scan(&retired, &kept); err != nil {
			return err
		}
		if retired != 0 || kept != 3 {
			t.Errorf("retired=%d surviving=%d, want 0 and 3", retired, kept)
		}
		for id, expected := range expected {
			var verdict, note, shown string
			var corrected *string
			if err := tx.QueryRow(ctx, `SELECT verdict, note, value_shown, corrected_value FROM ai_feedback WHERE id=@id AND subject_id=@target`, pgx.StrictNamedArgs{"id": id, "target": target}).Scan(&verdict, &note, &shown, &corrected); err != nil {
				return err
			}
			if verdict != expected.Verdict || note != *expected.Note || shown != *expected.ValueShown {
				t.Errorf("decision changed: %s %s %s", verdict, note, shown)
			}
			if expected.CorrectedValue != nil && (corrected == nil || *corrected != *expected.CorrectedValue) {
				t.Errorf("correction changed: %v", corrected)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
