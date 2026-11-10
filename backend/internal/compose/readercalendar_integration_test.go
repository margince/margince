// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A list of meetings says whether the reader's calendar feeds it, the same way
// at both doors, through the wiring each door is served with.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// listCalendarAtBothDoors reads one activity list as Rep1 through GET
// /activities and through list_records, and returns each door's `calendar`.
func listCalendarAtBothDoors(t *testing.T, e *integration.Env, kind crmcontracts.ListActivitiesParamsKind) (rest, tool *crmcontracts.ReaderCalendar) {
	t.Helper()
	ctx := horizonReader(e)

	req := httptest.NewRequest(http.MethodGet, "/v1/activities?kind="+string(kind), nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	newActivitiesHandlers(e.Pool).ListActivities(rec, req, crmcontracts.ListActivitiesParams{Kind: &[]crmcontracts.ListActivitiesParamsKind{kind}})
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /activities answered %d: %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.ActivityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decoding the REST page: %v", err)
	}

	var listed struct {
		Calendar *crmcontracts.ReaderCalendar `json:"calendar"`
	}
	invokeAnswer(ctx, t, composedRegistry(e), "list_records",
		`{"record_type":"activity","filters":{"kind":"`+string(kind)+`"}}`, &listed)
	return page.Calendar, listed.Calendar
}

func TestBothDoorsSayWhetherTheReadersCalendarFeedsAMeetingList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		connect bool
		failing bool
		want    crmcontracts.ReaderCalendar
	}{
		{"none", false, false, crmcontracts.ReaderCalendarNotConnected},
		{"connected", true, false, crmcontracts.ReaderCalendarConnected},
		{"sync failing", true, true, crmcontracts.ReaderCalendarUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			if tc.connect {
				seedCalendar(t, e, "gcal", "connected", tc.failing)
			}

			rest, tool := listCalendarAtBothDoors(t, e, crmcontracts.ListActivitiesParamsKindMeeting)

			if rest == nil || *rest != tc.want {
				t.Errorf("GET /activities calendar = %v, want %q", rest, tc.want)
			}
			if tool == nil || *tool != tc.want {
				t.Errorf("list_records calendar = %v, want %q", tool, tc.want)
			}
		})
	}
}

// A list that cannot be read as the diary owes no calendar answer at either door.
func TestAnEmailListCarriesNoCalendarAnswer(t *testing.T) {
	e := integration.Setup(t)

	rest, tool := listCalendarAtBothDoors(t, e, crmcontracts.ListActivitiesParamsKindEmail)

	if rest != nil || tool != nil {
		t.Errorf("an email list answered calendar %v (REST) and %v (tool), want neither", rest, tool)
	}
}

// The window rule, over the function both doors are wired to. A list of every
// kind reads as the diary with a window, and a list of named kinds when it holds meetings.
func TestAListOfEveryKindReadsAsTheDiaryOnlyOverAWindow(t *testing.T) {
	e := integration.Setup(t)
	calendar := activityListCalendar(captureHealthRegistry(e.DB()))
	ctx := horizonReader(e)

	for _, tc := range []struct {
		kinds    []string
		windowed bool
		owed     bool
	}{
		{nil, true, true},
		{nil, false, false},
		{[]string{"email", "meeting"}, true, true},
		{[]string{"email", "meeting"}, false, true},
		{[]string{"email"}, true, false},
	} {
		_, owed, err := calendar(ctx, datasource.ActivityListScope{Kinds: tc.kinds, Windowed: tc.windowed})
		if err != nil {
			t.Fatalf("reading the calendar answer: %v", err)
		}
		if owed != tc.owed {
			t.Errorf("a list of kinds %v, windowed %v, owed an answer %v, want %v", tc.kinds, tc.windowed, owed, tc.owed)
		}
	}
	if _, owed, err := calendar(ctx, datasource.ActivityListScope{Kinds: []string{"meeting"}, Narrowed: true}); err != nil || owed {
		t.Errorf("a meeting list held to one record or assignee is not the reader's diary, yet owed an answer (%v): %v", owed, err)
	}
	if _, owed, err := calendar(context.Background(), datasource.ActivityListScope{Kinds: []string{"meeting"}}); err != nil || owed {
		t.Errorf("a caller with no actor is refused its connections, which owes no answer (owed %v), not an error: %v", owed, err)
	}
}
