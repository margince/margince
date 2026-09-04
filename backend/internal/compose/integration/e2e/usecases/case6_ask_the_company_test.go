// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 6 — Ask the company.
//
// The prompt, spoken, naming no company, no person, no date and no record:
//
//	We are about to change the account manager on one of our major
//	accounts, and in the past customers complained if we swapped the
//	responsible people too much. Did we have it in the past, what happened,
//	how did we react, what should I be prepared for? Is there something we
//	can learn? Check in our CRM.
//
// The fixture is built around ONE deliberate contradiction, because criterion 3
// cannot pass unless it can fail: an email dated in September, and a
// post-mortem note whose prose says the complaint was raised "im Oktober". Both
// reach the caller. The record is right and the prose is wrong, and a briefing
// holding only the prose repeats October to the customer.
//
// What this suite pins is that the assistant HAS the choice — every event
// carries its own date, and every matching record reaches the caller rather
// than only the top one. Whether a model then prefers the record over the prose
// is criterion 3's other half and belongs to the weekly lane.
//
// NOT covered here: criteria 1 and 5, which are both about a model — finding
// the right history from a description that names nothing, and drawing a
// conclusion the records support without stating.

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contradiction the fixture is built around.
const (
	// complaintDaysAgo puts the email in a month the note then misremembers.
	complaintDaysAgo = 340
	// theRecordSays is in the email body — what actually happened, when.
	theRecordSays = "Der ständige Wechsel der Ansprechpartner ist für uns ein echtes Problem."
	// thePostMortemSays is a note written months later. It is WRONG about the
	// month, exactly as a real post-mortem was.
	thePostMortemSays = "Der Kunde hat das im Oktober klar angesprochen; wir haben zu spät reagiert."
	// theWrongMonth is what the prose claims. The record says otherwise, and
	// that disagreement is the entire fixture.
	theWrongMonth = "Oktober"
	// sharedAccountWord is what the sweep matches all three accounts on.
	//
	// It sits in the DISPLAY NAME because that is the only organization text
	// the lexical lane reads (search/branches.go's searchBranches), and this
	// harness binds no embedding lane — so the answer degrades to lexical and
	// ranks on literal words. A query phrased as a complaint would rank
	// nothing, which is a fact about how this deployment is composed rather
	// than about the history.
	//
	// The point being tested is still the right one: THREE accounts match and
	// all three have to come back, because an answer carrying only the best
	// one lets an assistant call a pattern a one-off.
	sharedAccountWord = "Betreuerwechsel"
)

// pastCase is one account that lived through an account-manager change.
type pastCase struct {
	org        ids.UUID
	complaint  ids.UUID
	postMortem ids.UUID
}

// seedContradiction builds the account whose record and prose disagree.
func (s *scenario) seedContradiction(t *testing.T) pastCase {
	t.Helper()
	var c pastCase
	c.org = s.seedID(t, `INSERT INTO organization
		(id, owner_id, display_name, industry, source, captured_by)
		VALUES ($1, $2, $3, 'Managed Services', 'manual', 'human:x')`,
		s.Colleague, "Reply Deutschland "+sharedAccountWord)
	person := s.seedPerson(t, "Katrin Sommer", c.org)

	// The email, dated. This is the record.
	c.complaint = s.seedOrgActivity(t, "email", "inbound", person, c.org,
		daysAgo(complaintDaysAgo), theRecordSays)
	// The note, written later, wrong about the month. This is the prose.
	c.postMortem = s.seedOrgActivity(t, "note", "", person, c.org,
		daysAgo(complaintDaysAgo-90), thePostMortemSays)
	return c
}

// seedTwoMorePastCases gives the sweep more than one thing to find, so
// criterion 4 has something to be about.
func (s *scenario) seedTwoMorePastCases(t *testing.T) []pastCase {
	t.Helper()
	var out []pastCase
	for _, account := range []struct{ company, complaint string }{
		{"valantic AG", "Nach dem Wechsel des Ansprechpartners kam fünf Tage lang keine Antwort."},
		{"Körber Digital GmbH", "Der Tiefpunkt war nicht der Fehler selbst, sondern dass niemand sich meldete."},
	} {
		org := s.seedID(t, `INSERT INTO organization
			(id, owner_id, display_name, industry, source, captured_by)
			VALUES ($1, $2, $3, 'Managed Services', 'manual', 'human:x')`,
			s.Colleague, account.company+" "+sharedAccountWord)
		person := s.seedPerson(t, "Kontakt "+account.company, org)
		out = append(out, pastCase{
			org:       org,
			complaint: s.seedOrgActivity(t, "email", "inbound", person, org, daysAgo(200), account.complaint),
		})
	}
	return out
}

// seedOrgActivity logs one dated activity against a company and a person.
//
// No deal here, unlike case 5's helper: this case is about a company's history
// rather than about a deal's coverage, and a deal the scenario never asks about
// would be a row nothing reads.
//
// The provenance is SYNTHETIC and deliberately so. A real write derives
// captured_by from the authenticated principal and stamps an actor participant
// beside the linked people; this fixture writes neither, because what case 6
// asserts is retrieval and dates rather than provenance. Case 1 is where the
// write path itself is under test, and it inserts nothing by hand. Say it here
// so a later assertion about who captured a record is not built on this.
func (s *scenario) seedOrgActivity(
	t *testing.T, kind, direction string, person, org ids.UUID, occurredAt time.Time, body string,
) ids.UUID {
	t.Helper()
	var id ids.UUID
	if direction == "" {
		id = s.seedID(t, `INSERT INTO activity (id, kind, occurred_at, body, source, captured_by)
			VALUES ($1, $2, $3, $4, 'manual', 'human:x')`, kind, occurredAt, body)
	} else {
		id = s.seedID(t, `INSERT INTO activity
			(id, kind, direction, occurred_at, body, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, 'manual', 'human:x')`, kind, direction, occurredAt, body)
	}
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), id, person)
	s.seed(t, `INSERT INTO activity_link (id, activity_id, entity_type, organization_id)
		VALUES ($1, $2, 'organization', $3)`, ids.NewV7(), id, org)
	if direction != "" {
		s.seed(t, `INSERT INTO activity_participant (id, activity_id, person_id, role)
			VALUES ($1, $2, $3, $4)`, ids.NewV7(), id, person, participantRoleFor(direction))
	}
	return id
}

// TestCase6BothTheRecordAndTheProseReachTheCaller pins criterion 3's server
// half.
//
// The assertion is about the WORDS, not about two ids appearing in a list.
// catch_me_up_on summarises an activity as its subject or its kind — never its
// body (search/graph.go's timeline query) — so a test reading only that surface
// passes with the contradiction deleted, which is what an earlier version of
// this test did. The prose reaches a caller through read_record, so that is
// where it is asserted.
func TestCase6BothTheRecordAndTheProseReachTheCaller(t *testing.T) {
	s := boot(t, scopesRead)
	c := s.seedContradiction(t)

	// The timeline is how an assistant FINDS the two activities.
	got := s.MCP.CallOK(t, "catch_me_up_on", map[string]any{
		"record_type": "organization", "record_id": c.org.String(),
	})
	var answer agents.AssembledContextResult
	got.JSON(t, &answer)

	items := briefingItems(answer)
	byID := map[ids.UUID]agents.ContextItem{}
	for _, item := range items {
		byID[item.RecordID] = item
	}
	record, hasRecord := byID[c.complaint]
	if !hasRecord {
		t.Fatalf("case 6 criterion 3: the dated email is not on the timeline, so an assistant has "+
			"nothing to read; items were %v", summariesOf(items))
	}
	if _, hasProse := byID[c.postMortem]; !hasProse {
		t.Fatalf("case 6 criterion 3: the post-mortem note is not on the timeline, so the " +
			"contradiction this fixture exists to create was never presented")
	}

	// And this is the assertion that binds: the actual words, both of them.
	// Corrupting either body now fails, where checking ids alone did not.
	if body := s.activityBody(t, c.complaint); !strings.Contains(body, theRecordSays) {
		t.Fatalf("case 6 criterion 3: the complaint's own words did not reach the caller; the "+
			"record read back as %q", body)
	}
	if body := s.activityBody(t, c.postMortem); !strings.Contains(body, thePostMortemSays) {
		t.Fatalf("case 6 criterion 3: the post-mortem's prose did not reach the caller, so nothing "+
			"contradicts the record and the case tests nothing; it read back as %q", body)
	}

	// The record's own date, which is what makes preferring it possible. The
	// prose says a different month; only one of them is a fact.
	if record.OccurredAt == nil {
		t.Fatalf("case 6 criterion 2: the complaint email carries no occurred_at; a model holding " +
			"an undated record and a note that says 'im Oktober' will say October")
	}
	wantMonth := daysAgo(complaintDaysAgo).Month()
	if got := record.OccurredAt.Month(); got != wantMonth {
		t.Fatalf("case 6 criterion 2: the complaint is dated %s and the record says %s",
			wantMonth, got)
	}
	// The fixture is only a fixture if the two genuinely disagree. This guards
	// the TEST rather than the product: a prose line that stopped naming a
	// month, or started naming the right one, would leave every assertion above
	// passing while the case tested nothing.
	if !strings.Contains(thePostMortemSays, theWrongMonth) {
		t.Fatalf("case 6: the post-mortem prose no longer names %q, so there is no contradiction "+
			"for a model to resolve and this case proves nothing", theWrongMonth)
	}
	if strings.EqualFold(theWrongMonth, wantMonth.String()) {
		t.Fatalf("case 6: the prose names %s and so does the record — the fixture has to disagree "+
			"with itself for the criterion to mean anything", theWrongMonth)
	}
}

// activityBody reads one activity's body back through the tool surface.
//
// Through read_record rather than out of Postgres, deliberately: the criterion
// is that the words reach the CALLER, and a row holding the right text that no
// tool returns is exactly the class of defect this suite exists to catch.
func (s *scenario) activityBody(t *testing.T, activity ids.UUID) string {
	t.Helper()
	got := s.MCP.CallOK(t, "read_record", map[string]any{
		"record_type": "activity", "id": activity.String(),
	})
	var record struct {
		Fields struct {
			Body string `json:"body"`
		} `json:"fields"`
	}
	got.JSON(t, &record)
	return record.Fields.Body
}

// TestCase6EveryEventCarriesItsOwnDate pins criterion 2.
//
// Not the one event the previous test names — EVERY event item in the answer.
// A tool that dated the first item and left the rest bare would pass a
// single-record check while leaving most of a history undated.
func TestCase6EveryEventCarriesItsOwnDate(t *testing.T) {
	s := boot(t, scopesRead)
	c := s.seedContradiction(t)

	got := s.MCP.CallOK(t, "catch_me_up_on", map[string]any{
		"record_type": "organization", "record_id": c.org.String(),
	})
	var answer agents.AssembledContextResult
	got.JSON(t, &answer)

	seeded := map[ids.UUID]bool{c.complaint: true, c.postMortem: true}
	dated := 0
	for _, item := range briefingItems(answer) {
		if !seeded[item.RecordID] {
			continue
		}
		if item.OccurredAt == nil {
			t.Fatalf("case 6 criterion 2: the item summarising %q carries no date", item.Summary)
		}
		dated++
	}
	if dated != len(seeded) {
		t.Fatalf("case 6 criterion 2: %d of the %d seeded events reached the answer with a date",
			dated, len(seeded))
	}
}

// TestCase6EveryPastCaseReachesTheCallerWithItsHistory pins criterion 4.
//
// Three accounts lived through this, and all three have to arrive WITH the
// complaint that makes them relevant. An earlier version searched organization
// names for a token the fixture planted there, which proved only that lexical
// search works: breaking the link between a complaint and its company would not
// have failed it.
//
// So the sweep finds the accounts, and then each one's own history is read
// back. That second half is the criterion — "everything relevant reaches the
// caller" is about the evidence, not about the names.
func TestCase6EveryPastCaseReachesTheCallerWithItsHistory(t *testing.T) {
	s := boot(t, scopesRead)
	first := s.seedContradiction(t)
	more := s.seedTwoMorePastCases(t)

	got := s.MCP.CallOK(t, "search_context", map[string]any{
		"query":        sharedAccountWord,
		"record_types": []string{"organization"},
		"limit":        20,
	})
	var answer agents.SearchContextResult
	got.JSON(t, &answer)

	found := map[ids.UUID]bool{}
	for _, hit := range answer.Hits {
		found[hit.Record.ID] = true
	}
	for _, account := range append([]pastCase{first}, more...) {
		if !found[account.org] {
			t.Fatalf("case 6 criterion 4: %s lived through this and is missing from the sweep, so "+
				"an assistant would call a pattern a one-off",
				s.readString(t, "organization", "display_name", account.org))
		}
		// The complaint itself, through the timeline. A company that arrives
		// with no history attached is a name, not a past case.
		if !s.hasComplaintOnTimeline(t, account) {
			t.Fatalf("case 6 criterion 4: %s came back with no complaint on its timeline — the "+
				"account is named and the evidence that makes it relevant is not there",
				s.readString(t, "organization", "display_name", account.org))
		}
	}

	// The tool never claims a complete match set — a ranked page is the top of
	// an ordering. It has to say WHICH class it is, from the closed set it
	// publishes; an unrecognised word tells a caller nothing.
	if !slices.Contains([]string{agents.CoverageRankedSemantic, agents.CoveragePartialDegraded},
		answer.Coverage) {
		t.Fatalf("case 6 criterion 4: coverage is %q, which is not one of the classes this tool "+
			"publishes — a caller branching on it cannot act", answer.Coverage)
	}
}

// hasComplaintOnTimeline reads an account's history back and says whether the
// complaint that makes it a past case is on it.
func (s *scenario) hasComplaintOnTimeline(t *testing.T, account pastCase) bool {
	t.Helper()
	got := s.MCP.CallOK(t, "catch_me_up_on", map[string]any{
		"record_type": "organization", "record_id": account.org.String(),
	})
	var answer agents.AssembledContextResult
	got.JSON(t, &answer)
	for _, item := range briefingItems(answer) {
		if item.RecordID == account.complaint {
			return true
		}
	}
	return false
}
