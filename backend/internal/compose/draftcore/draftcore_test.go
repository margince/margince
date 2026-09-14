// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// draft is a stand-in for whatever shape a surface returns. The loop only ever
// reads its body, which is the point of taking a reader rather than a type.
type draft struct{ body string }

func bodyOf(d draft) (string, []string) { return d.body, nil }

// scripted answers with each body in turn and records the corrections it was
// given, so a test can assert both what came back and what the model was told.
type scripted struct {
	bodies      []string
	corrections []string
	err         error
}

func (s *scripted) write(_ context.Context, correction string) (draft, error) {
	s.corrections = append(s.corrections, correction)
	if s.err != nil {
		return draft{}, s.err
	}
	i := len(s.corrections) - 1
	if i >= len(s.bodies) {
		i = len(s.bodies) - 1
	}
	return draft{body: s.bodies[i]}, nil
}

// A clean draft is served as-is, and — the part that matters for cost — the
// model is called exactly once.
func TestACleanDraftIsNotRetried(t *testing.T) {
	lane := &scripted{bodies: []string{"Hallo Marek,\n\nder Vertrag ist unterschrieben."}}

	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.German, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatalf("CorrectOnce errored on a clean draft: %v", err)
	}
	if len(lane.corrections) != 1 {
		t.Errorf("a clean draft should cost one call, got %d", len(lane.corrections))
	}
	if got.body != lane.bodies[0] {
		t.Errorf("the clean draft should be served unchanged, got %q", got.body)
	}
}

// A rejected phrase earns one retry, and the correction names the phrase — a
// model told only "try again" produces the same draft with new adjectives.
func TestARejectedPhraseEarnsOneRetryThatNamesIt(t *testing.T) {
	lane := &scripted{bodies: []string{
		"Hi Priya, just checking in on the integration.",
		"Hi Priya, the integration scope is ready. Is the project still live?",
	}}

	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(lane.corrections) != 2 {
		t.Fatalf("expected exactly one retry, got %d calls", len(lane.corrections))
	}
	if lane.corrections[0] != "" {
		t.Error("the first attempt should carry no correction")
	}
	if !strings.Contains(lane.corrections[1], "checking in") {
		t.Errorf("the correction should name the phrase, got %q", lane.corrections[1])
	}
	if got.body != lane.bodies[1] {
		t.Errorf("the corrected draft should be served, got %q", got.body)
	}
}

// One retry is the limit. A model that will not comply is not asked a third
// time — the cost is real and a deterministic floor sits underneath.
func TestTheModelIsNeverAskedMoreThanTwice(t *testing.T) {
	stubborn := "Hi Priya, just checking in as discussed."
	lane := &scripted{bodies: []string{stubborn, stubborn}}

	if _, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(lane.corrections) != 2 {
		t.Fatalf("a stubborn model should cost two calls and no more, got %d",
			len(lane.corrections))
	}
}

// A retry that makes things WORSE is discarded. Strictly worse: a TIE goes to
// the retry, because both attempts carry one finding often enough to matter —
// the model swaps "circling back" for "checking in" — and the retried one was
// at least written with the correction in hand.
func TestOnlyAStrictlyWorseRetryIsDiscarded(t *testing.T) {
	tied := &scripted{bodies: []string{
		"Hi Priya, just circling back on this.",
		"Hi Priya, just checking in on this.",
	}}
	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, tied.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.body != tied.bodies[1] {
		t.Errorf("a tie should go to the corrected attempt, got %q", got.body)
	}

	lane := &scripted{bodies: []string{
		"Hi Priya, just checking in.",
		"Hi Priya, just checking in, as discussed, and touching base.",
	}}

	worse, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if worse.body != lane.bodies[0] {
		t.Errorf("the strictly worse retry should be discarded, got %q", worse.body)
	}
}

// A retry that FAILS leaves the first draft standing. It carries the defect and
// it is still a real message a human can edit, which beats refusing to answer.
func TestAFailedRetryLeavesTheFirstDraftStanding(t *testing.T) {
	first := "Hi Priya, just checking in on the integration."
	lane := &failOnRetry{first: first}

	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatalf("a failed retry must not fail the draft: %v", err)
	}
	if got.body != first {
		t.Errorf("the first draft should stand, got %q", got.body)
	}
}

// A first attempt that fails IS a failure: there is nothing to serve, and the
// caller's own floor is the answer.
func TestAFailedFirstAttemptIsReturnedAsAnError(t *testing.T) {
	lane := &scripted{bodies: []string{"unused"}, err: errors.New("model unavailable")}

	if _, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandFresh, false, lane.write, bodyOf, nil, nil); err == nil {
		t.Fatal("a failed first attempt should return its error")
	}
}

type failOnRetry struct {
	first string
	calls int
}

func (f *failOnRetry) write(context.Context, string) (draft, error) {
	f.calls++
	if f.calls > 1 {
		return draft{}, errors.New("model unavailable on retry")
	}
	return draft{body: f.first}, nil
}

// recorder captures what the loop reported, so the observability the loop took
// over from its callers is proven rather than assumed.
type recorder struct {
	failed     int
	notCleared []string
}

func (r *recorder) RetryFailed(context.Context, int, error) { r.failed++ }
func (r *recorder) RetryDidNotClear(_ context.Context, _ draftcheck.Rule, phrase string, _ int) {
	r.notCleared = append(r.notCleared, phrase)
}

// A retry that does not help is invisible from the outside — the caller gets a
// draft either way — so the loop has to say so. Both ways of not helping are
// reported, and a retry that DOES help says nothing.
func TestTheLoopReportsARetryThatDidNotHelp(t *testing.T) {
	stubborn := "Hi Priya, just checking in as discussed."
	seen := &recorder{}
	lane := &scripted{bodies: []string{stubborn, stubborn}}

	if _, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, seen); err != nil {
		t.Fatal(err)
	}
	if len(seen.notCleared) != 1 {
		t.Errorf("surviving phrasing should be reported once, got %v", seen.notCleared)
	}
	if seen.failed != 0 {
		t.Error("a retry that answered is not a failed retry")
	}

	broken := &recorder{}
	if _, err := draftcore.CorrectOnce(context.Background(), textlang.English,
		convstate.BandMonths, false, (&failOnRetry{first: stubborn}).write, bodyOf, nil, broken); err != nil {
		t.Fatal(err)
	}
	if broken.failed != 1 {
		t.Errorf("a failed retry should be reported once, got %d", broken.failed)
	}

	quiet := &recorder{}
	clean := &scripted{bodies: []string{stubborn, "Hi Priya, the scope is ready."}}
	if _, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, clean.write, bodyOf, nil, quiet); err != nil {
		t.Fatal(err)
	}
	if quiet.failed != 0 || len(quiet.notCleared) != 0 {
		t.Errorf("a retry that worked should report nothing, got %+v", quiet)
	}
}

// A FALSE CLAIM LOSES TO A PHRASING TIC, however many phrases the tic carries.
//
// This is the case the raw finding count could not see and that deduplicating
// by rule does not fix either. The rules do not match at the same rate: the
// band-gated loops append one finding per matching phrase, so a draft saying
// the same wrong thing three ways scored 3, while a draft inventing a call
// scored 1 and was served as the better one.
//
// Both counts are beside the point. A rep sends what the product wrote, and an
// invented conversation reaches the recipient as the company's own word; a
// stale opener is a habit. Severity is the comparison a reader would have made
// first, so it is the one made first here.
func TestAFalseClaimIsWorseThanAnyNumberOfPhrasingTics(t *testing.T) {
	// The first attempt is three wellbeing openers — Style, three findings, one
	// rule. The retry invents a conversation on an unthreaded message — Claim,
	// one finding, one rule. Every count says the retry is better.
	lane := &scripted{bodies: []string{
		"I hope this finds you well. I hope you are well. Hope you're doing well. " +
			"The quote is attached and the depot slots are open on Tuesday.",
		"It was great speaking with you earlier. The quote is attached and the " +
			"depot slots are open on Tuesday.",
	}}
	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.body != lane.bodies[0] {
		t.Errorf("a draft claiming a conversation that never happened was served over one whose "+
			"only fault is three stale openers — the recipient reads the first as the company's "+
			"own word.\n  served: %q", got.body)
	}
}

// And the severity is what a surface is told, not only what the loop used.
//
// "The retry did not clear" means something different for a false claim than
// for a phrasing tic, and an operator scanning the line should not have to know
// every rule by name to tell them apart.
func TestTheUnclearedRetryIsReportedWithItsSeverity(t *testing.T) {
	stubborn := "It was great speaking with you earlier. The quote is attached."
	seen := &severityRecorder{}
	lane := &scripted{bodies: []string{stubborn, stubborn}}
	if _, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, seen); err != nil {
		t.Fatal(err)
	}
	if len(seen.rules) != 1 {
		t.Fatalf("the loop reported %d uncleared retries, want 1", len(seen.rules))
	}
	if got := seen.rules[0].Severity(); got != draftcheck.Claim {
		t.Errorf("the uncleared retry was reported as severity %v, want Claim — %q states a "+
			"conversation the input does not carry", got, stubborn)
	}
}

// severityRecorder keeps the RULE rather than its phrase, which is what the
// observer gained: the phrase says where, the rule says how bad.
type severityRecorder struct{ rules []draftcheck.Rule }

func (severityRecorder) RetryFailed(context.Context, int, error) {}
func (r *severityRecorder) RetryDidNotClear(_ context.Context, rule draftcheck.Rule, _ string, _ int) {
	r.rules = append(r.rules, rule)
}

// THE TICKET'S OWN EXAMPLE: two false statements lose to three phrasings of one.
//
// The rules do not report at the same rate. The band-gated loops append one
// finding per matching phrase, so "circling back … as discussed … touching
// base" scores 3 while the world-claim rules report once per rule, so an
// invented conversation plus an attributed claim scores 2. The raw count then
// served the draft making TWO separate false statements over the one making
// one, three ways.
//
// Deduplicating by rule is what makes the two styles comparable: 2 rules
// against 1. Severity cannot decide this pair — both are claims about the world
// — which is why this case is here beside the one severity does decide.
func TestTwoBrokenRulesLoseToThreePhrasingsOfOne(t *testing.T) {
	lane := &scripted{bodies: []string{
		"It was great speaking with you earlier.\n\nYou mentioned the depot slots " +
			"were the blocker, so the quote is attached.",
		"Circling back as discussed.\n\nTouching base on the quote, which is attached.",
	}}
	got, err := draftcore.CorrectOnce(context.Background(),
		textlang.English, convstate.BandMonths, false, lane.write, bodyOf, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.body != lane.bodies[1] {
		t.Errorf("a draft breaking TWO rules was served over one breaking one three ways — the "+
			"raw finding count is not comparable across rules that do not match at the same "+
			"rate.\n  served: %q", got.body)
	}
}
