// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The receipt lane makes a claim about WHO acted, and the claim is the whole
// point of the lane: "done for you" says the system handled something so the
// reader did not have to. Telling somebody that about a decision they made
// themselves is not a cosmetic error — it misreports their own work back to
// them, and it inflates what the product looks like it is doing.
//
// WHICH rows are the system's is now answered in SQL, by the decision's own
// decided_by_system marker, so what is left here is the lane's own horizon: a
// receipt older than the window belongs to a morning the reader has already
// seen. The window is the reason this function still exists.
func TestAReceiptOlderThanTheWindowIsNotTodaysNews(t *testing.T) {
	since := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC)
	thisMorning := approvalRow("Moved the Acme close date to 27 Sep", since.Add(time.Hour))
	lastWeek := approvalRow("Filed a message under Riverty", since.AddDate(0, 0, -7))

	receipts := receiptsWithin([]crmcontracts.Approval{thisMorning, lastWeek}, since)

	if len(receipts) != 1 {
		t.Fatalf("the lane carries %d receipts, want only the one inside the window", len(receipts))
	}
	if receipts[0].Summary != "Moved the Acme close date to 27 Sep" {
		t.Errorf("the lane kept %q, want this morning's act", receipts[0].Summary)
	}
}

// A row the store answered with no decision time cannot be placed against the
// window, and a receipt the reader cannot date is not a receipt.
func TestAnUndatedDecisionIsNotAReceipt(t *testing.T) {
	since := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC)
	undated := crmcontracts.Approval{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "close_date_correction",
	}

	if receipts := receiptsWithin([]crmcontracts.Approval{undated}, since); len(receipts) != 0 {
		t.Fatalf("the lane carries %d receipts, want none: nothing dates this row", len(receipts))
	}
}

func approvalRow(summary string, decidedAt time.Time) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:        openapi_types.UUID(ids.NewV7()),
		Kind:      "close_date_correction",
		Summary:   &summary,
		DecidedAt: &decidedAt,
	}
}

// The receipts read is bounded by the lane it fills.
//
// It used to read far wider and narrow afterwards, because "nobody was asked"
// could only be tested in Go. That put the bound on the wrong set: a page of the
// reader's own decisions filtered to nothing while real receipts sat behind it.
// The store now answers the question itself, so asking for the lane's width is
// the honest read — and this pins that the seam does not quietly go back to
// over-reading.
func TestTheReceiptsReadAsksForTheLaneItFills(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	page := make([]crmcontracts.Approval, 0, doneLaneWidth)
	for i := 0; i < doneLaneWidth; i++ {
		page = append(page, approvalRow("Filed a message under Riverty", decidedAt))
	}

	engine := &stubApprovalPage{rows: page}
	receipts, err := recentReceipts(decidedAt.Add(-time.Hour), doneLaneWidth, engine.list)
	if err != nil {
		t.Fatalf("reading the receipts: %v", err)
	}
	if engine.asked != doneLaneWidth {
		t.Errorf("the seam asked for %d rows to fill a lane of %d", engine.asked, doneLaneWidth)
	}
	if len(receipts) != doneLaneWidth {
		t.Fatalf("the lane carries %d receipts, want the %d the store answered", len(receipts), doneLaneWidth)
	}
}

// doneLaneWidth is what the day's surface shows in its receipt lane.
const doneLaneWidth = 8

// stubApprovalPage answers exactly the number of rows it is asked for, which is
// what lets a test see the width the seam requested.
type stubApprovalPage struct {
	rows  []crmcontracts.Approval
	asked int
}

func (s *stubApprovalPage) list(limit int) ([]crmcontracts.Approval, error) {
	s.asked = limit
	if limit > len(s.rows) {
		limit = len(s.rows)
	}
	return s.rows[:limit], nil
}

// The decay lane's CANDIDATES come from the projection's own threshold, and its
// VERDICT comes from the §4 derivation. Keeping the two apart is the whole
// design: the projection knows when a pair last spoke and nothing about why,
// so a candidate whose derivation says something else — they replied, the band
// moved — is not a lapsed relationship however old its last row is.
//
// Reported otherwise, the lane would contradict the contact's own page about
// the same relationship, which is the one thing two surfaces reading one engine
// must never do.
func TestOnlyADerivedSilenceReachesTheDecayLane(t *testing.T) {
	oldest := ids.NewV7()
	newer := ids.NewV7()
	returned := ids.NewV7()
	oldestSpoke := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	newerSpoke := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	// The derivation answers in ITS order — here deliberately not the
	// projection's — because the lane, not the derivation, owes the rep the
	// oldest silence on top.
	quiet := quietRelationships(
		[]search.InteractionEdge{
			{PersonID: oldest, LastAt: oldestSpoke},
			{PersonID: newer, LastAt: newerSpoke},
			{PersonID: returned, LastAt: newerSpoke},
		},
		[]people.PersonChanges{
			{
				PersonID:    ids.From[ids.PersonKind](returned),
				DisplayName: "Tomas Berg",
				Changes:     []relstrength.Change{{Kind: relstrength.ChangeRepliedAfterGap, Days: 41}},
			},
			{
				PersonID:    ids.From[ids.PersonKind](newer),
				DisplayName: "Ines Sommer",
				Changes:     []relstrength.Change{{Kind: relstrength.ChangeWentQuiet, Days: 41}},
			},
			{
				PersonID:    ids.From[ids.PersonKind](oldest),
				DisplayName: "Dana Weiss",
				Changes:     []relstrength.Change{{Kind: relstrength.ChangeWentQuiet, Days: 63}},
			},
		},
	)

	if len(quiet) != 2 {
		t.Fatalf("the lane carries %d relationships, want the two derived silences", len(quiet))
	}
	if quiet[0].Name != "Dana Weiss" || quiet[1].Name != "Ines Sommer" {
		t.Errorf("the lane reads %q then %q, want the oldest silence first", quiet[0].Name, quiet[1].Name)
	}
	// The DERIVATION's span, not the projection's: the card says this number out
	// loud, and the contact's own page says the same one.
	if quiet[0].QuietDays != 63 {
		t.Errorf("the card says %d days, want the derived 63", quiet[0].QuietDays)
	}
	if !quiet[0].LastAt.Equal(oldestSpoke) {
		t.Errorf("the card dates the silence at %s, want the last exchange %s", quiet[0].LastAt, oldestSpoke)
	}
}

// A contact the caller may not read never reaches the derivation, so the lane
// simply has nothing to say about them — no row, and no empty-handed entry that
// would disclose the pair exists.
func TestTheDecayLaneReportsNothingItCannotDerive(t *testing.T) {
	edge := search.InteractionEdge{PersonID: ids.NewV7(), LastAt: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)}
	if quiet := quietRelationships([]search.InteractionEdge{edge}, nil); len(quiet) != 0 {
		t.Errorf("the lane invented %d relationships from an empty derivation", len(quiet))
	}
}
