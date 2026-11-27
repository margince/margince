// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/dormancy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var dormancyNow = time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC)

// coldAccount is an account on the dormant rung, 70 days after its last
// activity, owned by somebody, with the follow-up switched on.
func coldAccount(owner ids.UUID) suggestionInputs {
	last := occurredday.Moment{At: dormancyNow.AddDate(0, 0, -70)}
	return suggestionInputs{
		language: textlang.English, companyName: "Acme",
		dormancy: dormancyInputs{
			reading:      dormancy.Default.Read(&last, dormancyNow, time.UTC),
			ladder:       dormancy.Default,
			lastActivity: &last,
			followUp:     true,
			owner:        &owner,
		},
	}
}

func TestADormantAccountSuggestsAFollowUpToItsOwner(t *testing.T) {
	t.Parallel()
	company := ids.From[ids.CompanyKind](ids.NewV7())
	owner := ids.NewV7()
	in := coldAccount(owner)

	got := goneDormantSuggestion(company, in)
	if got == nil || got.Kind != suggestGoneDormant {
		t.Fatalf("a dormant account with an owner raised %+v, want a gone_dormant suggestion", got)
	}
	if got.Action == nil || got.Action.Kind != "add_task" || got.Action.Task == nil {
		t.Fatalf("the suggestion prepares no task: %+v", got.Action)
	}
	if assignee := got.Action.Task.AssigneeId; assignee == nil || ids.UUID(*assignee) != owner {
		t.Errorf("the task is prepared for %v, want the owner %v", assignee, owner)
	}
	if got.DueAt == nil || !got.DueAt.Equal(in.dormancy.lastActivity.At) {
		t.Errorf("due_at = %v, want the last activity the silence is counted from", got.DueAt)
	}
	if got.Title == nil || *got.Title != "Get back in touch with Acme" {
		t.Errorf("title = %v", got.Title)
	}
}

// One silent stretch raises one suggestion. Going from dormant to stale keeps
// the fingerprint, so a dismissal holds. New activity starts a new stretch.
func TestOneSilentStretchRaisesTheFollowUpOnce(t *testing.T) {
	t.Parallel()
	company := ids.From[ids.CompanyKind](ids.NewV7())
	in := coldAccount(ids.NewV7())
	dormant := goneDormantSuggestion(company, in)

	in.dormancy.reading = dormancy.Default.Read(in.dormancy.lastActivity, dormancyNow.AddDate(0, 0, 40), time.UTC)
	stale := goneDormantSuggestion(company, in)
	if stale == nil || in.dormancy.reading.Rung != dormancy.Stale || stale.Fingerprint != dormant.Fingerprint {
		t.Errorf("the stale reading of the same stretch fingerprints as %v, want %q", stale, dormant.Fingerprint)
	}

	later := occurredday.Moment{At: in.dormancy.lastActivity.At.AddDate(0, 0, 5)}
	in.dormancy.lastActivity = &later
	if again := goneDormantSuggestion(company, in); again == nil || again.Fingerprint == dormant.Fingerprint {
		t.Errorf("a new silent stretch kept the old fingerprint, so a dismissal would hide it")
	}
}

func TestTheFollowUpNeedsAColdRungAnOwnerAndTheSwitch(t *testing.T) {
	t.Parallel()
	company := ids.From[ids.CompanyKind](ids.NewV7())
	cases := map[string]func(*suggestionInputs){
		"no owner":                   func(in *suggestionInputs) { in.dormancy.owner = nil },
		"switched off":               func(in *suggestionInputs) { in.dormancy.followUp = false },
		"owner already holds a task": func(in *suggestionInputs) { in.dormancy.ownerHasTask = true },
		"cooling":                    func(in *suggestionInputs) { in.dormancy.reading.Rung = dormancy.Cooling },
		"active":                     func(in *suggestionInputs) { in.dormancy.reading.Rung = dormancy.Active },
		"no activity ever": func(in *suggestionInputs) {
			in.dormancy.lastActivity = nil
			in.dormancy.reading = dormancy.Default.Read(nil, dormancyNow, time.UTC)
		},
	}
	for name, change := range cases {
		in := coldAccount(ids.NewV7())
		change(&in)
		if got := goneDormantSuggestion(company, in); got != nil {
			t.Errorf("%s: raised %+v, want nothing", name, got)
		}
	}
}

// The suggestion runs through the card's own rule list, so the page and the
// dismissal endpoint see it alike.
func TestTheCardCarriesTheFollowUp(t *testing.T) {
	t.Parallel()
	company := ids.From[ids.CompanyKind](ids.NewV7())
	found := candidateSuggestions(company, dormancyNow, coldAccount(ids.NewV7()))
	if !slices.ContainsFunc(found, func(s crmcontracts.Company360Suggestion) bool { return s.Kind == suggestGoneDormant }) {
		t.Errorf("the card's rules raised %+v, want a gone_dormant row", found)
	}
}

func TestTheStripStatesTheRuleBesideTheRung(t *testing.T) {
	t.Parallel()
	got := coldAccount(ids.NewV7()).dormancy.wire()
	if got.Rung != "dormant" || got.SilentDays != 70 || got.NoActivity || !slices.Equal(got.LadderDays, []int{30, 60, 90}) {
		t.Errorf("the strip reads %+v, want dormant on day 70 under 30, 60 and 90", got)
	}
}
