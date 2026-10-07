// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEvidenceMatchesTheChosenOwnerAndExclusiveCumulativeCutoff(t *testing.T) {
	owner, other := ids.NewV7(), ids.NewV7()
	cutoff := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	before := cutoff.Add(-time.Hour)
	facts := []Fact{{Metric: "bookings_won", ContextID: "interval", OwnerID: owner, Row: crmcontracts.ReportingEvidenceRow{Key: "included", OccurredAt: &before}}, {Metric: "bookings_won", ContextID: "interval", OwnerID: owner, Row: crmcontracts.ReportingEvidenceRow{Key: "on-boundary", OccurredAt: &cutoff}}, {Metric: "bookings_won", ContextID: "interval", OwnerID: other, Row: crmcontracts.ReportingEvidenceRow{Key: "other-owner", OccurredAt: &before}}}
	out, err := evidencePage(crmcontracts.ReportingContext{}, facts, "bookings_won", "interval", "owner:"+owner.String(), &cutoff, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Key != "included" {
		t.Fatalf("wrong mark evidence: %+v", out.Rows)
	}
}

func TestEvidenceContinuationDoesNotChangeTheUnderlyingCohort(t *testing.T) {
	facts := []Fact{{Metric: "meetings_held", ContextID: "target", Row: crmcontracts.ReportingEvidenceRow{Key: "first"}}, {Metric: "meetings_held", ContextID: "target", Row: crmcontracts.ReportingEvidenceRow{Key: "second"}}}
	first, err := evidencePage(crmcontracts.ReportingContext{}, facts, "meetings_held", "target", "", nil, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextCursor == nil || first.Rows[0].Key != "first" {
		t.Fatalf("first page = %+v", first)
	}
	second, err := evidencePage(crmcontracts.ReportingContext{}, facts, "meetings_held", "target", "", nil, first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Truncated || second.NextCursor != nil || len(second.Rows) != 1 || second.Rows[0].Key != "second" {
		t.Fatalf("second page = %+v", second)
	}
}
