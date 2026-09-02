// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var momentNow = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func due(days int) *time.Time {
	at := momentNow.AddDate(0, 0, days)
	return &at
}

func commitment(body string, dueInDays int, who string) people.OrgCommitment {
	return people.OrgCommitment{
		ID:          ids.NewV7(),
		PersonID:    ids.From[ids.PersonKind](ids.NewV7()),
		PersonName:  who,
		Body:        body,
		SourceQuote: "Ich schicke es Ihnen.",
		ActivityID:  ids.NewV7(),
		DueAt:       due(dueInDays),
		OccurredAt:  momentNow.AddDate(0, 0, -30),
	}
}

// stepsOf pairs tasks with their filing moments, the way the read hands them to
// the card. Every task here was filed thirty days ago unless a case says
// otherwise, so a tie-break on the filing moment is visible rather than
// accidental.
func stepsOf(rows ...crmcontracts.Organization360NextStep) ([]crmcontracts.Organization360NextStep, []time.Time) {
	filed := make([]time.Time, len(rows))
	for i := range rows {
		filed[i] = momentNow.AddDate(0, 0, -30)
	}
	return rows, filed
}

// accountMomentOf reads as one call at the call sites, where pairing tasks with
// their filing moments is not the thing under test.
func accountMomentOf(
	tasks []crmcontracts.Organization360NextStep, claims []people.OrgCommitment,
) crmcontracts.PersonMoment {
	rows, filed := stepsOf(tasks...)
	return accountMoment(momentNow, rows, filed, claims)
}

// tasksOf is the account's open tasks, as the read hands them over.
func tasksOf(rows ...crmcontracts.Organization360NextStep) []crmcontracts.Organization360NextStep {
	return rows
}

func task(subject string, dueInDays int) crmcontracts.Organization360NextStep {
	return crmcontracts.Organization360NextStep{
		ActivityId: openapi_types.UUID(ids.NewV7()),
		Subject:    subject,
		DueAt:      due(dueInDays),
	}
}

// The company page had no card at all: an account could owe three promises and
// open on a screen that said nothing about any of them.
func TestAnAccountWithAnOverduePromiseSaysSo(t *testing.T) {
	got := accountMomentOf(nil, []people.OrgCommitment{
		commitment("Send the signed order form", -4, "Carol Wagner"),
	})

	if got.Rule != crmcontracts.PersonMomentRuleOverduePromise {
		t.Fatalf("rule = %q, want overdue_promise", got.Rule)
	}
	if got.Headline != "You owe Carol Wagner: Send the signed order form" {
		t.Errorf("headline = %q; an account card has to name who is waiting", got.Headline)
	}
	if got.WhyNow != "Due 4 days ago and still open." {
		t.Errorf("why-now = %q, want the lateness", got.WhyNow)
	}
	if got.Evidence[0].Snippet == nil {
		t.Error("no quote; the sentence the promise was made in is what a claim carries")
	}
}

// The same rule the contact page uses: which of the two places a promise was
// recorded may not decide what a reader is shown.
func TestTheAccountCardRanksBothSourcesByDateAlone(t *testing.T) {
	claim := []people.OrgCommitment{commitment("Send the quote", -1, "Carol Wagner")}

	// The task slipped longer ago, so the claim — the latest slip — leads.
	got := accountMomentOf(tasksOf(task("Return the redlines", -20)), claim)
	if got.Headline != "You owe Carol Wagner: Send the quote" {
		t.Errorf("headline = %q, want the promise that slipped most recently", got.Headline)
	}

	// Reverse the dates and the task leads, on the same rule.
	older := []people.OrgCommitment{commitment("Send the quote", -20, "Carol Wagner")}
	got = accountMomentOf(tasksOf(task("Return the redlines", -1)), older)
	if got.Headline != "You owe them: Return the redlines" {
		t.Errorf("headline = %q, want the task, whose deadline passed most recently", got.Headline)
	}
}

// A promise still ahead is owed and belongs on the card; only an account with
// nothing outstanding gets the quiet state.
func TestAnUpcomingPromiseIsTheCard(t *testing.T) {
	got := accountMomentOf(nil, []people.OrgCommitment{
		commitment("Send the security questionnaire", 3, "Carol Wagner"),
	})
	if got.Rule != crmcontracts.PersonMomentRuleOpenPromise {
		t.Fatalf("rule = %q, want open_promise", got.Rule)
	}
	if got.WhyNow != "Due in 3 days." {
		t.Errorf("why-now = %q, want the deadline still ahead", got.WhyNow)
	}
}

func TestAnAccountOwingNothingGetsTheQuietState(t *testing.T) {
	got := accountMomentOf(nil, nil)
	if got.Rule != crmcontracts.PersonMomentRuleNothingNeeded {
		t.Errorf("rule = %q, want nothing_needed", got.Rule)
	}
	if got.Headline == "" {
		t.Error("the quiet state is an answer, not a blank card")
	}
}

// Two promises on one account must dismiss apart. The dismissal is one row per
// claim key, so a key naming only the rung would let putting the first away
// silence the second.
func TestTwoAccountPromisesDismissApart(t *testing.T) {
	first := accountClaimCard(momentNow, commitment("Send the NDA", -2, "Carol Wagner"), true)
	second := accountClaimCard(momentNow, commitment("Send the quote", -2, "Carol Wagner"), true)
	if first.ClaimKey == second.ClaimKey {
		t.Errorf("both carry claim key %q; dismissing one would silence the other", first.ClaimKey)
	}
}

// A promise whose person the caller may not name still belongs on the card —
// the account owes it either way. The sentence drops the name, never the
// promise.
func TestAPromiseWithNoNameableContactStillShows(t *testing.T) {
	anonymous := commitment("Send the report", -1, "")
	got := accountClaimCard(momentNow, anonymous, true)
	if got.Headline != "You owe them: Send the report" {
		t.Errorf("headline = %q, want the promise without a name", got.Headline)
	}
}

// An account task filed against no record offers no button rather than one
// that would land the reader nowhere.
func TestATaskLinkedToNothingOffersNoDestination(t *testing.T) {
	got := accountTaskCard(momentNow, task("Renew the certificate", -1), true)
	if got.RecommendedAction.State != crmcontracts.PersonMomentActionStateBlocked {
		t.Errorf("action state = %q, want blocked when there is no record to open",
			got.RecommendedAction.State)
	}
	if got.RecommendedAction.Destination != nil {
		t.Error("a blocked action must not carry a destination the reader cannot reach")
	}
}

// A task filed against a contact routes there, which is where the reader can
// see the conversation and act.
func TestATaskLinkedToAContactRoutesThere(t *testing.T) {
	person := openapi_types.UUID(ids.NewV7())
	step := task("Send the plan", -1)
	step.LinkedPersonId = &person

	got := accountTaskCard(momentNow, step, true)

	if got.RecommendedAction.State != crmcontracts.PersonMomentActionStateAvailable {
		t.Fatalf("action state = %q, want available", got.RecommendedAction.State)
	}
	if got.RecommendedAction.Destination.EntityId == nil ||
		*got.RecommendedAction.Destination.EntityId != person {
		t.Error("the action does not route to the contact the task is filed against")
	}
}

// The filing moment breaks ties between two promises sharing a due date, and
// the contact page ranks on it too. Dropping it on this page would make the
// same two promises rank differently depending on which record a reader came
// in through — which is the disagreement this card exists to remove.
func TestTwoPromisesDueTheSameDayRankByWhenTheyWerePromised(t *testing.T) {
	sameDay := 4
	older := task("Return the redlines", sameDay)
	newer := commitment("Send the quote", sameDay, "Carol Wagner")
	// The claim was promised yesterday; the task thirty days ago, which stepsOf
	// applies. The older promise leads.
	newer.OccurredAt = momentNow.AddDate(0, 0, -1)

	got := accountMomentOf(tasksOf(older), []people.OrgCommitment{newer})

	if got.Headline != "You owe them: Return the redlines" {
		t.Errorf("headline = %q; of two promises due the same day the one promised first leads", got.Headline)
	}
}

// A task whose filing moment never reached the card ranked as unknown and lost
// every tie. The read hands the moments over beside the rows for this reason,
// so a task with no moment is the read failing rather than the data.
func TestATaskCarriesTheMomentItWasFiled(t *testing.T) {
	rows, filed := stepsOf(task("Return the redlines", 2))
	if len(filed) != len(rows) {
		t.Fatalf("%d tasks came back with %d filing moments; the two slices have to line up", len(rows), len(filed))
	}
	items := accountPromises(rows, filed, nil)
	if len(items) != 1 {
		t.Fatalf("built %d promises from one task", len(items))
	}
	if items[0].FiledAt.IsZero() {
		t.Error("the task's filing moment is zero, so it loses every tie-break it should win")
	}
}

// A promise the caller may not see must not read as an account with nothing
// outstanding. The read reports what it could not admit, and the page says so.
func TestAWithheldPromiseIsAdmittedRatherThanSilent(t *testing.T) {
	// The store reports incompleteness; this holds the contract that the page
	// turns it into a visible admission rather than dropping it.
	page := &crmcontracts.Organization360{}
	markAttentionWithheld(page)
	if page.AttentionWithheld == nil || !*page.AttentionWithheld {
		t.Error("a withheld promise left no mark; a silent card reads as 'this account owes nothing'")
	}
}
