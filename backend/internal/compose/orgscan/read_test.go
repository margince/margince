// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// What Read makes of a lane: the floor when there is nothing to ask, the
// model's grounded findings when it answers, a typed lane error when it
// breaks, and the budget deferral passed through untouched for the job
// carrier to snooze on.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// cannedLane answers one reply, or fails with one error, without validating.
type cannedLane struct {
	reply string
	err   error
	asked int
}

func (l *cannedLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.asked++
	if l.err != nil {
		return model.Response{}, l.err
	}
	return model.Response{Text: l.reply}, nil
}

// checkedLane is a lane that validates before it answers, the way the
// structured lane does, and gives up with the validator's own words when
// its one reply does not pass.
type checkedLane struct{ reply string }

func (l *checkedLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}

func (l *checkedLane) CompleteValidated(_ context.Context, _ model.Request, validate ai.Validator) (model.Response, error) {
	if err := validate(l.reply); err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: l.reply}, nil
}

func TestWithNoLaneOrNothingToReadTheFloorAnswers(t *testing.T) {
	in, _ := scanInput()
	org := ids.New[ids.OrganizationKind]()
	if got, by, err := Read(t.Context(), nil, org, in, "en"); got != nil || by != crmcontracts.Deterministic || err != nil {
		t.Errorf("no lane: %v by %q, err %v; want the deterministic floor", got, by, err)
	}
	lane := &cannedLane{reply: reply()}
	silent := Input{Account: in.Account}
	if got, by, err := Read(t.Context(), lane, org, silent, "en"); got != nil || by != crmcontracts.Deterministic || err != nil || lane.asked != 0 {
		t.Errorf("no exchanges: %v by %q, err %v after %d calls; want the floor without a call", got, by, err, lane.asked)
	}
}

func TestReadKeepsWhatTheLaneGrounded(t *testing.T) {
	in, nudge := scanInput()
	lane := &checkedLane{reply: reply(finding(nudge, "did the sample reports get held up somewhere?"))}
	got, by, err := Read(t.Context(), lane, ids.New[ids.OrganizationKind](), in, "en")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if by != crmcontracts.Model || len(got) != 1 {
		t.Errorf("%d findings by %q, want the one grounded finding by the model", len(got), by)
	}
}

func TestALaneThatBreaksIsATypedErrorCarryingItsCause(t *testing.T) {
	in, nudge := scanInput()
	stranger := MessageIn{ID: ids.NewV7(), Kind: "email", Direction: "inbound", At: scanAt, Text: "unrelated"}
	for name, lane := range map[string]Completer{
		"gibberish, validated":       &checkedLane{reply: "not json"},
		"a fabricated citation":      &checkedLane{reply: reply(finding(stranger, "unrelated"))},
		"gibberish, unvalidated":     &cannedLane{reply: "not json"},
		"a lane that did not answer": &cannedLane{err: errors.New("upstream closed the connection")},
	} {
		t.Run(name, func(t *testing.T) {
			got, by, err := Read(t.Context(), lane, ids.New[ids.OrganizationKind](), in, "en")
			var lane *LaneError
			if !errors.As(err, &lane) {
				t.Fatalf("err = %v, want a LaneError", err)
			}
			if got != nil || by != crmcontracts.Deterministic {
				t.Errorf("%v by %q alongside the error; want nothing, on the floor", got, by)
			}
			if lane.Cause == nil || !errors.Is(err, lane.Cause) || !strings.HasPrefix(err.Error(), "account scan lane: ") {
				t.Errorf("LaneError = %q wrapping %v; want the cause reachable and named", err, lane.Cause)
			}
		})
	}
	_ = nudge
}

func TestABudgetDeferralPassesThroughForTheCarrierToSnoozeOn(t *testing.T) {
	in, _ := scanInput()
	resumes := scanAt.Add(30 * time.Minute)
	lane := &cannedLane{err: &ai.BudgetDeferralError{Task: ai.TaskAccountScan, NextAttemptAt: resumes}}
	_, _, err := Read(t.Context(), lane, ids.New[ids.OrganizationKind](), in, "en")
	var deferral *ai.BudgetDeferralError
	if !errors.As(err, &deferral) || !deferral.NextAttemptAt.Equal(resumes) {
		t.Fatalf("err = %v, want the deferral itself", err)
	}
	var broken *LaneError
	if errors.As(err, &broken) {
		t.Error("a deferral was reported as the lane breaking; the carrier would fail the job instead of snoozing it")
	}
}

func TestAFindingMissingItsWordsOrItsVerbIsRefused(t *testing.T) {
	in, nudge := scanInput()
	org := ids.New[ids.OrganizationKind]()
	wordless := finding(nudge, "did the sample reports get held up")
	wordless["title"] = ""
	oddVerb := finding(nudge, "did the sample reports get held up")
	oddVerb["action"] = "phone_them"
	kept, refused, err := ParseFindings(reply(wordless, oddVerb), org, in)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(kept) != 0 || len(refused) != 2 {
		t.Fatalf("kept %d, refused %v; want both refused", len(kept), refused)
	}
	if !strings.Contains(refused[0], "needs both") || !strings.Contains(refused[1], "phone_them") {
		t.Errorf("refusals = %v, want each to say what was missing", refused)
	}
}

func TestARefusalEchoesAModelTokenOnlyAsFarAsItIsBounded(t *testing.T) {
	in, nudge := scanInput()
	inflated := finding(nudge, "did the sample reports get held up")
	inflated["kind"] = strings.Repeat("x", 500)
	_, refused, err := ParseFindings(reply(inflated), ids.New[ids.OrganizationKind](), in)
	if err != nil || len(refused) != 1 {
		t.Fatalf("refused %v, err %v; want the one refusal", refused, err)
	}
	if len(refused[0]) > 200 || !strings.Contains(refused[0], "…") {
		t.Errorf("refusal is %d chars: %q; want the token clamped", len(refused[0]), refused[0])
	}
}

func TestTheReadKeepsAtMostItsCapAndSaysNothingOfTheRest(t *testing.T) {
	in, _ := scanInput()
	in.Messages = nil
	var findings []map[string]any
	for i := 0; i < maxFindings+2; i++ {
		message := MessageIn{
			ID: ids.NewV7(), Kind: "email", Direction: "inbound", At: scanAt,
			Text: "Can you confirm the delivery window for unit " + string(rune('A'+i)) + "?",
		}
		in.Messages = append(in.Messages, message)
		findings = append(findings, finding(message, "Can you confirm the delivery window"))
	}
	kept, refused, err := ParseFindings(reply(findings...), ids.New[ids.OrganizationKind](), in)
	if err != nil || len(refused) != 0 {
		t.Fatalf("refused %v, err %v; want every finding grounded", refused, err)
	}
	if len(kept) != maxFindings {
		t.Errorf("kept %d findings, want the cap of %d", len(kept), maxFindings)
	}
}

// A finding that asks for a step carries the step as the page writes it: the
// finding's own words as the subject, hung on the account, so the sentence
// the reader accepts and the task they get are one sentence.
func TestAFindingThatAsksForAStepCarriesTheStepItself(t *testing.T) {
	in, nudge := scanInput()
	org := ids.New[ids.OrganizationKind]()
	step := finding(nudge, "did the sample reports get held up")
	step["action"] = "add_task"
	kept, refused, err := ParseFindings(reply(step), org, in)
	if err != nil || len(refused) != 0 || len(kept) != 1 {
		t.Fatalf("kept %d, refused %v, err %v; want the one finding", len(kept), refused, err)
	}
	action := kept[0].Action
	if action == nil || action.Kind != crmcontracts.Organization360SuggestionActionKindAddTask || action.Task == nil {
		t.Fatalf("action = %+v, want add_task with its body", action)
	}
	if action.Task.Subject != *kept[0].Title || action.Task.Links == nil || len(*action.Task.Links) != 1 {
		t.Errorf("body = %+v, want the finding's title as the subject, linked once", action.Task)
	}
	link := (*action.Task.Links)[0]
	if string(link.EntityType) != "organization" || link.EntityId.String() != org.String() {
		t.Errorf("link = %+v, want the account itself", link)
	}
}

// A quote is bounded on both sides: too short to check a claim against, or
// long enough to be the message itself, and it is refused — except that a
// message shorter than the floor may be quoted whole.
func TestAQuoteIsNeitherAWordNorTheWholeMessage(t *testing.T) {
	in, nudge := scanInput()
	org := ids.New[ids.OrganizationKind]()
	brief := MessageIn{ID: ids.NewV7(), Kind: "email", Direction: "inbound", At: scanAt, Text: "Any news on the reports?"}
	in.Messages = append(in.Messages, brief)
	long := MessageIn{ID: ids.NewV7(), Kind: "email", Direction: "inbound", At: scanAt, Text: strings.Repeat("We need the sample reports before Thursday. ", 8)}
	in.Messages = append(in.Messages, long)
	for name, tc := range map[string]struct {
		finding map[string]any
		kept    bool
	}{
		"a word":                 {finding(nudge, "reports"), false},
		"the whole long message": {finding(long, strings.TrimSpace(long.Text)), false},
		"a short message, whole": {finding(brief, "Any news on the reports?"), true},
		"a bounded excerpt":      {finding(nudge, "did the sample reports get held up somewhere"), true},
	} {
		t.Run(name, func(t *testing.T) {
			kept, refused, err := ParseFindings(reply(tc.finding), org, in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if (len(kept) == 1) != tc.kept {
				t.Errorf("kept %d, refused %v; want kept=%v", len(kept), refused, tc.kept)
			}
		})
	}
}
