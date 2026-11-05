// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSignalFindingsFollowTheSuppliedEligibilityDecision(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	for _, admitted := range []bool{false, true} {
		scanner := (&Scanner{}).WithSignalSubjects(func(context.Context, pgx.Tx, time.Time) ([]SignalSubject, error) {
			if !admitted {
				return nil, nil
			}
			return []SignalSubject{{SignalID: "signal", CompanyID: "company", Owner: "owner", SuggestionID: "suggestion", OccurredAt: now, BuyerVerified: true}}, nil
		})
		pass := &walked{}
		count, err := scanner.askSignals(context.Background(), nil, now, pass)
		if err != nil {
			t.Fatal(err)
		}
		if !admitted {
			if count != 0 || len(pass.raised) != 0 {
				t.Fatalf("ineligible subject raised %+v", pass)
			}
			continue
		}
		if count != 1 || len(pass.raised) != 1 || len(pass.seen) != 1 {
			t.Fatalf("eligible census = %d/%+v", count, pass)
		}
		finding := pass.raised[0].finding
		if finding.Type != TypeUnlinkedSignal || finding.SubjectKind != "signal" || finding.SubjectID != "signal" || finding.Observed["suggestion_id"] != "suggestion" || pass.raised[0].owner != "owner" {
			t.Fatalf("finding = %+v", finding)
		}
	}
}
