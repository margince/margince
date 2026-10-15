// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// These are the pure classification rules Undo's loop relies on to decide
// whether a Reverse failure belongs to one row (errored, recorded, moved
// past) or means the estate itself could not be reached (fatal to the
// current pass) — proven here without a database, since neither function
// touches one.
func TestIsRowRefusalClassifiesTheThreeDomainSentinelsOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"permission denied", apperrors.ErrPermissionDenied, true},
		{"not found", apperrors.ErrNotFound, true},
		{"conflict", apperrors.ErrConflict, true},
		{"wrapped permission denied", errors.New("wrap: " + apperrors.ErrPermissionDenied.Error()), false}, // not errors.Is-compatible without %w
		{"unclassified", errors.New("connection reset by peer"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRowRefusal(tc.err); got != tc.want {
				t.Errorf("isRowRefusal(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestReversalRefusalReasonNamesEachSentinelAndFallsBackHonestly(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code ReasonCode
		want string
	}{
		{"permission denied", apperrors.ErrPermissionDenied, ReasonUndoGrantLost, "Your permissions no longer cover this record."},
		{"not found", apperrors.ErrNotFound, ReasonUndoNotVisible, "This record is no longer visible to you."},
		{"conflict", apperrors.ErrConflict, ReasonUndoRuleProtects, "The record refused the undo because a business rule protects it."},
		{"unclassified", errors.New("boom"), ReasonUndoFailed, "The record could not be undone."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := reversalRefusalReason(tc.err)
			if got.Code != tc.code || got.English() != tc.want {
				t.Errorf("reversalRefusalReason(%v) = %s %q, want %s %q", tc.err, got.Code, got.English(), tc.code, tc.want)
			}
		})
	}
}

// reverseOneRow itself, against a bare fake — no import_record_map or
// Postgres needed to prove the kept/reversed/propagate-on-unreachable
// split, which is the one behavior the whole redesign turns on.
type stubUndoWriters struct {
	err error
}

func (w stubUndoWriters) Reverse(_ context.Context, _ string, _ ids.UUID, _ time.Time) error {
	return w.err
}

func TestReverseOneRowSplitsKeptReversedAndUnreachable(t *testing.T) {
	row := mapRow{object: ObjectLead, nativeID: ids.NewV7()}

	t.Run("touched is kept, never calls Reverse", func(t *testing.T) {
		var rep UndoReport
		w := stubUndoWriters{err: errors.New("must not be called")}
		if err := reverseOneRow(context.Background(), w, row, true, &rep); err != nil {
			t.Fatalf("reverseOneRow: %v", err)
		}
		if len(rep.Kept) != 1 || rep.ReversedCount != 0 {
			t.Fatalf("rep = %+v, want the row kept and Reverse never invoked", rep)
		}
	})

	t.Run("untouched and reversible increments ReversedCount", func(t *testing.T) {
		var rep UndoReport
		w := stubUndoWriters{err: nil}
		if err := reverseOneRow(context.Background(), w, row, false, &rep); err != nil {
			t.Fatalf("reverseOneRow: %v", err)
		}
		if rep.ReversedCount != 1 || len(rep.Kept) != 0 || len(rep.Errored) != 0 {
			t.Fatalf("rep = %+v, want exactly one reversal", rep)
		}
	})

	t.Run("a row refusal is recorded as errored, not propagated", func(t *testing.T) {
		var rep UndoReport
		w := stubUndoWriters{err: apperrors.ErrConflict}
		if err := reverseOneRow(context.Background(), w, row, false, &rep); err != nil {
			t.Fatalf("reverseOneRow returned %v, want nil (a row refusal is recorded, not propagated)", err)
		}
		if len(rep.Errored) != 1 || rep.Errored[0].Reason == "" {
			t.Fatalf("rep = %+v, want the row named as errored", rep)
		}
	})

	t.Run("an unclassified failure propagates rather than being recorded", func(t *testing.T) {
		var rep UndoReport
		cause := errors.New("connection reset")
		w := stubUndoWriters{err: cause}
		if err := reverseOneRow(context.Background(), w, row, false, &rep); !errors.Is(err, cause) {
			t.Fatalf("reverseOneRow err = %v, want the unclassified cause propagated", err)
		}
		if len(rep.Errored) != 0 || rep.ReversedCount != 0 {
			t.Fatalf("rep = %+v, want nothing recorded for an unreachable estate", rep)
		}
	})
}

func TestAnUndoReversesTheTypesALaterImportLandedFirst(t *testing.T) {
	want := []string{ObjectActivity, ObjectDeal, ObjectLead, ObjectContact, ObjectCompany}
	if got := reverseImportOrder(); !slices.Equal(got, want) {
		t.Fatalf("undo order = %v, want %v: a contact archived first leaves its deals and activities naming an archived record", got, want)
	}
	if ImportOrder[0] != ObjectCompany {
		t.Fatalf("reversing changed ImportOrder itself: %v", ImportOrder)
	}
}
