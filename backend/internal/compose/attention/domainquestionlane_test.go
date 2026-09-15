// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The domain_question lane: the reader's own undecided domains, each carrying
// the two verbs that answer it.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type stubDomainQuestions struct {
	rows []DomainQuestion
	err  error
}

func (s *stubDomainQuestions) OpenDomainQuestions(context.Context) ([]DomainQuestion, error) {
	return s.rows, s.err
}

func domainQuestionLaneService(questions DomainQuestions) *Service {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
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

// TestADomainQuestionOffersBothAnswersAndNoThirdOne is the verb contract.
//
// Two verbs, and deliberately no `open`: the subject is a domain, which has no
// page to open, so a third control would send the reader nowhere. Keeping the
// list exact is what stops the row growing a verb the client cannot perform —
// the defect the source/action census exists to catch, asserted here at the
// lane rather than only across the whole queue.
func TestADomainQuestionOffersBothAnswersAndNoThirdOne(t *testing.T) {
	t.Parallel()
	svc := domainQuestionLaneService(&stubDomainQuestions{rows: []DomainQuestion{{
		Domain: "botsync.co", Reason: "Nothing on the site named a company.", AskedAt: readInstant,
	}}})
	out, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	row := (*out.DomainQuestions)[0]
	want := []crmcontracts.AttentionItemActions{"keep", "discard"}
	if len(row.Actions) != len(want) {
		t.Fatalf("actions = %v, want exactly %v", row.Actions, want)
	}
	for i, action := range want {
		if row.Actions[i] != action {
			t.Errorf("action %d = %q, want %q", i, row.Actions[i], action)
		}
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
