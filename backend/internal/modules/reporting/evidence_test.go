// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

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

// The largest named rows skip what the reader may not see whole before they
// rank. So withheld deals never hide an eligible one. Equal values order by
// record id.
func TestTheLargestNamedEvidenceRanksOnlyRowsTheReaderSeesWhole(t *testing.T) {
	ref := crmcontracts.ReportingEvidenceRef{Metric: "open_pipeline", ContextId: "state"}
	deal := func(value float64, owned bool) Fact {
		id, owner := ids.NewV7(), ids.NewV7()
		fact := Fact{Metric: ref.Metric, ContextID: ref.ContextId, OwnerID: owner, Row: crmcontracts.ReportingEvidenceRow{Key: id.String(), SourceId: new(openapi_types.UUID(id)), Value: &value}}
		if owned {
			fact.Row.OwnerId = new(openapi_types.UUID(owner))
		}
		return fact
	}
	facts := []Fact{}
	for _, value := range []float64{30, 110, 70, 10, 90, 50, 120, 20, 100, 60, 40, 80, 80} {
		facts = append(facts, deal(value, true))
	}
	restricted, withheld, unowned := deal(500, true), deal(400, true), deal(300, false)
	restricted.Row.Restricted = true
	withheld.OwnerWithheld = true
	facts = append([]Fact{restricted, withheld, unowned}, facts...)
	facts = append(facts, Fact{Metric: ref.Metric, ContextID: "movement_delta", Row: deal(900, true).Row})

	rows := largestNamedEvidence(crmcontracts.ReportingContext{}, facts, ref, 8)
	got := []float64{}
	for _, row := range rows {
		got = append(got, *row.Value)
	}
	if want := []float64{120, 110, 100, 90, 80, 80, 70, 60}; !slices.Equal(got, want) {
		t.Fatalf("the largest named rows are %v, want %v", got, want)
	}
	if rows[4].SourceId.String() > rows[5].SourceId.String() {
		t.Fatalf("equal values are not in record id order: %v, %v", rows[4].SourceId, rows[5].SourceId)
	}
}
