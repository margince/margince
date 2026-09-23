// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Every shipped language writes the follow-up summary in its own words, with
// the English sentence's holes in the English order: the verbs are positional,
// so a swapped pair prints the date where the deal's name belongs.
func TestEveryShippedLanguageWritesItsOwnFollowUpSummary(t *testing.T) {
	english, ok := summaryByLang[textlang.English]
	if !ok {
		t.Fatal("English has no summary set, and it is the fallback for everything else")
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := summaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			got := said.draftFollowUp
			if got == "" {
				t.Fatal("draftFollowUp is empty, and a blank summary reaches the inbox as nothing at all")
			}
			if strings.Count(got, "%q") != 1 || strings.Count(got, "%s") != 2 || strings.Count(got, "%") != 3 {
				t.Errorf("draftFollowUp must carry exactly one %%q and two %%s: %q", got)
			}
			if strings.Index(got, "%q") > strings.Index(got, "%s") {
				t.Errorf("draftFollowUp puts a %%s before the deal's %%q: %q", got)
			}
			if lang != textlang.English && got == english.draftFollowUp {
				t.Error("draftFollowUp is the English sentence verbatim; the entry exists and the reader still gets English")
			}
		})
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageStagesTheFollowUpInEnglish(t *testing.T) {
	unshipped := func(context.Context) textlang.Lang { return textlang.Lang("kl") }
	if got := summaryIn(context.Background(), unshipped); got != summaryByLang[textlang.English] {
		t.Errorf("an unshipped language answered %+v, want the English set", got)
	}
}

type stagedFollowUp struct {
	summary  string
	proposal FollowUpProposal
}

type recordingFollowUpStager struct{ staged []stagedFollowUp }

func (s *recordingFollowUpStager) HasPendingFollowUp(context.Context, ids.UUID) (bool, error) {
	return false, nil
}

func (s *recordingFollowUpStager) StageFollowUp(_ context.Context, _ ids.UUID, summary string, proposal FollowUpProposal) error {
	s.staged = append(s.staged, stagedFollowUp{summary: summary, proposal: proposal})
	return nil
}

// stageOneFollowUp stages a call on "Acme Rollout" in the copy set the
// reconciler's resolver answers, the way reconcileWorkspace resolves it.
func stageOneFollowUp(t *testing.T, reconciler *FollowUpReconciler, subject *string) stagedFollowUp {
	t.Helper()
	stager := &recordingFollowUpStager{}
	reconciler.stager = stager
	cand := followUpCandidate{
		dealID:       ids.New[ids.DealKind](),
		dealName:     "Acme Rollout",
		activityID:   ids.New[ids.ActivityKind](),
		activityKind: "call",
		occurredAt:   time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
		subject:      subject,
	}
	ctx := context.Background()
	if err := reconciler.stage(ctx, cand, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), summaryIn(ctx, reconciler.language)); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if len(stager.staged) != 1 {
		t.Fatalf("staged %d proposals, want exactly one", len(stager.staged))
	}
	return stager.staged[0]
}

// The staged summary and the drafted task are written in the language the
// injected resolver answers; the exchange's own subject stays as it was.
func TestAFollowUpFollowsTheInjectedLanguage(t *testing.T) {
	german := func(context.Context) textlang.Lang { return textlang.German }
	offer := "Angebot Q4"
	got := stageOneFollowUp(t, NewFollowUpReconciler(nil, nil, nil).WithBaseLanguage(german), &offer)

	for field, pair := range map[string][2]string{
		"summary": {got.summary, `Entwirf ein Follow-up zu "Acme Rollout": Nach dem Austausch (call) am 2026-09-21 ist kein nächster Schritt geplant.`},
		"subject": {got.proposal.Subject, "Follow-up zu Acme Rollout"},
		"body":    {got.proposal.Body, "Follow-up zum Austausch (call) „Angebot Q4“ vom 2026-09-21. Im Verlauf steht noch kein nächster Schritt."},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}
}

// A reconciler composed without a resolver writes the English it always wrote.
func TestAFollowUpWithoutAResolverIsEnglish(t *testing.T) {
	offer := "Q4 offer"
	quoted := stageOneFollowUp(t, NewFollowUpReconciler(nil, nil, nil), &offer)
	bare := stageOneFollowUp(t, NewFollowUpReconciler(nil, nil, nil), nil)

	for field, pair := range map[string][2]string{
		"summary":     {bare.summary, `Draft a follow-up on "Acme Rollout" — a call on 2026-09-21 left no next step planned`},
		"subject":     {bare.proposal.Subject, "Follow up on Acme Rollout"},
		"body":        {bare.proposal.Body, "Follow up on the call from 2026-09-21. No next step is on the timeline yet."},
		"quoted body": {quoted.proposal.Body, "Follow up on the call “Q4 offer” from 2026-09-21. No next step is on the timeline yet."},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}
}

// sentenceFields lists every %s-only sentence with its holes: a translation
// that drops one stops naming the criterion, the deal or the date it is about.
func sentenceFields(said summaryCopy) map[string]struct {
	sentence string
	holes    int
} {
	type field = struct {
		sentence string
		holes    int
	}
	return map[string]field{
		"followUpSubject":       {said.followUpSubject, 1},
		"followUpBody":          {said.followUpBody, 2},
		"followUpBodyQuoted":    {said.followUpBodyQuoted, 3},
		"stageAutoApply":        {said.stageAutoApply, 0},
		"stageAllSettled":       {said.stageAllSettled, 0},
		"stageUnreliable":       {said.stageUnreliable, 0},
		"stageNoCriteria":       {said.stageNoCriteria, 0},
		"stageStillAsksForKey":  {said.stageStillAsksForKey, 1},
		"stageOurSideOnlyKey":   {said.stageOurSideOnlyKey, 1},
		"stageClosing":          {said.stageClosing, 0},
		"stageCrossPipeline":    {said.stageCrossPipeline, 0},
		"stageSkips":            {said.stageSkips, 0},
		"stageOptionalOpenKey":  {said.stageOptionalOpenKey, 1},
		"stageNotAgreedKey":     {said.stageNotAgreedKey, 1},
		"stageUncertainKey":     {said.stageUncertainKey, 1},
		"protectedMovedByYou":   {said.protectedMovedByYou, 0},
		"protectedUndoneBefore": {said.protectedUndoneBefore, 0},
		"protectedTurnedDown":   {said.protectedTurnedDown, 0},
		"protectedRecently":     {said.protectedRecently, 0},
	}
}

// Every shipped language writes every task line and stage-policy reason in its
// own words.
func TestEveryShippedLanguageWritesItsOwnSentences(t *testing.T) {
	english := sentenceFields(summaryByLang[textlang.English])
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			for name, field := range sentenceFields(summaryFor(lang)) {
				if field.sentence == "" {
					t.Errorf("%s is empty, and a blank reason reaches the card as nothing at all", name)
					continue
				}
				if strings.Count(field.sentence, "%s") != field.holes || strings.Count(field.sentence, "%") != field.holes {
					t.Errorf("%s must carry exactly %d %%s and no other verb: %q", name, field.holes, field.sentence)
				}
				if lang != textlang.English && field.sentence == english[name].sentence {
					t.Errorf("%s is the English sentence verbatim", name)
				}
			}
		})
	}
}

// A decision's reason renders in whichever language the writer resolved, and
// its English is the sentence the policy always wrote.
func TestAStageReasonRendersInTheWritersLanguage(t *testing.T) {
	got := DecideStageMove(unmetFacts())
	if want := "the stage still asks for problem_confirmed, and nothing says it is settled"; got.ReasonIn(textlang.English) != want {
		t.Errorf("English reason = %q, want %q", got.ReasonIn(textlang.English), want)
	}
	if want := "die Phase verlangt noch problem_confirmed, und nichts zeigt, dass es erfüllt ist"; got.ReasonIn(textlang.German) != want {
		t.Errorf("German reason = %q, want %q", got.ReasonIn(textlang.German), want)
	}

	protected := settledFacts()
	protected.Protection = ProtectionRejected
	if want := "du hast diesen Schritt abgelehnt, und seitdem ist nichts Neues bekannt geworden"; DecideStageMove(protected).ReasonIn(textlang.German) != want {
		t.Errorf("German protection reason = %q, want %q", DecideStageMove(protected).ReasonIn(textlang.German), want)
	}
}

// Every protection reads its own sentence, and a value protectionReason has no
// case for still reads one: a protected deal's card never goes blank.
func TestEveryProtectionSaysItsOwnReason(t *testing.T) {
	seen := map[string]Protection{}
	for _, protection := range []Protection{
		ProtectionHumanMove, ProtectionReversal, ProtectionRejected, Protection("unmapped"),
	} {
		said := protectionReason(protection).in(textlang.English)
		if said == "" {
			t.Errorf("protection %q reads no sentence", protection)
		}
		if other, dup := seen[said]; dup {
			t.Errorf("protections %q and %q read the same sentence %q", other, protection, said)
		}
		seen[said] = protection
	}
}
