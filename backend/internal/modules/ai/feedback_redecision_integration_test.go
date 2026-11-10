// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func feedbackSubject(t *testing.T) (context.Context, *database.DB, ids.UUID) {
	t.Helper()
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	subject := ids.NewV7()
	if _, err := env.owner.Exec(ctx, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES (@id, 'Demo contact', 'manual', 'system:test')`, pgx.StrictNamedArgs{"id": subject}); err != nil {
		t.Fatal(err)
	}
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:feedback-test",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	return ctx, env.dbFor(ws), subject
}

func readFeedback(ctx context.Context, t *testing.T, db *database.DB, subject ids.UUID) Verdict {
	t.Helper()
	var verdicts map[string]Verdict
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		verdicts, err = NewFeedbackStore(db).VerdictsForTx(ctx, tx, "contact", subject)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v, ok := verdicts[VerdictLookupKey(ClaimProfileField, ClaimKey(ProfileFieldClaimPath("title")))]
	if !ok {
		t.Fatal("recorded feedback is missing")
	}
	return v
}

func TestRepeatedCorrectionsRemainAboutTheOriginalSource(t *testing.T) {
	ctx, db, subject := feedbackSubject(t)
	store := NewFeedbackStore(db)
	stamp := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	original, shown := "Sales manager", "Sales manager"
	for _, corrected := range []string{"Head of sales", "Sales director", "VP sales"} {
		if err := store.Record(ctx, RecordInput{
			SubjectType: "contact", SubjectID: subject, ClaimKind: ClaimProfileField,
			ClaimPath: ProfileFieldClaimPath("title"), Verdict: VerdictCorrected,
			ValueShown: &shown, ValueCapturedAt: &stamp, CorrectedValue: &corrected,
		}); err != nil {
			t.Fatal(err)
		}
		v := readFeedback(ctx, t, db, subject)
		decision, applies := v.AsOf(original, stamp)
		if !applies || decision.CorrectedValue == nil || *decision.CorrectedValue != corrected {
			t.Fatalf("correction of %q: decision=%+v, applies=%v", shown, decision, applies)
		}
		shown = corrected
	}
	if err := store.Record(ctx, RecordInput{
		SubjectType: "contact", SubjectID: subject, ClaimKind: ClaimProfileField,
		ClaimPath: ProfileFieldClaimPath("title"), Verdict: VerdictSuppressed,
		ValueShown: &shown, ValueCapturedAt: &stamp,
	}); err != nil {
		t.Fatal(err)
	}
	decision, applies := readFeedback(ctx, t, db, subject).AsOf(original, stamp)
	if !applies || decision.Verdict != VerdictSuppressed || decision.CorrectedValue != nil {
		t.Fatalf("suppression of corrected field: decision=%+v, applies=%v", decision, applies)
	}
}

func TestCorrectionOverlayRequiresTheSameSourceVersion(t *testing.T) {
	stamp := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	newer, older := stamp.Add(time.Hour), stamp.Add(-time.Hour)
	for _, tc := range []struct {
		name       string
		priorStamp *time.Time
		nextStamp  *time.Time
		shown      string
		conflict   bool
	}{
		{name: "new source equals previous correction", priorStamp: &stamp, nextStamp: &newer, shown: "Head of sales"},
		{name: "same stamp but stale overlay", priorStamp: &stamp, nextStamp: &stamp, shown: "CEO", conflict: true},
		{name: "prior source unknown", nextStamp: &stamp, shown: "Head of sales"},
		{name: "incoming source unknown", priorStamp: &stamp, shown: "Head of sales"},
		{name: "neither source known", shown: "Head of sales"},
		{name: "stale source refused", priorStamp: &stamp, nextStamp: &older, shown: "Head of sales", conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, db, subject := feedbackSubject(t)
			store := NewFeedbackStore(db)
			original, first, second := "Sales manager", "Head of sales", "VP sales"
			input := RecordInput{
				SubjectType: "contact", SubjectID: subject, ClaimKind: ClaimProfileField,
				ClaimPath: ProfileFieldClaimPath("title"), Verdict: VerdictCorrected,
				ValueShown: &original, ValueCapturedAt: tc.priorStamp, CorrectedValue: &first,
			}
			if err := store.Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			input.ValueShown, input.ValueCapturedAt, input.CorrectedValue = &tc.shown, tc.nextStamp, &second
			err := store.Record(ctx, input)
			if tc.conflict {
				if !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("stale correction: %v, want conflict", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			wantShown, wantCorrection := tc.shown, second
			if tc.conflict {
				wantShown, wantCorrection = original, first
			}
			v := readFeedback(ctx, t, db, subject)
			decision, applies := v.AsOf(wantShown, stamp)
			if !applies || decision.CorrectedValue == nil || *decision.CorrectedValue != wantCorrection {
				t.Fatalf("decision=%+v, applies=%v, want %q on %q", decision, applies, wantCorrection, wantShown)
			}
			if !tc.conflict {
				if _, applies := v.AsOf(original, stamp); applies {
					t.Fatal("correction was incorrectly attached to an older source value")
				}
			}
		})
	}
}

func TestLegacyCorrectionKeepsItsOrderingWhenEdited(t *testing.T) {
	for _, newerSource := range []bool{false, true} {
		name := "overlay on the old source"
		if newerSource {
			name = "new source equals the old correction"
		}
		t.Run(name, func(t *testing.T) {
			ctx, db, subject := feedbackSubject(t)
			store := NewFeedbackStore(db)
			first, second := "Head of sales", "VP sales"
			input := RecordInput{
				SubjectType: "contact", SubjectID: subject, ClaimKind: ClaimProfileField,
				ClaimPath: ProfileFieldClaimPath("title"), Verdict: VerdictCorrected, CorrectedValue: &first,
			}
			if err := store.Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			stamp := readFeedback(ctx, t, db, subject).recordedAt
			if newerSource {
				stamp = stamp.Add(time.Hour)
			}
			input.ValueShown, input.ValueCapturedAt, input.CorrectedValue = &first, &stamp, &second
			if !newerSource {
				confirmation := input
				confirmation.Verdict, confirmation.CorrectedValue = VerdictConfirmed, nil
				if err := store.Record(ctx, confirmation); !errors.Is(err, apperrors.ErrConflict) {
					t.Fatalf("confirmation of a legacy overlay: %v, want conflict", err)
				}
			}
			if err := store.Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			v := readFeedback(ctx, t, db, subject)
			source := "Sales manager"
			if newerSource {
				source = first
			}
			decision, applies := v.AsOf(source, stamp)
			if !applies || decision.CorrectedValue == nil || *decision.CorrectedValue != second {
				t.Fatalf("legacy correction: decision=%+v, applies=%v", decision, applies)
			}
			if !newerSource {
				if _, applies := v.AsOf("New source", v.recordedAt.Add(time.Hour)); applies {
					t.Fatal("legacy correction applied to a source newer than the decision")
				}
			}
		})
	}
}

func TestAStaleOverlayAndConfirmationCannotReplaceTheCurrentCorrection(t *testing.T) {
	for _, verdict := range []string{VerdictCorrected, VerdictConfirmed} {
		t.Run(verdict, func(t *testing.T) {
			ctx, db, subject := feedbackSubject(t)
			store := NewFeedbackStore(db)
			stamp := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
			original, first, second, stale := "Sales manager", "Head of sales", "VP sales", "Sales director"
			input := RecordInput{
				SubjectType: "contact", SubjectID: subject, ClaimKind: ClaimProfileField,
				ClaimPath: ProfileFieldClaimPath("title"), Verdict: VerdictCorrected,
				ValueShown: &original, ValueCapturedAt: &stamp, CorrectedValue: &first,
			}
			if err := store.Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			input.ValueShown, input.CorrectedValue = &first, &second
			if err := store.Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			input.Verdict, input.CorrectedValue = verdict, &stale
			if verdict == VerdictConfirmed {
				input.ValueShown, input.CorrectedValue = &second, nil
			}
			if err := store.Record(ctx, input); !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("%s over an overlay: %v, want conflict", verdict, err)
			}
			decision, applies := readFeedback(ctx, t, db, subject).AsOf(original, stamp)
			if !applies || decision.Verdict != VerdictCorrected || decision.CorrectedValue == nil || *decision.CorrectedValue != second {
				t.Fatalf("latest correction lost: decision=%+v, applies=%v", decision, applies)
			}
		})
	}
}
