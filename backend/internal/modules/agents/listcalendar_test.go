// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// calendarAsked records what list_records asked the calendar seam, and answers
// with a fixed reader calendar, or with none owed when answer is empty.
type calendarAsked struct {
	answer crmcontracts.ReaderCalendar
	scope  datasource.ActivityListScope
	asked  bool
}

func (c *calendarAsked) seam(_ context.Context, scope datasource.ActivityListScope) (crmcontracts.ReaderCalendar, bool, error) {
	c.asked, c.scope = true, scope
	return c.answer, c.answer != "", nil
}

func listWithCalendar(t *testing.T, calendar *calendarAsked, args string) (Envelope, ListRecordsResult) {
	t.Helper()
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}), WithActivityListCalendar(calendar.seam))
	RegisterListTool(registry, &listProbeProvider{}, probeVocabulary{})
	sealed, err := registry.Invoke(scopedAgentCtx(principal.ScopeRead), "list_records", json.RawMessage(args))
	if err != nil {
		t.Fatalf("list_records: %v", err)
	}
	env := sealedEnvelope(t, sealed)
	var out ListRecordsResult
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("list_records answered %s: %v", env.Data, err)
	}
	return env, out
}

func calendarCaveated(env Envelope) bool {
	_, unconnected := warningNamed(env, warningNoCalendarConnected)
	_, unreadable := warningNamed(env, warningCalendarUnreadable)
	return unconnected || unreadable
}

// An empty meeting list for a user with no calendar is the meetings somebody
// typed in, not their diary. The answer says so, and gives up the authority
// claim an empty diary would earn.
func TestAMeetingListWithNoCalendarSaysItIsNotTheDiary(t *testing.T) {
	for _, tc := range []struct {
		calendar crmcontracts.ReaderCalendar
		code     string
	}{
		{crmcontracts.ReaderCalendarNotConnected, warningNoCalendarConnected},
		{crmcontracts.ReaderCalendarUnreadable, warningCalendarUnreadable},
	} {
		t.Run(string(tc.calendar), func(t *testing.T) {
			calendar := &calendarAsked{answer: tc.calendar}
			env, out := listWithCalendar(t, calendar,
				`{"record_type":"activity","filters":{"kind":"meeting","occurred_after":"2026-11-07T00:00:00Z"}}`)

			if !slices.Equal(calendar.scope.Kinds, []string{"meeting"}) || !calendar.scope.Windowed {
				t.Errorf("the seam was asked kinds %q windowed %v, want the call's meeting window", calendar.scope.Kinds, calendar.scope.Windowed)
			}
			if out.Calendar == nil || *out.Calendar != tc.calendar {
				t.Errorf("calendar = %v, want %q, the field GET /activities publishes", out.Calendar, tc.calendar)
			}
			warning, warned := warningNamed(env, tc.code)
			if !warned {
				t.Fatalf("no %s warning on a meeting list that is not the diary: %v", tc.code, env.Warnings)
			}
			if !strings.Contains(warning.Message, "may still be in their calendar") {
				t.Errorf("the warning no longer names the conclusion not to draw: %q", warning.Message)
			}
			if env.Freshness.Authoritative {
				t.Error("a meeting list that is not the diary claims to be authoritative about it")
			}
		})
	}
}

// A connected calendar feeds the list, so an empty day is a real answer with no
// caveat. A list the seam owes nothing for carries no field.
func TestAMeetingListFedByTheCalendarOrNotADiaryCarriesNoCaveat(t *testing.T) {
	connected := &calendarAsked{answer: crmcontracts.ReaderCalendarConnected}
	env, out := listWithCalendar(t, connected, `{"record_type":"activity","filters":{"kind":"meeting"}}`)
	if out.Calendar == nil || *out.Calendar != crmcontracts.ReaderCalendarConnected {
		t.Errorf("calendar = %v, want connected", out.Calendar)
	}
	if calendarCaveated(env) || !env.Freshness.Authoritative {
		t.Errorf("a list the user's calendar feeds was caveated: warnings %v, authoritative %v",
			env.Warnings, env.Freshness.Authoritative)
	}

	notADiary := &calendarAsked{}
	env, out = listWithCalendar(t, notADiary, `{"record_type":"activity","filters":{"kind":"email"}}`)
	if !slices.Equal(notADiary.scope.Kinds, []string{"email"}) || notADiary.scope.Windowed {
		t.Errorf("the seam was asked kinds %q windowed %v, want email with no window", notADiary.scope.Kinds, notADiary.scope.Windowed)
	}
	if out.Calendar != nil || calendarCaveated(env) {
		t.Errorf("an email list carries a calendar answer: %v, warnings %v", out.Calendar, env.Warnings)
	}

	contacts := &calendarAsked{answer: crmcontracts.ReaderCalendarNotConnected}
	if _, out = listWithCalendar(t, contacts, `{"record_type":"contact"}`); contacts.asked || out.Calendar != nil {
		t.Errorf("a contact list asked the calendar seam (%v) or carries its answer: %v", contacts.asked, out.Calendar)
	}
}

// A meeting list held to a record, project or assignee is not the reader's
// diary. The tool tells the seam so, whichever filter holds it.
func TestAMeetingListNarrowedToOneSubjectSaysSoToTheSeam(t *testing.T) {
	for _, filter := range []string{"entity_id", "assignee_id", "project_id"} {
		t.Run(filter, func(t *testing.T) {
			calendar := &calendarAsked{}
			listWithCalendar(t, calendar,
				`{"record_type":"activity","filters":{"kind":"meeting","`+filter+`":"0190a8a0-0000-7000-8000-000000000001"}}`)
			if !calendar.scope.Narrowed {
				t.Errorf("a meeting list filtered by %s reached the seam as un-narrowed", filter)
			}
		})
	}
}
