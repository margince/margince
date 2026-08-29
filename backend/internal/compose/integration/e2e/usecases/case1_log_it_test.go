// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 1 — Log it while it is fresh.
//
// The prompt, spoken, from the car:
//
//	Just left Kugellager-online. Met Matthias Ortner, went well, they want
//	to move forward. Grab the transcript from Plaud and put it all in the
//	CRM.
//
// Nothing it names exists yet: the company, the person and the deal all have to
// be created from one sentence naming no fields.
//
// UNLIKE cases 4 and 5, this suite WRITES through the tools rather than seeding
// rows. That is the point of case 1 — the write path IS the journey — and it is
// also why the fixtures elsewhere had to be corrected: rows inserted by hand
// skip what real writes maintain. Here nothing is inserted by hand at all.
//
// The transcript arrives as TEXT. Fetching it from Plaud and reading the
// calendar are the assistant's half, and the server cannot tell a pasted
// transcript from an uploaded one — the contract says so outright: "There is no
// presigned file-upload path in V1 — a plain-text .txt file is read to text
// client-side and sent the same way a paste is."
//
// NOT covered here: whether a MODEL reads the right commitments out of the
// transcript (criterion 8, the weekly lane's question — the reading calls a
// model and this suite wires an insert-only runner with no model lane), and
// whether the assistant REPORTS what it staged (criterion 14, also a model
// question).

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The meeting this case logs, as the assistant would supply it after reading a
// recording. Line 3 carries the one plainly stated commitment; line 2 is a
// topic discussed WITHOUT one, which is what proves a reader is reporting what
// the transcript says rather than everything it mentions.
const meetingTranscript = "Matthias: Danke für die Übersicht zu den Lieferzeiten.\n" +
	"Lars: Wie sieht es mit dem Thema Verpackung aus?\n" +
	"Lars: Ich schicke Ihnen die Preisliste bis Freitag.\n" +
	"Matthias: Passt, dann schauen wir uns das nächste Woche an."

const (
	newCompanyName = "Kugellager-online.de"
	newPersonName  = "Matthias Ortner"
	newDealName    = "Erstauftrag Kugellager"
)

// loggedMeeting is what one run of this journey created.
type loggedMeeting struct {
	org      ids.UUID
	person   ids.UUID
	deal     ids.UUID
	activity ids.UUID
}

// createRecord creates one record through the tool and returns its id.
func (s *scenario) createRecord(t *testing.T, recordType string, fields map[string]any) ids.UUID {
	t.Helper()
	got := s.MCP.CallOK(t, "create_record", map[string]any{
		"record_type": recordType, "fields": fields,
	})
	var created struct {
		ID ids.UUID `json:"id"`
	}
	got.JSON(t, &created)
	if created.ID.IsZero() {
		t.Fatalf("create_record answered no id for the %s:\n%s", recordType, got.Text)
	}
	return created.ID
}

// logTheMeeting walks the journey: the company, the person who works there, the
// deal, and the meeting linked to all three in ONE call.
//
// Every step goes through a tool. Nothing here inserts a row, so what the test
// then reads is a state the product produced rather than one a fixture claimed.
func (s *scenario) logTheMeeting(t *testing.T) loggedMeeting {
	t.Helper()
	var m loggedMeeting

	m.org = s.createRecord(t, "organization", map[string]any{"display_name": newCompanyName})
	m.person = s.createRecord(t, "person", map[string]any{"full_name": newPersonName})
	// The employment edge, so the person is AT the company rather than merely
	// mentioned in the same conversation.
	s.MCP.CallOK(t, "create_record", map[string]any{
		"record_type": "relationship",
		"fields": map[string]any{
			"kind": "employment", "person_id": m.person.String(), "organization_id": m.org.String(),
		},
	})

	pipeline, stage := s.defaultOpenStage(t)
	m.deal = s.createRecord(t, "deal", map[string]any{
		"name": newDealName, "organization_id": m.org.String(),
		"pipeline_id": pipeline.String(), "stage_id": stage.String(),
	})

	// EVERY LINK IN ONE CALL. The tool's own copy says so — "Every record this
	// was about, ALL OF THEM in this call" — and an earlier run that supplied
	// them one at a time raised three relink approvals for what is not a
	// decision.
	//
	// The company is NOT among them, and cannot be: a meeting is with a person,
	// and it reaches the company through the employment edge written above. The
	// tool refuses the company link rather than storing one, which the case
	// below asserts from the other end.
	got := s.MCP.CallOK(t, "log_activity", map[string]any{
		"kind":          "meeting",
		"body":          meetingTranscript,
		"source_system": transcriptMarker,
		"links": []map[string]any{
			{"entity_type": "person", "entity_id": m.person.String()},
			{"entity_type": "deal", "entity_id": m.deal.String()},
		},
	})
	var logged struct {
		ID ids.UUID `json:"id"`
	}
	got.JSON(t, &logged)
	m.activity = logged.ID
	return m
}

// transcriptMarker is the one value that turns a body into a transcript.
//
// A caller cannot guess a magic string, and the honest name for where a
// recording came from — "plaud" — is not it. The tool documentation says the
// value out loud for exactly that reason.
const transcriptMarker = "transcript"

// TestCase1TheCompanyThePersonAndTheDealAllExistAfterwards pins criteria 1, 2
// and 3.
//
// None of the three existed before. If the assistant reports success and only
// some of them are there, the case has failed — and the employment edge must be
// written a single time, because an early run left the person's page listing
// the company twice.
//
// Held by: this test's own employment-edge count, below.
func TestCase1TheCompanyThePersonAndTheDealAllExistAfterwards(t *testing.T) {
	s := boot(t, scopesReadWrite)
	m := s.logTheMeeting(t)

	for _, want := range []struct {
		table, column string
		id            ids.UUID
		value         string
	}{
		{"organization", "display_name", m.org, newCompanyName},
		{"person", "full_name", m.person, newPersonName},
		{"deal", "name", m.deal, newDealName},
	} {
		if got := s.readString(t, want.table, want.column, want.id); got != want.value {
			t.Fatalf("case 1 criterion 1: the %s should be %q and reads %q",
				want.table, want.value, got)
		}
	}

	// Criterion 2: attached once, not twice.
	if edges := s.countRows(t, `SELECT count(*) FROM relationship
		WHERE kind = 'employment' AND person_id = $1 AND organization_id = $2
		  AND archived_at IS NULL`, m.person, m.org); edges != 1 {
		t.Fatalf("case 1 criterion 2: %s is linked to %s %d times — a duplicate edge makes the "+
			"person's page list the company twice", newPersonName, newCompanyName, edges)
	}

	// Criterion 3: nothing invented. The conversation named no amount and no
	// close date, so both must be empty rather than filled with a plausible
	// number.
	if filled := s.countRows(t, `SELECT count(*) FROM deal
		WHERE id = $1 AND (amount_minor IS NOT NULL OR expected_close_date IS NOT NULL)`,
		m.deal); filled != 0 {
		t.Fatalf("case 1 criterion 3: the deal carries an amount or a close date, neither of which " +
			"was discussed — an empty field is the correct answer when nothing was said")
	}
}

// TestCase1TheMeetingLandsOnEveryRecordInOneWrite pins criteria 4 and 5.
//
// One write, not one write plus three corrections: the three link rows share a
// timestamp because they were written together. And no approval is raised —
// connecting a meeting to the company it was with is not a decision.
func TestCase1TheMeetingLandsOnEveryRecordInOneWrite(t *testing.T) {
	s := boot(t, scopesReadWrite)
	m := s.logTheMeeting(t)

	for _, want := range []struct {
		column string
		id     ids.UUID
		what   string
	}{
		{"person_id", m.person, "the person"},
		{"deal_id", m.deal, "the deal"},
	} {
		if n := s.countRows(t,
			`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND `+want.column+` = $2`,
			m.activity, want.id); n != 1 {
			t.Fatalf("case 1 criterion 4: the meeting links to %s %d times, want exactly once",
				want.what, n)
		}
	}
	// And NOT to the company, which is not somebody you can meet. The account
	// still sees the meeting — through the person who was in it — and that half
	// is asserted below against the timeline an assistant actually reads.
	if n := s.countRows(t,
		`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND organization_id = $2`,
		m.activity, m.org); n != 0 {
		t.Fatalf("case 1 criterion 4: the meeting is filed against the company %d times; a meeting "+
			"is with a person, and a direct link is the redundancy that made two records disagree "+
			"about who was in the room", n)
	}

	// Written together, IN THE SAME TRANSACTION AS THE ACTIVITY.
	//
	// The three links sharing a created_at proves only that they shared a
	// transaction — Postgres now() is transaction-scoped — and a build that
	// committed the activity and then relinked in a second transaction would
	// still show one distinct value among the links. Comparing them against the
	// ACTIVITY's own stamp is what closes that: two transactions cannot agree on
	// now(), so equality here means one write covered both.
	if together := s.countRows(t, `SELECT count(*) FROM activity_link l
		JOIN activity a ON a.id = l.activity_id
		WHERE l.activity_id = $1 AND l.created_at = a.created_at`, m.activity); together != 2 {
		t.Fatalf("case 1 criterion 4: %d of the meeting's 2 links were written in the same "+
			"transaction as the meeting itself — the rest are a relink afterwards, which is what "+
			"used to stop for three separate approvals", together)
	}

	// Criterion 5: nothing was staged for a human. An approval here would mean
	// a rep is asked to confirm that a meeting they just had was with the
	// company they had it with.
	if staged := s.countRows(t, `SELECT count(*) FROM approval`); staged != 0 {
		t.Fatalf("case 1 criterion 5: logging the meeting staged %d approval(s); linking a meeting "+
			"to its own records is not a decision", staged)
	}
}

// The other half of criterion 4: the company link a meeting cannot carry was a
// REDUNDANCY, not the account's only way of seeing the meeting.
//
// This is the assertion the rule rests on. Forbidding the direct link without
// it removes the company from every surface that assembles context — which is
// what happened the first time the refusal shipped, and why it was withdrawn.
// The account reaches the meeting through the person who was in the room, and
// the timeline an assistant actually reads is where that has to be true.
func TestCase1TheAccountSeesTheMeetingThroughThePersonWhoWasInIt(t *testing.T) {
	s := boot(t, scopesReadWrite)
	m := s.logTheMeeting(t)

	got := s.MCP.CallOK(t, "catch_me_up_on", map[string]any{
		"record_type": "organization", "record_id": m.org.String(),
	})
	var answer agents.AssembledContextResult
	got.JSON(t, &answer)

	items := briefingItems(answer)
	for _, item := range items {
		if item.RecordID == m.activity {
			return
		}
	}
	t.Fatalf("case 1 criterion 4: the meeting is not on %s's timeline, so the company link was the "+
		"only thing carrying it there and forbidding that link cost the account the meeting; "+
		"items were %v", newCompanyName, summariesOf(items))
}

// And the refusal itself, from the door an assistant knocks on.
//
// A check violation surfaced raw says "a value in this request is outside what
// its field accepts", which names neither the link nor what to do instead — so
// a model retries the same call or gives up on the write. The tool answers with
// the rule and the alternative in one sentence.
func TestCase1FilingTheMeetingAgainstTheCompanyIsRefusedWithSomethingToDo(t *testing.T) {
	s := boot(t, scopesReadWrite)
	m := s.logTheMeeting(t)

	refusal := s.MCP.CallRefused(t, "log_activity", map[string]any{
		"kind": "meeting", "body": meetingTranscript,
		"links": []map[string]any{
			{"entity_type": "person", "entity_id": m.person.String()},
			{"entity_type": "organization", "entity_id": m.org.String()},
		},
	})
	for _, want := range []string{"with a person", "employer"} {
		if !strings.Contains(refusal, want) {
			t.Errorf("the refusal does not say %q, so it tells the caller what is wrong without "+
				"telling them what to do instead: %s", want, refusal)
		}
	}
}

// TestCase1TheTranscriptIsStoredAsATranscriptAndReadWithoutAsking pins criteria
// 6, 7 and 10.
//
// The reading starts when the transcript LANDS, not when somebody asks. The
// extraction lane was fully built and had never run once — transcript_read held
// zero rows — because the only thing that enqueued it was an endpoint nothing
// called.
//
// Criterion 10 is the one that can only break on this path: the reading must be
// requested by the HUMAN behind the passport, not by the agent. Proposals routed
// to a software account reach nobody.
func TestCase1TheTranscriptIsStoredAsATranscriptAndReadWithoutAsking(t *testing.T) {
	s := bootWithTranscriptReading(t)
	m := s.logTheMeeting(t)

	// Criterion 6: the system knows this body is a recording of a conversation
	// rather than notes about one.
	if got := s.readString(t, "activity", "coalesce(source_system, '')", m.activity); got != transcriptMarker {
		t.Fatalf("case 1 criterion 6: the meeting's source_system is %q, want %q — any other value "+
			"stores the text and reads nothing", got, transcriptMarker)
	}

	// Criterion 7: a reading was queued by the landing itself. Nobody asked.
	var requestedBy string
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT requested_by FROM transcript_read WHERE activity_id = $1`, m.activity).
			Scan(&requestedBy)
	})
	if err != nil {
		t.Fatalf("case 1 criterion 7: no reading was queued for the transcript that just landed, so "+
			"a recording nobody asked to read is a recording nobody reads: %v", err)
	}

	// Criterion 10: routed to the human, not to the passport.
	//
	// Asserted through the SAME parser the product routes with, not by looking
	// at the string's shape. A value like "system:<uuid>" or "human:<uuid>:junk"
	// is neither an agent prefix nor missing the rep's id, so a shape check
	// passes while the proposals reach nobody — which is the whole failure.
	routedTo, isHuman := principal.HumanUserID(requestedBy)
	if !isHuman {
		t.Fatalf("case 1 criterion 10: the reading was requested by %q, which the product's own "+
			"parser does not read as a person — the proposals it stages reach no rep", requestedBy)
	}
	if routedTo != s.Rep {
		t.Fatalf("case 1 criterion 10: the reading is routed to %s, and the human behind the "+
			"passport is %s", routedTo, s.Rep)
	}
}

// readString reads one column off one row, for an assertion about what the
// product wrote.
func (s *scenario) readString(t *testing.T, table, column string, id ids.UUID) string {
	t.Helper()
	var out string
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT `+column+` FROM `+table+` WHERE id = $1`, id).Scan(&out)
	})
	if err != nil {
		t.Fatalf("reading %s.%s for %s: %v", table, column, id, err)
	}
	return out
}

// countRows answers one counting query.
func (s *scenario) countRows(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	err := apptest.InWorkspace(s.AppEnv, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(&n)
	})
	if err != nil {
		t.Fatalf("counting: %v\n%s", err, sql)
	}
	return n
}
