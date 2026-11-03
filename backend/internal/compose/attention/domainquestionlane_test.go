// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubDomainQuestions struct {
	rows []DomainQuestion
	err  error
}

func (s *stubDomainQuestions) OpenDomainQuestions(context.Context) ([]DomainQuestion, error) {
	return s.rows, s.err
}

func domainQuestionLaneService(questions DomainQuestions) *Service {
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	if questions == nil {
		return svc
	}
	return svc.WithDomainQuestions(questions)
}

// TestADomainQuestionNamesTheDomainAndWhyItStopped holds the two facts the card
// is made of.
//
// The DOMAIN is the title rather than a sentence about it: the reader either
// recognises the name or does not, and that recognition is the whole decision.
// The machine's grounds are the supporting line, because "we could not tell"
// and "the mail is too old to trust the site" lead to different answers.
func TestADomainQuestionNamesTheDomainAndWhyItStopped(t *testing.T) {
	t.Parallel()
	asked := readInstant.Add(-48 * time.Hour)
	svc := domainQuestionLaneService(&stubDomainQuestions{rows: []DomainQuestion{{
		Domain:  "mckinsey.com",
		Reason:  "Nothing on the site named a company.",
		AskedAt: asked,
	}}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.DomainQuestions == nil || len(*out.DomainQuestions) != 1 {
		t.Fatalf("expected one domain question, got %v", out.DomainQuestions)
	}
	row := (*out.DomainQuestions)[0]
	if row.Id != "mckinsey.com" {
		t.Errorf("the row is identified by %q; an open question is named by its domain", row.Id)
	}
	if row.Title == nil || *row.Title != "mckinsey.com" {
		t.Errorf("title = %v, want the domain itself", row.Title)
	}
	if row.Detail == nil || *row.Detail != "Nothing on the site named a company." {
		t.Errorf("detail = %v, want the machine's own grounds", row.Detail)
	}
	if row.OccurredAt == nil || !row.OccurredAt.Equal(asked) {
		t.Errorf("occurred_at = %v, want when the question was last touched", row.OccurredAt)
	}
	if out.Counts.DomainQuestions == nil || *out.Counts.DomainQuestions != 1 {
		t.Errorf("count = %v, want 1", out.Counts.DomainQuestions)
	}
}

func TestADomainQuestionOffersAnswersAndRetry(t *testing.T) {
	t.Parallel()
	svc := domainQuestionLaneService(&stubDomainQuestions{rows: []DomainQuestion{{
		Domain: "botsync.co", Reason: "Nothing on the site named a company.", AskedAt: readInstant,
	}}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	row := (*out.DomainQuestions)[0]
	want := []crmcontracts.AttentionItemActions{"keep", "discard", "reopen"}
	if len(row.Actions) != len(want) {
		t.Fatalf("actions = %v, want exactly %v", row.Actions, want)
	}
	for i, action := range want {
		if row.Actions[i] != action {
			t.Errorf("action %d = %q, want %q", i, row.Actions[i], action)
		}
	}
}

func TestAVendorQuestionOffersCreateOrKeepAsVendorAndCarriesTheTriageSentence(t *testing.T) {
	t.Parallel()
	const sentence = "Classified as a mail or hosting vendor: the page sells managed hosting."
	svc := domainQuestionLaneService(&stubDomainQuestions{rows: []DomainQuestion{{
		Domain: "hosting.example", Reason: "The site reads as a mail or hosting vendor.",
		ReasonCode: contacts.PendingVendorVerdict, Note: sentence, AskedAt: readInstant,
	}, {
		Domain: "botsync.co", Reason: "Nothing on the site named a company.",
		ReasonCode: contacts.PendingUnevidenced, Note: "ignored", AskedAt: readInstant,
	}}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	rows := map[string]crmcontracts.AttentionItem{}
	for _, row := range *out.DomainQuestions {
		rows[row.Id] = row
	}
	vendor := rows["hosting.example"]
	want := []crmcontracts.AttentionItemActions{"keep", "keep_as_vendor", "discard"}
	if !slices.Equal(vendor.Actions, want) {
		t.Errorf("vendor actions = %v, want %v: a re-read would ask the same front page again", vendor.Actions, want)
	}
	if vendor.DomainNote == nil || *vendor.DomainNote != sentence {
		t.Errorf("vendor note = %v, want the triage's sentence", vendor.DomainNote)
	}
	other := rows["botsync.co"]
	if slices.Contains(other.Actions, "keep_as_vendor") || other.DomainNote != nil {
		t.Errorf("an unevidenced question offers %v with note %v; only a vendor question keeps a vendor verdict", other.Actions, other.DomainNote)
	}
	found := false
	for _, item := range queueOf(t, out, nil) {
		if item.Id != "hosting.example" {
			continue
		}
		found = true
		if item.DomainNote == nil || *item.DomainNote != sentence {
			t.Errorf("the worklist row lost the triage sentence: %v", item.DomainNote)
		}
	}
	if !found {
		t.Fatal("the vendor question never reached the worklist")
	}
}

// TestAnUnboundDomainQuestionLaneIsAbsentRatherThanEmpty holds the distinction
// every optional lane makes.
//
// An installation whose feed reads no triage ledger has no such questions to
// show. Answering with an empty lane would tell a reader they have answered
// everything, which is a different and false statement.
func TestAnUnboundDomainQuestionLaneIsAbsentRatherThanEmpty(t *testing.T) {
	t.Parallel()
	out, err := domainQuestionLaneService(nil).Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.DomainQuestions != nil {
		t.Errorf("an unbound lane answered %v; it must be absent, not empty", out.DomainQuestions)
	}
	if out.Counts.DomainQuestions != nil {
		t.Errorf("an unbound lane reported a count of %v; absent carries no number",
			out.Counts.DomainQuestions)
	}
}

func TestDomainMaintenanceFoldsWithoutEnteringFocus(t *testing.T) {
	for _, count := range []int{2, 3, 60, DomainQuestionLimit} {
		rows := make([]ranked, 0, count)
		for i := range count {
			rows = append(rows, classifyDomainQuestion(domainQuestionItem(DomainQuestion{
				Domain: fmt.Sprintf("company-%d.test", i), AskedAt: readInstant,
			}), readInstant))
		}
		grouped := foldRoutineDecisionsBounded(rows, map[crmcontracts.WorklistItemSource]bool{crmcontracts.WorklistItemSource(sourceDomainQuestion): count == DomainQuestionLimit})
		assertDomainMaintenanceGroup(t, grouped, count)
		for _, row := range append(rows, grouped...) {
			if focusEligible(row, readInstant) {
				t.Fatal("routine domain maintenance entered Focus")
			}
		}
	}
}

func (s *stubDomainQuestions) OwnsDomainQuestion(_ context.Context, domain string) (bool, error) {
	for _, row := range s.rows {
		if row.Domain == domain {
			return true, nil
		}
	}
	return false, nil
}

func TestDomainMaintenanceGroupDefersThroughItsMembers(t *testing.T) {
	group := ranked{item: crmcontracts.WorklistItem{Source: "batch", Batch: &crmcontracts.WorklistBatch{Key: "domain_questions", Count: 100}}}
	if got := dispositionsFor(group, ids.UUID{}); len(got) != 0 {
		t.Fatalf("bounded maintenance group offered a blanket disposition: %v", got)
	}
	member := ranked{item: crmcontracts.WorklistItem{Source: "domain_question"}}
	if got := dispositionsFor(member, ids.UUID{}); len(got) != 1 {
		t.Fatalf("individual question cannot be deferred: %v", got)
	}
}

func TestDomainQuestionAttentionCountsNameTheirBound(t *testing.T) {
	for _, count := range []int{0, 2, DomainQuestionLimit} {
		rows := make([]DomainQuestion, count)
		for i := range rows {
			rows[i] = DomainQuestion{Domain: fmt.Sprintf("domain-%d.test", i), AskedAt: readInstant}
		}
		out, err := domainQuestionLaneService(&stubDomainQuestions{rows: rows}).Assemble(pageReader())
		if err != nil {
			t.Fatal(err)
		}
		if out.Counts.DomainQuestions == nil || *out.Counts.DomainQuestions != count || out.DomainQuestionsMoreAvailable == nil || *out.DomainQuestionsMoreAvailable != (count == DomainQuestionLimit) {
			t.Fatalf("count=%d totals=%+v bounded=%v", count, out.Counts, out.DomainQuestionsMoreAvailable)
		}
	}
}

func assertDomainMaintenanceGroup(t *testing.T, grouped []ranked, count int) {
	t.Helper()
	if count < batchFloor {
		if len(grouped) != count {
			t.Fatalf("small backlog changed size: %d", len(grouped))
		}
		return
	}
	if len(grouped) != 1 || grouped[0].item.Batch == nil || grouped[0].item.Batch.Key != domainQuestionsBatchKey || grouped[0].item.Batch.Count != count {
		t.Fatalf("%d questions did not form one maintenance group: %+v", count, grouped)
	}
	if count == DomainQuestionLimit && (grouped[0].item.Batch.AtLeast == nil || !*grouped[0].item.Batch.AtLeast) {
		t.Fatal("bounded batch claimed a total")
	}
	if len(grouped[0].members) != count {
		t.Fatal("batch lost the member identities")
	}
}
