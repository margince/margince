// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

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

// summaryVerbs is the formatting verbs each sentence's call site passes, in
// order. The census holds its key set equal to summaryCopy's fields, so a
// sentence added to the struct with no row here fails.
var summaryVerbs = map[string][]string{
	"draftFollowUp":         {"%q", "%s", "%s"},
	"followUpSubject":       {"%s"},
	"followUpBody":          {"%s", "%s"},
	"followUpBodyQuoted":    {"%s", "%s", "%s"},
	"stageAutoApply":        nil,
	"stageAllSettled":       nil,
	"stageUnreliable":       nil,
	"stageNoCriteria":       nil,
	"stageStillAsksForKey":  {"%s"},
	"stageOurSideOnlyKey":   {"%s"},
	"stageClosing":          nil,
	"stageCrossPipeline":    nil,
	"stageSkips":            nil,
	"stageOptionalOpenKey":  {"%s"},
	"stageNotAgreedKey":     {"%s"},
	"stageUncertainKey":     {"%s"},
	"protectedMovedByYou":   nil,
	"protectedUndoneBefore": nil,
	"protectedTurnedDown":   nil,
	"protectedRecently":     nil,
}

// summarySentences reads every field of one set by reflection, so the census
// walks the struct itself rather than a list that could fall behind it.
func summarySentences(said summaryCopy) map[string]string {
	value := reflect.ValueOf(said)
	sentences := map[string]string{}
	for i := range value.NumField() {
		sentences[value.Type().Field(i).Name] = value.Field(i).String()
	}
	return sentences
}

// verbsIn answers a sentence's formatting verbs in order. Go's verbs are
// positional, so a translation that swaps %q and %s prints the date where the
// deal's name belongs.
func verbsIn(sentence string) []string {
	var verbs []string
	for i := 0; i+1 < len(sentence); i++ {
		if sentence[i] == '%' {
			verbs = append(verbs, sentence[i:i+2])
			i++
		}
	}
	return verbs
}

// Every shipped language writes every sentence in its own words, with the
// English sentence's formatting verbs in the English order.
func TestEveryShippedLanguageWritesItsOwnDealSentences(t *testing.T) {
	english := summarySentences(summaryByLang[textlang.English])
	for name := range english {
		if _, held := summaryVerbs[name]; !held {
			t.Errorf("%s is a field of summaryCopy with no row in summaryVerbs, so nothing holds its verbs", name)
		}
	}
	for name := range summaryVerbs {
		if _, exists := english[name]; !exists {
			t.Errorf("summaryVerbs holds %s, which summaryCopy no longer has", name)
		}
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := summaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for name, got := range summarySentences(said) {
				if got == "" {
					t.Errorf("%s is empty, and a blank sentence reaches the record as nothing at all", name)
					continue
				}
				if verbs := verbsIn(got); !slices.Equal(verbs, summaryVerbs[name]) {
					t.Errorf("%s carries verbs %v, its call site passes %v in that order: %q",
						name, verbs, summaryVerbs[name], got)
				}
				if lang != textlang.English && got == english[name] {
					t.Errorf("%s is the English sentence verbatim; the entry exists and the reader still gets English", name)
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
