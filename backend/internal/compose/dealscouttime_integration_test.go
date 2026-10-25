// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func TestScoutUsesTheSourceDateInsteadOfDetection(t *testing.T) {
	for _, age := range []int{2, 120} {
		t.Run(time.Duration(age).String(), func(t *testing.T) {
			e := setupScout(t)
			e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
			company := e.SeedCompany(t, "Maple Studio", nil)
			source, buyer := e.buyingEmail(t, company, e.daysAgo(age))
			e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
			e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
			want := 1
			if age > 90 {
				want = 0
			}
			if pass := e.pass(t); pass.Raised != want {
				t.Fatalf("source age %d: raised %d, want %d", age, pass.Raised, want)
			}
			for _, suggestion := range e.suggestions(e.signalReader(), t, company) {
				for _, evidence := range suggestion.Evidence {
					if !evidence.OccurredAt.Equal(e.daysAgo(age)) {
						t.Fatalf("evidence date = %v, want source date %v", evidence.OccurredAt, e.daysAgo(age))
					}
				}
			}
		})
	}
}

func TestSuggestionWriterRefusesDetectionTimeAsEvidenceTime(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source := e.email(t, "An old project", "inbound", company, e.daysAgo(120))
	signal := e.signal(t, "new_opportunity", company, source, e.now, ids.Nil)
	raised, err := e.record(e.system(), t, deals.SuggestionDraft{
		CompanyID: company, NameHint: deals.HintOpportunitySignaled, Confidence: .7,
		Evidence: []deals.SuggestionEvidence{{Kind: deals.EvidenceSignal, SignalID: &signal, OccurredAt: e.now}},
	})
	if err != nil || raised {
		t.Fatalf("record with processing date = %v, %v; want refusal without write", raised, err)
	}
}

// dayMeeting logs a held meeting known only by its date, with the contact.
func (e *scoutEnv) dayMeeting(t *testing.T, contact ids.UUID, date time.Time) deals.SuggestionEvidence {
	t.Helper()
	held, subject, anchor := "held", "Scoping day", occurredday.Anchor(date)
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &anchor, OccurredPrecision: activities.PrecisionDay,
		MeetingStatus: &held, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the day meeting: %v", err)
	}
	id := ids.UUID(logged.Id)
	return deals.SuggestionEvidence{Kind: deals.EvidenceMeeting, ActivityID: &id, OccurredAt: anchor}
}

// A day meeting falls past a dismissal by its date, as the scout's SQL places
// it. One dated the dismissal's own date began before it, so it is refused.
func TestTheWriterPlacesADayMeetingByItsDateAgainstTheFloor(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.buyerPair(t, acme, e.daysAgo(6))
	e.pass(t)
	dismissed := e.onlySuggestion(e.signalReader(), t, acme).ID
	if _, err := e.decider().DismissSuggestion(e.signalReader(), dismissed); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	date := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE deal_suggestion SET decided_at = $2 WHERE id = $1`, dismissed, date.Add(10*time.Hour)); err != nil {
		t.Fatal(err)
	}
	draft := func(evidence deals.SuggestionEvidence) deals.SuggestionDraft {
		return deals.SuggestionDraft{
			CompanyID: acme, NameHint: deals.HintMeetingHeld, Confidence: 0.6,
			Evidence: []deals.SuggestionEvidence{evidence},
		}
	}

	if raised, err := e.record(e.system(), t, draft(e.dayMeeting(t, dana, date))); err != nil || raised {
		t.Fatalf("a day meeting dated the dismissal's date = %v, %v; want nothing written", raised, err)
	}
	if raised, err := e.record(e.system(), t, draft(e.dayMeeting(t, dana, date.AddDate(0, 0, 1)))); err != nil || !raised {
		t.Fatalf("a day meeting dated the day after = %v, %v; want it raised", raised, err)
	}
}

// A signal citing a day entry keeps its evidence when the installation zone
// changes. Its recorded stamp does not move, so accepting opens a deal.
func TestAZoneChangeLeavesASignalCitingADayStanding(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(2))
	e.WsExec(t, `UPDATE activity SET occurred_precision = 'day', occurred_at = $2 WHERE id = $1`,
		source, occurredday.Anchor(e.daysAgo(2)))
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
	if pass := e.pass(t); pass.Raised != 1 {
		t.Fatalf("raised %d suggestions, want 1", pass.Raised)
	}
	e.WsExec(t, `UPDATE setting SET value = '"Pacific/Kiritimati"'::jsonb WHERE key = 'installation.timezone'`)

	suggestion := e.onlySuggestion(e.signalReader(), t, company)
	for _, item := range suggestion.Evidence {
		if item.Precision == nil || *item.Precision != "day" || !item.OccurredAt.Equal(occurredday.Anchor(e.daysAgo(2))) {
			t.Errorf("%s evidence reads %v at %v, want the cited day's anchor as a day", item.Kind, item.Precision, item.OccurredAt)
		}
	}
	if _, err := e.decider().AcceptSuggestion(e.signalReader(), suggestion.ID, deals.AcceptSuggestionInput{}); err != nil {
		t.Fatalf("accepting after the zone moved = %v, want the deal opened", err)
	}
}
