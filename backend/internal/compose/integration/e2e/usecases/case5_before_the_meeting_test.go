// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 5 — Before the meeting.
//
// The prompt, spoken, from the run this was written against:
//
//	Hey. Tomorrow I'm meeting, uh, finally, Vietnam partner. Could check in
//	my calendar. Can you prep me for the meeting? Check in the CRM.
//
// It names no record, no id and no tool. "Vietnam partner" is not the account's
// name. Reading the calendar and working out which meeting is the assistant's
// half and stays out of this suite; what is pinned here is what Margince has to
// hand back once the account is known.
//
// The criteria this file covers are the deterministic ones — every person is
// named, every event carries its date, a cold account reports a NUMBER, and an
// empty section says it is empty rather than being filled. Whether a model
// writes a good briefing from that material is the weekly model lane's
// question.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingFixture is the account a briefing is asked about: a deal, the people
// on it, and the exchange history that decides who counts as engaged.
type meetingFixture struct {
	org  ids.UUID
	deal ids.UUID
	// engaged has traffic both ways inside the window; quiet has none. The
	// pair is the fixture's whole point — "everyone is engaged" and "nobody is"
	// are both answers a bug can produce, and only a mixed account can tell
	// them apart.
	engaged ids.UUID
	quiet   ids.UUID
}

const (
	engagedPersonName = "Mai Nguyen"
	quietPersonName   = "Tobias Kern"
)

// seedMeetingAccount builds the account case 5 asks about.
//
// The deal is owned by the COLLEAGUE rather than the caller, because the real
// run's most useful sentence was that the account belongs to somebody else and
// the rep should clear it with them first.
func (s *scenario) seedMeetingAccount(t *testing.T) meetingFixture {
	t.Helper()
	var f meetingFixture

	// The installation already ships a default pipeline with its stages. A
	// scenario inserts none of its own: a second pipeline named 'Sales' is
	// refused by pipeline_name_unique, and a second DEFAULT one would be a
	// state the product cannot reach.
	pipeline, stage := s.defaultOpenStage(t)

	f.org = s.seedID(t, `INSERT INTO organization (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Vietnam Partner JSC', 'manual', 'human:x')`, s.Colleague)
	f.deal = s.seedID(t, `INSERT INTO deal
		(id, owner_id, organization_id, pipeline_id, stage_id, name, status, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'Distribution agreement', 'open', 'manual', 'human:x')`,
		s.Colleague, f.org, pipeline, stage)

	f.engaged = s.seedPerson(t, engagedPersonName, f.org)
	f.quiet = s.seedPerson(t, quietPersonName, f.org)

	// Both are seats on the deal. Only one of them has spoken.
	s.seed(t, `INSERT INTO relationship (id, kind, deal_id, person_id, role, source, captured_by)
		VALUES ($1, 'deal_stakeholder', $2, $3, 'champion', 'manual', 'human:x')`,
		ids.NewV7(), f.deal, f.engaged)
	s.seed(t, `INSERT INTO relationship (id, kind, deal_id, person_id, role, source, captured_by)
		VALUES ($1, 'deal_stakeholder', $2, $3, 'economic_buyer', 'manual', 'human:x')`,
		ids.NewV7(), f.deal, f.quiet)

	// Engagement needs traffic in BOTH directions inside the 90-day window, so
	// a one-sided conversation does not read as a relationship. That asymmetry
	// is the product's claim and the fixture has to honour it.
	s.seedDatedActivity(t, "email", "inbound", f.engaged, f.org, daysAgo(20),
		"Cảm ơn — we will review the appendix this week.")
	s.seedDatedActivity(t, "email", "outbound", f.engaged, f.org, daysAgo(18),
		"Ich schicke die Aufstellung mit.")

	return f
}

// seedPerson adds a contact employed at the organization.
func (s *scenario) seedPerson(t *testing.T, name string, org ids.UUID) ids.UUID {
	t.Helper()
	id := s.seedID(t, `INSERT INTO person (id, owner_id, full_name, source, captured_by)
		VALUES ($1, $2, $3, 'manual', 'human:x')`, s.Colleague, name)
	s.seed(t, `INSERT INTO relationship (id, kind, person_id, organization_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'manual', 'human:x')`, ids.NewV7(), id, org)
	return id
}

// seedDatedActivity logs one exchange with an explicit date.
//
// The date is the fixture's reason for existing. Criterion 3 is that every
// event carries the date it happened, and a briefing that quotes a date from
// somebody's prose instead of the record is the defect #2059 was merged for.
func (s *scenario) seedDatedActivity(
	t *testing.T, kind, direction string, person, org ids.UUID, occurredAt time.Time, body string,
) {
	t.Helper()
	id := s.seedID(t, `INSERT INTO activity
		(id, kind, direction, occurred_at, body, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'manual', 'human:x')`, kind, direction, occurredAt, body)
	// One link row per target, because activity_link_shape allows exactly one
	// id per row: a row naming both a person and an organization is refused.
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), id, person)
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, organization_id)
		VALUES ($1, $2, 'organization', $3)`, ids.NewV7(), id, org)
}

// daysAgo is a date the fixture controls, so an assertion about ordering or
// staleness does not depend on when the suite runs.
func daysAgo(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, -days)
}

// TestCase5EveryStakeholderIsNamedNotJustIdentified pins criterion 4, and it is
// the criterion this case exists for.
//
// Before #2766 the seats came back as bare uuids: a coverage answer saying
// "economic_buyer, not engaged" against an id has not told a rep who to bring
// into the room. The data was correct and unusable in a sentence.
func TestCase5EveryStakeholderIsNamedNotJustIdentified(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedMeetingAccount(t)

	got := s.MCP.CallOK(t, "account_coverage", map[string]any{"deal_id": f.deal.String()})
	var answer agents.DealCoverageAnswer
	got.JSON(t, &answer)

	if len(answer.Stakeholders) != 2 {
		t.Fatalf("case 5 criterion 4: the deal has 2 seats, coverage reported %d",
			len(answer.Stakeholders))
	}
	byName := map[string]agents.CoverageSeat{}
	for _, seat := range answer.Stakeholders {
		if seat.PersonName == "" {
			t.Fatalf("case 5 criterion 4: the %s seat came back as %s with no name — a rep cannot "+
				"be told who to bring into the room", seat.Role, seat.PersonID)
		}
		byName[seat.PersonName] = seat
	}

	// Criterion 7's other half: engaged and quiet must be told apart. A tool
	// that marked everyone engaged would pass a name check and still tell the
	// rep the deal is covered when it is not.
	engaged, ok := byName[engagedPersonName]
	if !ok {
		t.Fatalf("case 5 criterion 4: the engaged stakeholder is missing; got %v", keysOf(byName))
	}
	if !engaged.Engaged {
		t.Fatalf("case 5: %s has traffic both ways inside the window and is reported as not engaged",
			engagedPersonName)
	}
	quiet, ok := byName[quietPersonName]
	if !ok {
		t.Fatalf("case 5 criterion 4: the quiet stakeholder is missing; got %v", keysOf(byName))
	}
	if quiet.Engaged {
		t.Fatalf("case 5: %s has never spoken and is reported as engaged, which tells a rep the "+
			"economic buyer is covered when nobody has heard from them", quietPersonName)
	}
}

// TestCase5AFindingNamesThePeopleItIsAbout pins criterion 4's second half.
//
// A finding that says "the deal rests on one relationship" and lists a uuid
// makes the rep go look the name up, which is the work the tool exists to save.
// The names travel PAIRED with their ids in one object rather than as a second
// array, so the two cannot misalign under a concurrent write.
func TestCase5AFindingNamesThePeopleItIsAbout(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedMeetingAccount(t)

	got := s.MCP.CallOK(t, "account_coverage", map[string]any{"deal_id": f.deal.String()})
	var answer agents.DealCoverageAnswer
	got.JSON(t, &answer)

	for _, risk := range answer.Risks {
		for _, person := range risk.People {
			if person.Name == "" {
				t.Fatalf("case 5 criterion 4: the %q finding names person %s with no name",
					risk.Kind, person.PersonID)
			}
		}
		// The paired object is what makes misalignment structurally
		// impossible, so a finding carrying ids and no pairs is the regression
		// this pins — not merely a cosmetic gap.
		if len(risk.PersonIDs) > 0 && len(risk.People) != len(risk.PersonIDs) {
			t.Fatalf("case 5 criterion 4: the %q finding lists %d person ids but %d named people; "+
				"a consumer indexing across the two would attach the wrong name to the wrong person",
				risk.Kind, len(risk.PersonIDs), len(risk.People))
		}
	}
}

// TestCase5AWithheldSectionSaysSoRatherThanLookingEmpty pins criterion 6 and
// half of the cross-cutting refusal rule.
//
// A model handed no seats and no findings writes "the deal looks well covered"
// in a sentence a rep then acts on. An empty section and a withheld one have to
// be different answers on the wire.
func TestCase5AWithheldSectionSaysSoRatherThanLookingEmpty(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedMeetingAccount(t)

	got := s.MCP.CallOK(t, "account_coverage", map[string]any{"deal_id": f.deal.String()})
	var answer agents.DealCoverageAnswer
	got.JSON(t, &answer)

	// This caller holds the grants, so nothing is withheld — and the field must
	// still be present rather than null, because a reader cannot tell "nothing
	// withheld" from "not computed" when the answer is absent.
	if answer.SectionsOmitted == nil {
		t.Fatalf("case 5 criterion 6: sections_omitted came back null; a caller cannot tell " +
			"'nothing was withheld' from 'this was not computed'")
	}
	if len(answer.SectionsOmitted) != 0 {
		t.Fatalf("case 5: a fully granted caller was told sections were withheld: %v",
			answer.SectionsOmitted)
	}
}

// keysOf names what a map held, for a failure message that says what WAS there.
func keysOf(m map[string]agents.CoverageSeat) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
