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
// The criteria this file covers are the deterministic ones: every person is
// named (4), every event carries its date (3), and a cold account reports a
// NUMBER rather than "a while ago" (7). Whether a model writes a good briefing
// from that material is the weekly model lane's question.
//
// NOT covered here, and named so the gap is not read as coverage:
//
//   - Criterion 1, a briefing found without naming a record. That is the
//     assistant resolving "Vietnam partner" from a calendar, which is its half.
//   - Criterion 5, the unkept promise. It needs a model to notice that
//     something promised was never sent.
//   - Criterion 9, a caller who may not read a person being refused rather than
//     shown a blank. AppEnv mints its passport for whoever holds the cookie and
//     exposes no role control, so the restricted principal cannot be built over
//     MCP today. networktools_integration_test.go covers it in process.
//
// The fixtures write rows directly rather than through log_activity, so what is
// asserted is the READ side. Two consequences worth stating: the deal link and
// the participant row are written by hand here because the real logging path
// writes them, and a coverage rule that reads deal.last_activity_at is silent
// without the first — which is how three of these tests came to pass against
// nothing before Codex pointed at them.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/network"
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
	// exchanges are the activity ids this fixture logged. A test asserting
	// about events matches on these rather than on a summary: the retriever
	// renders an email's summary as "email", so prose matching would quietly
	// match nothing and assert against an empty set.
	exchanges map[ids.UUID]bool
}

const (
	engagedPersonName = "Mai Nguyen"
	quietPersonName   = "Tobias Kern"
	// coldDays is how long the cold fixture's account has been silent. Well
	// past the 30-day threshold, so the finding is not sitting on a boundary
	// that a clock skew could tip either way.
	coldDays = 83
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
	f.exchanges = map[ids.UUID]bool{
		s.seedDatedActivity(t, "email", "inbound", f.engaged, f.org, f.deal, daysAgo(20),
			"Cảm ơn — we will review the appendix this week."): true,
		s.seedDatedActivity(t, "email", "outbound", f.engaged, f.org, f.deal, daysAgo(18),
			"Ich schicke die Aufstellung mit."): true,
	}

	return f
}

// seedColdAccount is an open deal nobody has touched in a long time.
//
// Separate from seedMeetingAccount rather than a flag on it: the going-cold
// finding is about the ABSENCE of recent traffic, and a fixture that seeded
// both a recent exchange and a cold account would be asserting two contradictory
// things about one timeline.
func (s *scenario) seedColdAccount(t *testing.T) meetingFixture {
	t.Helper()
	var f meetingFixture
	pipeline, stage := s.defaultOpenStage(t)

	f.org = s.seedID(t, `INSERT INTO organization (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Quiet Partner GmbH', 'manual', 'human:x')`, s.Colleague)
	f.deal = s.seedID(t, `INSERT INTO deal
		(id, owner_id, organization_id, pipeline_id, stage_id, name, status, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'Renewal', 'open', 'manual', 'human:x')`,
		s.Colleague, f.org, pipeline, stage)

	f.engaged = s.seedPerson(t, engagedPersonName, f.org)
	s.seed(t, `INSERT INTO relationship (id, kind, deal_id, person_id, role, source, captured_by)
		VALUES ($1, 'deal_stakeholder', $2, $3, 'champion', 'manual', 'human:x')`,
		ids.NewV7(), f.deal, f.engaged)

	// The one exchange there has ever been, long enough ago to be cold.
	f.exchanges = map[ids.UUID]bool{
		s.seedDatedActivity(t, "email", "inbound", f.engaged, f.org, f.deal, daysAgo(coldDays),
			"We will come back to you after the budget round."): true,
	}
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
	t *testing.T, kind, direction string, person, org, deal ids.UUID, occurredAt time.Time, body string,
) ids.UUID {
	t.Helper()
	id := s.seedID(t, `INSERT INTO activity
		(id, kind, direction, occurred_at, body, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'manual', 'human:x')`, kind, direction, occurredAt, body)

	// One link row per target, because activity_link_shape allows exactly one
	// id per row: a row naming both a person and an organization is refused.
	//
	// THE DEAL LINK IS LOAD-BEARING. deal.last_activity_at is maintained by a
	// trigger on activity_link, and the coverage rules read that column to
	// decide whether the deal has ever been touched. An exchange linked only
	// to the person and the company leaves the deal looking untouched, and
	// every risk that gates on EverTouched stays silent — which is what this
	// fixture did before Codex pointed at it.
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), id, person)
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, organization_id)
		VALUES ($1, $2, 'organization', $3)`, ids.NewV7(), id, org)
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), id, deal)

	// The participant row every real logging path writes for a person-linked
	// interaction. Relationship strength and the person graph are projected
	// from these, so an exchange without one is a conversation the product
	// cannot see two people having.
	s.seed(t, `INSERT INTO activity_participant (id, activity_id, person_id, role)
		VALUES ($1, $2, $3, $4)`, ids.NewV7(), id, person, participantRoleFor(direction))
	return id
}

// participantRoleFor names which end of the exchange the contact was on.
func participantRoleFor(direction string) string {
	if direction == "inbound" {
		return "from"
	}
	return "to"
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

	// The answer must be ABOUT the deal that was asked for. The fixture holds
	// one deal, so a tool ignoring deal_id entirely would otherwise pass every
	// other assertion in this file.
	if answer.DealID != f.deal {
		t.Fatalf("case 5: asked about deal %s, answered about %s", f.deal, answer.DealID)
	}
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
//
// The fixture is built so a finding MUST exist: one engaged contact on an open
// deal is single_threaded_theirs, and it carries the person it is about. The
// test demands that finding by name rather than looping over whatever came
// back — a loop over an empty list asserts nothing, and risk generation could
// be deleted without this test noticing.
func TestCase5AFindingNamesThePeopleItIsAbout(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedMeetingAccount(t)

	got := s.MCP.CallOK(t, "account_coverage", map[string]any{"deal_id": f.deal.String()})
	var answer agents.DealCoverageAnswer
	got.JSON(t, &answer)

	risk, ok := riskOfKind(answer, network.RiskSingleThreadedTheirs)
	if !ok {
		t.Fatalf("case 5 criterion 4: the deal has exactly one engaged contact, so %q must be "+
			"reported; findings were %v", network.RiskSingleThreadedTheirs, kindsOf(answer.Risks))
	}
	if len(risk.People) == 0 {
		t.Fatalf("case 5 criterion 4: the %q finding names nobody, so a rep is told the deal rests "+
			"on one relationship without being told whose", risk.Kind)
	}

	// The pairing is the assertion, not the two lengths. Names travel INSIDE
	// one object with their ids precisely so a consumer cannot attach the
	// wrong name to the wrong person — checking only that the lists are the
	// same length would pass on exactly the misattribution the shape prevents.
	for _, person := range risk.People {
		if person.Name == "" {
			t.Fatalf("case 5 criterion 4: the %q finding names person %s with no name",
				risk.Kind, person.PersonID)
		}
		if person.PersonID != f.engaged {
			t.Fatalf("case 5 criterion 4: the %q finding is about %s, but the only engaged contact "+
				"on this deal is %s", risk.Kind, person.PersonID, f.engaged)
		}
		if person.Name != engagedPersonName {
			t.Fatalf("case 5 criterion 4: person %s is named %q in the finding and %q in the CRM — "+
				"a rep repeating that sentence names the wrong human",
				person.PersonID, person.Name, engagedPersonName)
		}
	}
}

// TestCase5AColdAccountReportsHowManyDays pins criterion 7.
//
// "83 days since contact" and "some time ago" are different answers, and only
// the first tells a rep whether to worry. The fixture's last touch is old
// enough to be cold and recent enough that the number is checkable.
func TestCase5AColdAccountReportsHowManyDays(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedColdAccount(t)

	got := s.MCP.CallOK(t, "account_coverage", map[string]any{"deal_id": f.deal.String()})
	var answer agents.DealCoverageAnswer
	got.JSON(t, &answer)

	risk, ok := riskOfKind(answer, network.RiskGoingCold)
	if !ok {
		t.Fatalf("case 5 criterion 7: nothing has touched this deal in %d days and no %q finding "+
			"was reported; findings were %v", coldDays, network.RiskGoingCold, kindsOf(answer.Risks))
	}
	if risk.DaysSinceTouch == nil {
		t.Fatalf("case 5 criterion 7: the account is reported cold with no number, so a rep cannot " +
			"tell a fortnight from half a year")
	}
	// A day either side, because the fixture stamps a timestamp and the
	// product counts whole days from it.
	if *risk.DaysSinceTouch < coldDays-1 || *risk.DaysSinceTouch > coldDays+1 {
		t.Fatalf("case 5 criterion 7: last touch was %d days ago, reported as %d",
			coldDays, *risk.DaysSinceTouch)
	}
}

// TestCase5EveryEventCarriesTheDateItHappened pins criterion 3, the half of
// #2059 that lives on the server.
//
// A briefing that dates an event from somebody's prose instead of from the
// record is the defect that issue was merged for: a loss post-mortem written
// months later said "im Oktober" about an email dated in September, and a
// briefing holding only the prose repeats that to the customer. Whether a model
// PREFERS the record when the two disagree is the weekly lane's question; this
// pins that the date is there to prefer.
func TestCase5EveryEventCarriesTheDateItHappened(t *testing.T) {
	s := boot(t, scopesRead)
	f := s.seedMeetingAccount(t)

	got := s.MCP.CallOK(t, "prep_for_meeting", map[string]any{
		"record_type": "deal",
		"record_id":   f.deal.String(),
	})
	var answer agents.PrepForMeetingResult
	got.JSON(t, &answer)

	items := briefingItems(answer.Briefing)
	if len(items) == 0 {
		t.Fatalf("case 5 criterion 3: the account has two logged exchanges and the briefing carries "+
			"no items across %d sections, so there is nothing for a date to be on (deal %s)",
			len(answer.Briefing.Sections), f.deal)
	}
	// An item that is not an event legitimately carries no date, so the
	// assertion is about the ones that are. They are identified by the ACTIVITY
	// ids the fixture wrote rather than by their summaries: the retriever
	// summarises an email as "email", so matching on prose would silently match
	// nothing and the loop would assert against an empty set.
	dated := 0
	for _, item := range items {
		if !f.exchanges[item.RecordID] {
			continue
		}
		if item.OccurredAt == nil {
			t.Fatalf("case 5 criterion 3: the item summarising %q carries no occurred_at, so a "+
				"briefing has nothing to date it by except the prose in its body", item.Summary)
		}
		if item.OccurredAt.IsZero() {
			t.Fatalf("case 5 criterion 3: %q is dated to the zero time", item.Summary)
		}
		dated++
	}
	if dated == 0 {
		t.Fatalf("case 5 criterion 3: neither seeded exchange reached the briefing, so the date "+
			"assertion ran against nothing; items were %v", summariesOf(items))
	}
}

// briefingItems flattens the briefing's sections. The section NAMES are the
// retriever's own and not a closed set, so a test that named one would break
// when the retriever renamed it — the assertion is about the dates, not the
// grouping.
func briefingItems(briefing agents.AssembledContextResult) []agents.ContextItem {
	var out []agents.ContextItem
	for _, section := range briefing.Sections {
		out = append(out, section.Items...)
	}
	return out
}

// summariesOf names what the briefing DID carry, for a failure worth reading.
func summariesOf(items []agents.ContextItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Summary)
	}
	return out
}

// riskOfKind finds one finding by kind.
func riskOfKind(answer agents.DealCoverageAnswer, kind string) (agents.CoverageRisk, bool) {
	for _, risk := range answer.Risks {
		if risk.Kind == kind {
			return risk, true
		}
	}
	return agents.CoverageRisk{}, false
}

// kindsOf names what WAS reported, for a failure message that says so.
func kindsOf(risks []agents.CoverageRisk) []string {
	out := make([]string, 0, len(risks))
	for _, risk := range risks {
		out = append(out, risk.Kind)
	}
	return out
}

// keysOf names what a map held, for a failure message that says what WAS there.
func keysOf(m map[string]agents.CoverageSeat) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
