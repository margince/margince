// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAReceiptSaysHowThePassEnded(t *testing.T) {
	at := time.Date(2026, 9, 1, 3, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		tally   sweepTally
		err     error
		outcome capture.SweepOutcome
		class   string
	}{
		{"a clean pass", sweepTally{processed: 4}, nil, capture.SweepOK, ""},
		{"a pass stopped at its bound", sweepTally{processed: 200, capHit: true}, nil, capture.SweepPartial, ""},
		{
			"a known failure",
			sweepTally{processed: 2},
			fmt.Errorf("lifting: %w", apperrors.ErrConflict),
			capture.SweepFailed, "write_conflict",
		},
		{
			"a failure nobody classified",
			sweepTally{capHit: true},
			errors.New("pq: <a@b.test> refused"),
			capture.SweepFailed, unclassifiedSweepFailure,
		},
	}
	for _, c := range cases {
		got := sweepReceiptFor(capture.SweepFiledMeetingHolds, at, at.Add(time.Second), c.tally, c.err)
		if got.Outcome != c.outcome || got.ErrorClass != c.class || got.Processed != c.tally.processed {
			t.Errorf("%s: receipt = %+v, want outcome %s class %q processed %d",
				c.name, got, c.outcome, c.class, c.tally.processed)
		}
	}
}

// The column admits any token; the page shows only one an operator can look up.
func TestOnlyAVocabularyClassReachesThePage(t *testing.T) {
	for stored, want := range map[string]string{
		"":                       "",
		"write_conflict":         "write_conflict",
		unclassifiedSweepFailure: unclassifiedSweepFailure,
		panickedSweepFailure:     panickedSweepFailure,
		"someone_example_test":   unclassifiedSweepFailure,
	} {
		if got := vettedSweepClass(stored); got != want {
			t.Errorf("vettedSweepClass(%q) = %q, want %q", stored, got, want)
		}
	}
}
