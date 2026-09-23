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

type recordingFollowUpStager struct{ summaries []string }

func (s *recordingFollowUpStager) HasPendingFollowUp(context.Context, ids.UUID) (bool, error) {
	return false, nil
}

func (s *recordingFollowUpStager) StageFollowUp(_ context.Context, _ ids.UUID, summary string, _ FollowUpProposal) error {
	s.summaries = append(s.summaries, summary)
	return nil
}

func stagedFollowUpSummary(t *testing.T, reconciler *FollowUpReconciler) string {
	t.Helper()
	stager := &recordingFollowUpStager{}
	reconciler.stager = stager
	cand := followUpCandidate{
		dealID:       ids.New[ids.DealKind](),
		dealName:     "Acme Rollout",
		activityID:   ids.New[ids.ActivityKind](),
		activityKind: "call",
		occurredAt:   time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
	}
	if err := reconciler.stage(context.Background(), cand, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("stage: %v", err)
	}
	if len(stager.summaries) != 1 {
		t.Fatalf("staged %d proposals, want exactly one", len(stager.summaries))
	}
	return stager.summaries[0]
}

// The staged summary is written in the language the injected resolver answers.
func TestAFollowUpSummaryFollowsTheInjectedLanguage(t *testing.T) {
	german := func(context.Context) textlang.Lang { return textlang.German }
	reconciler := NewFollowUpReconciler(nil, nil, nil).WithBaseLanguage(german)

	want := `Entwirf ein Follow-up zu "Acme Rollout": Nach dem Austausch (call) am 2026-09-21 ist kein nächster Schritt geplant.`
	if got := stagedFollowUpSummary(t, reconciler); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// A reconciler composed without a resolver writes the English sentence it
// always wrote.
func TestAFollowUpSummaryWithoutAResolverIsEnglish(t *testing.T) {
	want := `Draft a follow-up on "Acme Rollout" — a call on 2026-09-21 left no next step planned`
	if got := stagedFollowUpSummary(t, NewFollowUpReconciler(nil, nil, nil)); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// stageReasonFields lists the stage policy's sentences with their holes: a keyed
// one names the criterion, and a translation that drops the %s stops saying
// which criterion the card is about.
func stageReasonFields(said summaryCopy) map[string]struct {
	sentence string
	holes    int
} {
	type field = struct {
		sentence string
		holes    int
	}
	return map[string]field{
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
	}
}

// Every shipped language writes every stage-policy reason in its own words.
func TestEveryShippedLanguageWritesItsOwnStageReasons(t *testing.T) {
	english := stageReasonFields(summaryByLang[textlang.English])
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			for name, field := range stageReasonFields(summaryFor(lang)) {
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
// its English Reason is the sentence the policy always wrote.
func TestAStageReasonRendersInTheWritersLanguage(t *testing.T) {
	got := DecideStageMove(unmetFacts())
	if want := "the stage still asks for problem_confirmed, and nothing says it is settled"; got.Reason != want ||
		got.ReasonIn(textlang.English) != want {
		t.Errorf("English reason = %q / %q, want %q", got.Reason, got.ReasonIn(textlang.English), want)
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
