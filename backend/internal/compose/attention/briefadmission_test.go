// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func withCurrentIntervention() func(*crmcontracts.AttentionItem) {
	return func(item *crmcontracts.AttentionItem) {
		quiet := 20
		item.QuietDays = &quiet
		if item.Subject == nil {
			withDealSubject(ids.NewV7())(item)
		}
	}
}

func TestBriefAdmissionPrecedesCountsPagingAndFocus(t *testing.T) {
	deal := ids.NewV7()
	unsupported := item("unsupported", "brief_item", withKind("stalled"), withDealSubject(deal))
	supported := item("supported", "brief_item", withKind("moved"), withCurrentIntervention())
	task := item("task", "task", withDue(rankInstant))
	decision := item("decision", "approval", withKind("send_email"), decidable())
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{unsupported, supported}, Planned: []crmcontracts.AttentionItem{task}, NeedsYou: []crmcontracts.AttentionItem{decision}}
	page := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 2, waitingRead{}, worklistCursor{}, nil)
	total := 0
	for _, count := range page.Counts {
		total += count.Considered
	}
	if total != 3 || len(page.Queue) != 2 {
		t.Fatalf("admission did not precede count/page: total=%d rows=%d", total, len(page.Queue))
	}
	rows := classifyDay(day, rankInstant, dayMoney{})
	for _, row := range rows {
		if row.item.Id == unsupported.Id {
			t.Fatal("a signal with no current basis entered the queue")
		}
	}
	focus := focusOf(rows, rows, rankInstant, ids.Nil)
	if focus.Total != 3 {
		t.Fatalf("focus total=%d, want three supported obligations", focus.Total)
	}
}

func TestBriefAdmissionCarriesCurrentScopedObservation(t *testing.T) {
	deal := ids.NewV7()
	brief := item("brief", "brief_item", withKind("opportunity"), withDealSubject(deal))
	brief.Deal = &crmcontracts.AttentionDealFacts{WinProbability: new(65)}
	risk := item(deal.String(), "deal_at_risk", withDealSubject(deal))
	risk.Deal = &crmcontracts.AttentionDealFacts{NoNextStep: new(true)}
	quiet := 18
	risk.QuietDays = &quiet
	day := crmcontracts.Attention{AsOf: rankInstant, ThisMorning: []crmcontracts.AttentionItem{brief}, AtRisk: lane(risk)}
	admitted := admittedBriefItems(day, rankInstant)
	if len(admitted) != 1 {
		t.Fatal("current observation failed to admit the suggestion")
	}
	if admitted[0].Deal.WinProbability == nil || *admitted[0].Deal.WinProbability != 65 || admitted[0].Deal.NoNextStep == nil || !*admitted[0].Deal.NoNextStep {
		t.Fatal("observations replaced independent stage facts")
	}
	row := classifyBriefItem(admitted[0], rankInstant, dayMoney{})
	if !hasReasonKind(row.item.Because, "quiet_days") {
		t.Fatal("current observation did not reach the reason")
	}
	day.AtRisk = nil
	if got := admittedBriefItems(day, rankInstant); len(got) != 0 {
		t.Fatal("unread observation was treated as proof")
	}
}

func TestFilteredRiskScanRetainsItsTruncation(t *testing.T) {
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{rows: []RiskyDeal{{DealID: ids.NewV7(), Name: "Quiet deal", QuietDays: 19}}, cut: true}, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	page, err := svc.Worklist(pageReader(), scopeAll, "", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if !page.Readings.MoreAvailable {
		t.Fatal("filtered scan was reported complete")
	}
}
