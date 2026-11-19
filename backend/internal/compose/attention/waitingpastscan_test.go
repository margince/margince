// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealsWaitingStore answers DealsWaiting from a fixed set of waited-on deals
// and records which deals each read asked about.
type dealsWaitingStore struct {
	waitedOn map[ids.UUID]bool
	asked    [][]ids.UUID
}

func (d *dealsWaitingStore) WaitingAfter(context.Context, time.Time, WaitingFor, string, int) (WaitingTail, error) {
	return WaitingTail{}, nil
}

func (d *dealsWaitingStore) DealsWaiting(_ context.Context, _ time.Time, _ WaitingFor, deals []ids.UUID) (map[ids.UUID]bool, error) {
	d.asked = append(d.asked, slices.Clone(deals))
	named := map[ids.UUID]bool{}
	for _, deal := range deals {
		if d.waitedOn[deal] {
			named[deal] = true
		}
	}
	return named, nil
}

func (d *dealsWaitingStore) CardsAmong(context.Context, time.Time, WaitingFor, []ids.UUID) (map[ids.UUID]bool, error) {
	return map[ids.UUID]bool{}, nil
}

// atRiskDay is a day whose at-risk lane holds these deals.
func atRiskDay(deals ...ids.UUID) crmcontracts.Attention {
	risk := make([]crmcontracts.AttentionItem, len(deals))
	for i, deal := range deals {
		risk[i] = item(deal.String(), "deal_at_risk", func(i *crmcontracts.AttentionItem) {
			i.Subject = &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(deal)}
		})
	}
	return crmcontracts.Attention{AsOf: rankInstant, AtRisk: &risk}
}

// cutScan is a waiting lane whose scan was cut, holding one card on a deal.
type cutScan struct {
	hidingWork
	scanned ids.UUID
}

func (c cutScan) Unanswered(context.Context, time.Time, WaitingFor) ([]WaitingCustomer, bool, error) {
	return []WaitingCustomer{{ActivityID: ids.NewV7(), DealID: c.scanned, Since: rankInstant.Add(-time.Hour)}}, true, nil
}

// atRiskRowsOf names the at-risk rows of a page.
func atRiskRowsOf(page crmcontracts.Worklist) map[string]bool {
	atRisk := map[string]bool{}
	for _, row := range page.Queue {
		if row.Source == sourceAtRisk {
			atRisk[row.Id] = true
		}
	}
	return atRisk
}

// A ranked deal whose customer's card lies past a cut waiting scan is not
// shown at risk. Step B asks the tail's own read, so both drop the same deal.
func TestARankedDealWaitedOnPastACutScanIsNotAtRisk(t *testing.T) {
	waitedOn, scanned, quiet := ids.NewV7(), ids.NewV7(), ids.NewV7()
	store := &dealsWaitingStore{waitedOn: map[ids.UUID]bool{waitedOn: true, scanned: true}}
	s := (&Service{}).WithWaiting(cutScan{scanned: scanned}).WithWaitingBacklog(store)
	day := atRiskDay(waitedOn, scanned, quiet)

	read, unavailable := s.waitingCustomers(t.Context(), day)
	if unavailable != nil {
		t.Fatalf("the waiting read failed: %+v", unavailable)
	}
	atRisk := atRiskRowsOf(s.worklistFrom(t.Context(), day, scopeAll, "", 50, read, worklistCursor{}, nil))

	if atRisk[waitedOn.String()] {
		t.Error("the deal whose card lies past the scan is still shown at risk")
	}
	if !atRisk[quiet.String()] {
		t.Error("a deal nobody waits on lost its at-risk row")
	}
	if len(store.asked) != 1 || slices.Contains(store.asked[0], scanned) {
		t.Errorf("asked %v; want one read that leaves out the deal a scanned card names", store.asked)
	}
}

// A scan that was not cut read every waiting card, so the page asks nothing.
func TestAWholeWaitingScanAsksNoMore(t *testing.T) {
	store := &dealsWaitingStore{waitedOn: map[ids.UUID]bool{}}
	s := (&Service{}).WithWaitingBacklog(store)

	if got := s.dealsWaitingPastTheScan(t.Context(), atRiskDay(ids.NewV7()), waitingRead{read: true}); got != nil {
		t.Errorf("deals = %v, want none", got)
	}
	if len(store.asked) != 0 {
		t.Errorf("a whole scan asked the store %d times, want 0", len(store.asked))
	}
}

// failingDealsWaiting is a waiting backlog whose deal read fails.
type failingDealsWaiting struct{ dealsWaitingStore }

func (f *failingDealsWaiting) DealsWaiting(context.Context, time.Time, WaitingFor, []ids.UUID) (map[ids.UUID]bool, error) {
	return nil, errors.New("the read failed")
}

// A failed read past the scan is counted under its own label, so /metrics
// does not report it as the waiting lane failing.
func TestAFailedReadPastTheScanCountsUnderItsOwnLabel(t *testing.T) {
	before := LaneFailures()
	s := (&Service{}).WithWaitingBacklog(&failingDealsWaiting{})

	if got := s.dealsWaitingPastTheScan(t.Context(), atRiskDay(ids.NewV7()), waitingRead{read: true, cut: true}); got != nil {
		t.Errorf("deals = %v, want none from a failed read", got)
	}
	after := LaneFailures()
	own := LaneKey{Lane: waitingPastScanLane, Reason: LaneFailed}
	lane := LaneKey{Lane: string(sourceWaiting), Reason: LaneFailed}
	if after[own]-before[own] != 1 || after[lane] != before[lane] {
		t.Errorf("failures: own label %d, waiting lane %d; want 1 and 0",
			after[own]-before[own], after[lane]-before[lane])
	}
}
