// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRequestCoverageKeepsInstantAndDateOnlyEvidenceApart(t *testing.T) {
	for _, precision := range []crmcontracts.OccurredPrecision{
		crmcontracts.OccurredPrecisionOccurredPrecisionDay,
		crmcontracts.OccurredPrecisionOccurredPrecisionInstant,
	} {
		t.Run(string(precision), func(t *testing.T) {
			source := act(crmcontracts.ActivityKindEmail, testNow.AddDate(0, 0, -10))
			source.OccurredPrecision = precision
			f := facts{now: testNow, currentRequests: []crmcontracts.Activity{source}}
			s := &Service{}
			s.gatherMailCoverage(context.Background(), &f)
			if f.mailCoverage == nil || f.mailCoverage.State != crmcontracts.MailCaptureCoverageStateUnknown {
				t.Fatalf("unbound coverage = %+v", f.mailCoverage)
			}
			from := f.mailCoverage.IntervalFrom
			if precision == crmcontracts.OccurredPrecisionOccurredPrecisionDay {
				if from != nil {
					t.Fatal("date-only source invented a coverage instant")
				}
			} else if from == nil || !from.Equal(source.OccurredAt) {
				t.Fatalf("instant source lower bound = %v", from)
			}
		})
	}
}

func TestCoverageChangesPreserveQualifiedWordsAndHumanDismissals(t *testing.T) {
	coverage := crmcontracts.MailCaptureCoverage{
		Channel:    crmcontracts.MailCaptureCoverageChannelEmail,
		State:      crmcontracts.MailCaptureCoverageStatePartial,
		IntervalTo: testNow, EvaluatedAt: testNow,
	}
	f := facts{deal: openDeal(), now: testNow, loc: time.UTC, mailCoverage: &coverage}
	move := decideMove(f)
	dismissal := moveFingerprint(move)
	key := func() string {
		t.Helper()
		got, err := Fingerprint(project(f, decideMove(f)), ids.Nil, "fixture", testNow, time.UTC, "en")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := key()
	coverage.EvaluatedAt = testNow.Add(time.Minute)
	coverage.IntervalTo = coverage.EvaluatedAt
	if got := key(); got != before {
		t.Fatal("a new observation alone invalidated unchanged words")
	}
	coverage.State = crmcontracts.MailCaptureCoverageStateUnavailable
	if got := key(); got != before {
		t.Fatal("capture state invalidated words that describe recorded evidence")
	}
	if got := moveFingerprint(decideMove(f)); got != dismissal {
		t.Fatal("capture state reopened a human dismissal of the same move")
	}
}

func TestCachedMovesKeepWorkAndReplaceEveryReadersCoverage(t *testing.T) {
	prior := crmcontracts.MailCaptureCoverage{State: crmcontracts.MailCaptureCoverageStatePartial, LastSuccessAt: &testNow}
	withheld := crmcontracts.MailCaptureCoverage{
		State:       crmcontracts.MailCaptureCoverageStateWithheld,
		Channel:     crmcontracts.MailCaptureCoverageChannelEmail,
		IntervalTo:  testNow,
		EvaluatedAt: testNow,
	}
	oldID, legacyID := ids.NewV7(), ids.NewV7()
	moves := map[ids.UUID]crmcontracts.DealStatusCardMove{
		oldID:    {MailCoverage: &prior, Basis: &crmcontracts.WorkEvidenceBasis{MailCoverage: &prior}},
		legacyID: {},
	}
	s := NewService(nil, nil, nil, nil, func() time.Time { return testNow })
	s.WithMailCoverage(func(context.Context, *time.Time, time.Time) (crmcontracts.MailCaptureCoverage, error) {
		return withheld, nil
	})
	s.refreshMoveCoverage(context.Background(), moves)
	if len(moves) != 2 {
		t.Fatalf("capture context removed recorded work: %v", moves)
	}
	for _, move := range moves {
		if move.MailCoverage == nil || move.MailCoverage.LastSuccessAt != nil || move.MailCoverage.State != withheld.State {
			t.Fatalf("human capture context escaped refresh: %+v", move)
		}
	}
	if basis := moves[oldID].Basis; basis == nil || basis.MailCoverage == nil || basis.MailCoverage.LastSuccessAt != nil || basis.MailCoverage.State != withheld.State {
		t.Fatalf("nested human capture context escaped refresh: %+v", basis)
	}
}

func TestAnOmittedClockStillAllowsCacheOnlyReads(t *testing.T) {
	s := NewService(nil, nil, nil, nil, nil)
	if s.now == nil {
		t.Fatal("a cache-only reader has no observation clock")
	}
}
