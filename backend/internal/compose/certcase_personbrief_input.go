// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The fixture summarize/person_brief is prepared from, and the refusals that
// stop a scenario measuring nothing.
//
// Ids are MINTED here and never written in the corpus. The model is asked to
// return ids it was handed, so a corpus-supplied id would be one whoever wrote
// the expected answer could copy in, and a model echoing it would be
// indistinguishable from one that read the right conversation.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/personbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// personBriefFixture is one relationship as the brief reads it.
type personBriefFixture struct {
	Name            string                `json:"name"`
	Title           string                `json:"title"`
	Employer        string                `json:"employer"`
	BuyingRole      string                `json:"buying_role"`
	Strength        int                   `json:"strength"`
	SectionsOmitted []string              `json:"sections_omitted"`
	Deal            *personBriefDeal      `json:"deal"`
	Changes         []personBriefChange   `json:"changes"`
	Messages        []personBriefMessage  `json:"messages"`
	Claims          []personBriefClaimRow `json:"claims"`
}

// personBriefDeal is the commercial stake this contact sits on.
type personBriefDeal struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	CloseDate   string `json:"close_date"`
}

// personBriefChange is one thing that moved about the relationship.
type personBriefChange struct {
	Kind string `json:"kind"`
	Days int    `json:"days"`
}

// personBriefMessage is one captured message, labelled so the expectation can
// name it without naming an id.
//
// Withheld is the case a brief gets wrong in the most damaging direction: the
// row's date is the reader's and its words are not, so the fold carries the row
// with no subject and no preview — exactly as the 360 hands it over.
type personBriefMessage struct {
	Label     string `json:"label"`
	DaysAgo   int    `json:"days_ago"`
	Direction string `json:"direction"`
	Subject   string `json:"subject"`
	Preview   string `json:"preview"`
	Move      string `json:"move"`
	Withheld  bool   `json:"withheld"`
}

// personBriefClaimRow is one thing this contact said, as the extractor read it.
type personBriefClaimRow struct {
	Kind      string `json:"kind"`
	Body      string `json:"body"`
	Quote     string `json:"quote"`
	Status    string `json:"status"`
	DueAt     string `json:"due_at"`
	FromLabel string `json:"from_label"`
}

// personBriefExpectation is what a right brief looks like.
type personBriefExpectation struct {
	// CitesLabel is the message the brief must have read. Named by label,
	// resolved to a minted id at Prepare time.
	CitesLabel string `json:"cites_label"`
	// NamesToken is a phrase only this relationship produces. A brief whose
	// prose never contains it would read the same about any contact, whatever
	// else it got right.
	NamesToken string `json:"names_token"`
	// Avoids are phrases a right brief never writes — the deterministic half of
	// a silence expectation, which a rubric alone can only ask a judge about.
	Avoids []string `json:"avoids"`
}

// The activity kind and direction a fixture message folds to, DERIVED from the
// contract's own enums rather than re-spelled — the same rule the site itself
// follows, and the reason a rename upstream fails to compile here instead of
// leaving a fixture describing a shape the product no longer has.
const (
	fixtureKindEmail = string(crmcontracts.ActivityKindEmail)
	fixtureInbound   = string(crmcontracts.ActivityDirectionInbound)
)

// personBriefInput assembles what the service assembles, with minted ids.
func personBriefInput(f personBriefFixture) (personbrief.Input, map[string]string) {
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	in := personbrief.Input{
		Name: f.Name, Title: f.Title, Employer: f.Employer,
		BuyingRole: f.BuyingRole, Strength: f.Strength,
		SectionsOmitted: f.SectionsOmitted,
	}
	if f.Deal != nil {
		in.OpenDeal = &personbrief.DealIn{
			ID: ids.NewV7().String(), Name: f.Deal.Name, Stage: f.Deal.Stage,
			AmountMinor: f.Deal.AmountMinor, Currency: f.Deal.Currency, CloseDate: f.Deal.CloseDate,
		}
	}
	for _, change := range f.Changes {
		in.Changes = append(in.Changes, personbrief.ChangeIn{
			Kind: change.Kind, At: now.AddDate(0, 0, -change.Days).UTC().Format(time.RFC3339),
			Days: change.Days,
		})
	}
	byLabel := foldFixtureMessages(&in, f, now)
	for _, claim := range f.Claims {
		in.Claims = append(in.Claims, personbrief.ClaimIn{
			ID: ids.NewV7().String(), Kind: claim.Kind, Body: claim.Body,
			Status: claim.Status, DueAt: claim.DueAt, Quote: claim.Quote,
			SourceID: byLabel[claim.FromLabel],
		})
	}
	return in, byLabel
}

// foldFixtureMessages mints one id per labelled message and folds the timeline
// the way the 360 hands it over — newest first, and a withheld row carrying its
// date alone.
//
// The ORDER is imposed here rather than asked of the corpus author. Production
// reads `view.Activities.Data`, which arrives newest-first, and the floor takes
// `Recent[0]` as the newest message; a scenario written the way people write
// conversations — oldest first — would otherwise hand the floor its oldest
// message as the newest, quote the wrong one, and shift what
// refuseUnpreparableBrief believes the floor already says. Nothing about the
// fixture would look wrong, which is the kind of trap a corpus must not carry.
func foldFixtureMessages(
	in *personbrief.Input, f personBriefFixture, now time.Time,
) map[string]string {
	byLabel := map[string]string{}
	// A copy, stable: the fixture's own order is the author's and is left
	// alone, and two messages the same age keep the order they were written in
	// rather than depending on the sort.
	ordered := slices.Clone(f.Messages)
	slices.SortStableFunc(ordered, func(a, b personBriefMessage) int {
		return a.DaysAgo - b.DaysAgo
	})
	for _, message := range ordered {
		id := ids.NewV7().String()
		byLabel[message.Label] = id
		at := now.AddDate(0, 0, -message.DaysAgo)
		folded := personbrief.ActIn{
			ID: id, Kind: fixtureKindEmail, Direction: message.Direction,
			At: at.UTC().Format(time.RFC3339), Withheld: message.Withheld,
		}
		if !message.Withheld {
			folded.Subject, folded.Preview, folded.Move = message.Subject, message.Preview, message.Move
		}
		in.Recent = append(in.Recent, folded)
		if message.Direction == fixtureInbound {
			in.LastInbound = maxStamp(in.LastInbound, folded.At)
			continue
		}
		in.LastOutbound = maxStamp(in.LastOutbound, folded.At)
	}
	return byLabel
}

// maxStamp keeps the later of two RFC3339 instants. They compare as strings the
// way the instants compare, which is the property the fold relies on everywhere
// else it holds a stamp.
func maxStamp(held, candidate string) string {
	if candidate > held {
		return candidate
	}
	return held
}

// refuseUnpreparableBrief names a fixture or expectation that would measure
// nothing, at parse time rather than after a paid run.
func refuseUnpreparableBrief(f personBriefFixture, want personBriefExpectation) error {
	if len(f.Messages) < 2 {
		return fmt.Errorf(
			"summarize/person_brief: the fixture supplies %d message(s); with fewer than two there is no wrong one to cite",
			len(f.Messages))
	}
	if strings.TrimSpace(want.NamesToken) == "" {
		return errors.New(
			"summarize/person_brief: the expectation names no relationship-specific token, so generic prose would satisfy it")
	}
	if err := refuseUnnameableMessages(f, want); err != nil {
		return err
	}
	token := strings.ToLower(want.NamesToken)
	if !strings.Contains(strings.ToLower(fixtureWords(f)), token) {
		return fmt.Errorf(
			"summarize/person_brief: the expectation's token %q appears in nothing the summary carries, so only an invented brief could name it",
			want.NamesToken)
	}
	// And the token must be something the FLOOR does not already say. The
	// deterministic brief quotes previews and claim bodies, so a token drawn
	// from one would be in the prose whatever the model returned — the scenario
	// would pass forever without the model contributing anything, which is the
	// one way a certification case fails silently.
	//
	// Folded, like the check Evaluate runs: a token the floor prints in another
	// case is one the floor prints, and a refusal that missed it would let a
	// capitalisation difference hide exactly the silent pass it exists to catch.
	in, _ := personBriefInput(f)
	floor := personbrief.Prose(personbrief.Deterministic(ids.NewV7().String(), in))
	if strings.Contains(strings.ToLower(floor), token) {
		return fmt.Errorf(
			"summarize/person_brief: the token %q is already in the deterministic floor's own prose, so a reply saying nothing would satisfy this scenario",
			want.NamesToken)
	}
	for _, avoided := range want.Avoids {
		if strings.TrimSpace(avoided) == "" {
			return errors.New("summarize/person_brief: the expectation forbids a blank phrase, which every reply contains")
		}
	}
	return nil
}

// refuseUnnameableMessages rejects labels no expectation could refer to: a
// blank one names nothing and a repeated one names two messages, so an
// expectation using it means neither.
func refuseUnnameableMessages(f personBriefFixture, want personBriefExpectation) error {
	seen := map[string]bool{}
	for i, message := range f.Messages {
		if strings.TrimSpace(message.Label) == "" {
			return fmt.Errorf("summarize/person_brief: the message at position %d carries no label", i+1)
		}
		if seen[message.Label] {
			return fmt.Errorf(
				"summarize/person_brief: two messages are labelled %q, so an expectation naming it means neither",
				message.Label)
		}
		seen[message.Label] = true
	}
	if !seen[want.CitesLabel] {
		return fmt.Errorf(
			"summarize/person_brief: the expectation names %q, which the fixture does not carry — no reply could satisfy it",
			want.CitesLabel)
	}
	return nil
}

// fixtureWords is everything a right brief could have read the token from. A
// withheld message contributes nothing: its words are not the reader's, so a
// token drawn from one could only reach the prose by invention.
func fixtureWords(f personBriefFixture) string {
	var all strings.Builder
	for _, message := range f.Messages {
		if message.Withheld {
			continue
		}
		all.WriteString(message.Subject + " " + message.Preview + " ")
	}
	for _, claim := range f.Claims {
		all.WriteString(claim.Body + " " + claim.Quote + " ")
	}
	if f.Deal != nil {
		all.WriteString(f.Deal.Name + " " + f.Deal.Stage + " ")
	}
	all.WriteString(f.Name + " " + f.Title + " " + f.Employer)
	return all.String()
}
