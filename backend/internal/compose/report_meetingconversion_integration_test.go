// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Held meeting to accepted opportunity, against a real database.
//
// Every rule this report depends on is SQL: which meetings count, which handoff
// a meeting is allowed to claim, and — the one that matters most — that a deal
// which merely appeared at the same company claims nothing at all. None of it is
// reachable from a unit test, and each can be wrong in a way that produces a
// plausible number.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A HELD meeting whose prospect was handed on and accepted converts. One that
// was merely booked, or no-showed, is not in the report at all.
func TestOnlyHeldMeetingsCountTowardTheConversion(t *testing.T) {
	e := setupForecast(t)
	lead := e.seedHandoffLead(t, "converted@customer.test")

	e.seedMeeting(t, "held", lead, "2026-03-01T10:00:00Z")
	e.seedMeeting(t, "booked", lead, "2026-03-02T10:00:00Z")
	e.seedMeeting(t, "no_show", lead, "2026-03-03T10:00:00Z")
	e.seedMeeting(t, "canceled", lead, "2026-03-04T10:00:00Z")
	e.acceptHandoff(t, lead, "2026-03-10T10:00:00Z")

	result := e.runReport(e.activityReader(tableLead), t, "meeting-conversion",
		`{"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`)

	// One row, one meeting: the three that were not held are absent rather than
	// counted as failures to convert.
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want 1 — only the held meeting belongs here: %+v", len(result.Rows), result.Rows)
	}
	converted := conversionRow(t, result, true)
	if got := wireInt(t, converted, "meetings"); got != 1 {
		t.Errorf("converted meetings = %d, want 1", got)
	}
}

// THE test for this report: a deal at the same company that nobody handed on
// converts NOTHING.
//
// This is the inference the report exists to refuse. It credits an SDR for work
// somebody else sourced, and it is wrong in the direction that flatters — the
// error grows with account size, because the biggest customers generate the most
// coincidental matches. A report that got this wrong would still produce a
// number, and the number would look better than the truth.
func TestACoincidentalDealAtTheSameCompanyConvertsNothing(t *testing.T) {
	e := setupForecast(t)
	company := e.seedHandoffCompany(t)
	metNobodyHandedOn := e.seedHandoffLead(t, "met-only@customer.test")
	e.seedMeeting(t, "held", metNobodyHandedOn, "2026-04-01T10:00:00Z")
	// A deal at that same company, sourced by somebody else entirely: no handoff
	// names this prospect, so nothing connects the two but the account.
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Someone else''s deal', $2, $3, $4, 900000, 'EUR', 'manual', 'human:x')`,
		e.pipeline, e.stages[60], company)

	result := e.runReport(e.activityReader(tableLead), t, "meeting-conversion",
		`{"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`)

	unconverted := conversionRow(t, result, false)
	if got := wireInt(t, unconverted, "meetings"); got != 1 {
		t.Errorf("unconverted meetings = %d, want 1 — the company's other deal converted this meeting", got)
	}
	// And no converted bucket exists at all: nothing in this fixture converted.
	for _, row := range result.Rows {
		if row["became_opportunity"] == true {
			t.Errorf("a meeting converted with no handoff behind it: %+v", row)
		}
	}
}

// An acceptance BEFORE the meeting converts nothing either.
//
// A prospect accepted in March and met in June was not converted by that June
// meeting — the opportunity already existed when it happened. Without the
// ordering the report would credit the meeting for it.
func TestAnEarlierAcceptanceDoesNotConvertALaterMeeting(t *testing.T) {
	e := setupForecast(t)
	lead := e.seedHandoffLead(t, "accepted-first@customer.test")
	e.acceptHandoff(t, lead, "2026-03-01T10:00:00Z")
	e.seedMeeting(t, "held", lead, "2026-06-01T10:00:00Z")

	result := e.runReport(e.activityReader(tableLead), t, "meeting-conversion",
		`{"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`)

	unconverted := conversionRow(t, result, false)
	if got := wireInt(t, unconverted, "meetings"); got != 1 {
		t.Errorf("unconverted = %d, want 1 — an acceptance that predates the meeting converted it", got)
	}
}

// A prospect handed on TWICE still counts its meeting once.
//
// Recycled-and-resubmitted is an ordinary path, so a plain join would multiply
// the meeting by its handoffs — and a rate whose numerator can exceed its
// denominator is not a rate.
func TestAProspectHandedOnTwiceCountsItsMeetingOnce(t *testing.T) {
	e := setupForecast(t)
	lead := e.seedHandoffLead(t, "twice@customer.test")
	e.seedMeeting(t, "held", lead, "2026-05-01T10:00:00Z")
	e.acceptHandoff(t, lead, "2026-05-10T10:00:00Z")
	e.acceptHandoff(t, lead, "2026-05-20T10:00:00Z")

	result := e.runReport(e.activityReader(tableLead), t, "meeting-conversion",
		`{"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`)

	converted := conversionRow(t, result, true)
	if got := wireInt(t, converted, "meetings"); got != 1 {
		t.Errorf("converted meetings = %d, want 1 — two handoffs multiplied one meeting", got)
	}
}

// The conversion is a fact about a HANDOFF, so it takes the handoff's grant.
//
// A seat holding activity.read alone can see the meeting; whether an AE accepted
// the prospect is recorded on sdr_handoff, which gates on the `lead` object. The
// dimension is this report's DEFAULT grouping, so without the grant an
// unfiltered run would disclose it having named nothing at all — and the filter
// form turns it into a per-meeting oracle: ask for became_opportunity=true, read
// the count, and learn the acceptance one meeting at a time.
//
// Both halves are asserted. A refusal test with no admitted case passes equally
// against a report refusing everybody, which would be a different bug wearing
// this test as cover.
func TestTheConversionTakesTheHandoffGrant(t *testing.T) {
	e := setupForecast(t)
	lead := e.seedHandoffLead(t, "gated@customer.test")
	e.seedMeeting(t, "held", lead, "2026-06-01T10:00:00Z")
	e.acceptHandoff(t, lead, "2026-06-10T10:00:00Z")

	const grouped = `{"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`
	if status, body := e.runReportStatus(e.activityReader(), t, "meeting-conversion", grouped); status != http.StatusForbidden {
		t.Errorf("grouping by became_opportunity without lead.read → %d, want 403: %s", status, body)
	}

	// The filter is the same disclosure asked one meeting at a time, and it is a
	// separate entry in spec.filters — a grant applied to the dimension alone
	// would leave this arm open.
	const filtered = `{"filters":{"became_opportunity":true},"aggregates":[{"fn":"count","as":"meetings"}]}`
	if status, body := e.runReportStatus(e.activityReader(), t, "meeting-conversion", filtered); status != http.StatusForbidden {
		t.Errorf("filtering on became_opportunity without lead.read → %d, want 403: %s", status, body)
	}

	// With the grant the same request is answered, which is what makes the two
	// refusals above about the grant rather than about the report.
	if status, body := e.runReportStatus(e.activityReader(tableLead), t, "meeting-conversion", grouped); status != http.StatusOK {
		t.Errorf("grouping by became_opportunity WITH lead.read → %d, want 200: %s", status, body)
	}
}

// conversionRow finds the bucket for one side of the conversion.
//
// bucketRow beside it compares strings, and this dimension arrives as a real
// JSON boolean — the expression is a predicate, not a label. Comparing it as
// "true" matches nothing and reads as a report that returned no rows, which is
// the wrong diagnosis for a fixture that is working.
func conversionRow(t *testing.T, result reportResultWire, want bool) map[string]any {
	t.Helper()
	for _, row := range result.Rows {
		if got, ok := row["became_opportunity"].(bool); ok && got == want {
			return row
		}
	}
	t.Fatalf("no row with became_opportunity=%v in %+v", want, result.Rows)
	return nil
}

// seedMeeting writes one meeting at a given standing, linked to a lead.
func (e *forecastEnv) seedMeeting(t *testing.T, status string, lead ids.UUID, occurredAt string) {
	t.Helper()
	id := e.seedID(t, `INSERT INTO activity (id, kind, direction, meeting_status, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'meeting', 'outbound', $2, 'Intro call', $3::timestamptz, gen_random_uuid()::text, 'manual', 'human:x')`,
		status, occurredAt)
	e.seedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, lead_id) VALUES ($1, $2, 'lead', $3)`,
		id, lead)
}

// acceptHandoff writes an accepted handoff for a lead, with the deal it
// produced. Seeded directly rather than through the store: this test is about
// the REPORT's join, and reaching for the contacts module here would drag its
// whole permission context into a compose fixture.
func (e *forecastEnv) acceptHandoff(t *testing.T, lead ids.UUID, decidedAt string) {
	t.Helper()
	deal := e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, amount_minor, currency, source, captured_by)
		VALUES ($1, 'Accepted opportunity', $2, $3, 250000, 'EUR', 'manual', 'human:x')`,
		e.pipeline, e.stages[60])
	e.seedID(t, `INSERT INTO sdr_handoff (id, lead_id, submitted_by, status, deal_id, decided_at, captured_by)
		VALUES ($1, $2, $3, 'accepted', $4, $5::timestamptz, 'human:x')`,
		lead, e.Rep1, deal, decidedAt)
}

func (e *forecastEnv) seedHandoffCompany(t *testing.T) ids.UUID {
	t.Helper()
	return e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Handoff Customer', 'manual', 'human:x')`)
}

func (e *forecastEnv) seedHandoffLead(t *testing.T, email string) ids.UUID {
	t.Helper()
	return e.seedID(t, `INSERT INTO lead (id, email, full_name, source, captured_by)
		VALUES ($1, $2, 'Prospect', 'manual', 'human:x')`, email)
}
