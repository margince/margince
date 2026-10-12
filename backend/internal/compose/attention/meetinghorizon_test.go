// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// When the reader's own page says whether their calendar feeds the meetings
// count, and when it points past an empty day to the next customer meeting.

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubHorizon answers both questions and records the window it was asked about.
type stubHorizon struct {
	calendar    crmcontracts.WorklistCalendar
	calendarErr error
	next        *crmcontracts.Contact360NextMeeting
	nextErr     error
	asked       bool
	after       time.Time
	before      time.Time
}

func (h *stubHorizon) Calendar(context.Context) (crmcontracts.WorklistCalendar, error) {
	return h.calendar, h.calendarErr
}

func (h *stubHorizon) NextCustomerMeeting(
	_ context.Context, after, before time.Time,
) (crmcontracts.Contact360NextMeeting, bool, error) {
	h.asked, h.after, h.before = true, after, before
	if h.next == nil {
		return crmcontracts.Contact360NextMeeting{}, false, h.nextErr
	}
	return *h.next, true, h.nextErr
}

// threeDaysOut is a booked customer meeting past today, as the seam hands it over.
func threeDaysOut() *crmcontracts.Contact360NextMeeting {
	return &crmcontracts.Contact360NextMeeting{
		ActivityId: openapi_types.UUID(ids.NewV7()),
		StartsAt:   readInstant.Add(72 * time.Hour),
	}
}

func horizonService(meetings *stubMeetings, horizon *stubHorizon) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, meetings,
		nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithMeetingHorizon(horizon)
}

func readHorizonPage(t *testing.T, svc *Service, scope string) crmcontracts.Worklist {
	t.Helper()
	page, err := svc.Worklist(pageReader(), scope, "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}
	return page
}

func TestAnEmptyDayNamesTheCalendarAndTheNextCustomerMeeting(t *testing.T) {
	horizon := &stubHorizon{calendar: crmcontracts.WorklistCalendarConnected, next: threeDaysOut()}

	page := readHorizonPage(t, horizonService(&stubMeetings{}, horizon), scopeMine)

	if page.Calendar == nil || *page.Calendar != crmcontracts.WorklistCalendarConnected {
		t.Errorf("calendar = %v, want connected", page.Calendar)
	}
	if page.NextMeeting == nil || page.NextMeeting.ActivityId != horizon.next.ActivityId {
		t.Fatalf("next_meeting = %+v, want the meeting three days out", page.NextMeeting)
	}
	// The window opens where today's lane stopped reading and runs thirty days.
	endOfToday := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if !horizon.after.Equal(endOfToday) || !horizon.before.Equal(endOfToday.Add(30*24*time.Hour)) {
		t.Errorf("asked [%s, %s), want [%s, +30d)", horizon.after, horizon.before, endOfToday)
	}
}

// A meeting still ahead today is the next conversation, so nothing points past it.
func TestADayWithAMeetingLeftNamesNoNextMeeting(t *testing.T) {
	today := &stubMeetings{rows: []Meeting{{ID: ids.NewV7(), Subject: "later", StartsAt: readInstant.Add(time.Hour)}}}
	horizon := &stubHorizon{calendar: crmcontracts.WorklistCalendarConnected, next: threeDaysOut()}

	page := readHorizonPage(t, horizonService(today, horizon), scopeMine)

	if page.NextMeeting != nil || horizon.asked {
		t.Errorf("a day with a meeting left asked=%v and named %+v past it", horizon.asked, page.NextMeeting)
	}
	if page.Calendar == nil {
		t.Error("the calendar answer depends on the scope, not on whether today is busy")
	}
}

// A withheld lane's emptiness is not a free day: no pointer past it, and the
// withheld source is named by the word the meetings reading matches on.
func TestAWithheldMeetingsLaneIsNamedAsTheMeetingSourceAndPointsNowhere(t *testing.T) {
	refused := &stubMeetings{err: apperrors.ErrPermissionDenied}
	horizon := &stubHorizon{calendar: crmcontracts.WorklistCalendarConnected, next: threeDaysOut()}

	page := readHorizonPage(t, horizonService(refused, horizon), scopeMine)

	if page.NextMeeting != nil || horizon.asked {
		t.Errorf("a withheld lane asked=%v and named %+v as the next meeting", horizon.asked, page.NextMeeting)
	}
	entry, named := unavailableEntry(page, sourceMeeting)
	if !named {
		t.Fatalf("sources_unavailable = %+v, want the withheld lane named %q", page.SourcesUnavailable, sourceMeeting)
	}
	if entry.Category == nil || *entry.Category != "meetings" {
		t.Errorf("the withheld meetings lane was filed under %v, want meetings", entry.Category)
	}
}

// A wider scope has no single calendar and no single next meeting.
func TestOnlyTheReadersOwnDayCarriesTheCalendar(t *testing.T) {
	horizon := &stubHorizon{calendar: crmcontracts.WorklistCalendarConnected, next: threeDaysOut()}

	page := readHorizonPage(t, horizonService(&stubMeetings{}, horizon), scopeAll)

	if page.Calendar != nil || page.NextMeeting != nil || horizon.asked {
		t.Errorf("scope=all carried calendar=%v next=%+v", page.Calendar, page.NextMeeting)
	}
}

// A failed read leaves the field unknown and says so; a refused one is silent,
// because the refusal is the seat's standing fact rather than news about today.
func TestAFailedCalendarReadIsNamedAndARefusedOneIsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		named bool
	}{
		{"failed", errors.New("connection reset"), true},
		{"refused", apperrors.ErrPermissionDenied, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			horizon := &stubHorizon{calendarErr: tc.err}

			page := readHorizonPage(t, horizonService(&stubMeetings{}, horizon), scopeMine)

			if page.Calendar != nil {
				t.Errorf("calendar = %v after a %s read, want absent", *page.Calendar, tc.name)
			}
			entry, named := unavailableEntry(page, sourceCalendar)
			if named != tc.named {
				t.Fatalf("calendar named unavailable = %v, want %v: %+v", named, tc.named, page.SourcesUnavailable)
			}
			if named && entry.Reason != crmcontracts.WorklistSourceUnavailableReasonFailed {
				t.Errorf("reason = %q, want failed", entry.Reason)
			}
		})
	}
}

// A failed next-meeting read leaves the field unknown and names it under the
// meetings reading, so an empty month is never claimed over a read that broke.
func TestAFailedNextMeetingReadIsNamedUnderMeetings(t *testing.T) {
	horizon := &stubHorizon{calendar: crmcontracts.WorklistCalendarConnected, nextErr: errors.New("statement timeout")}

	page := readHorizonPage(t, horizonService(&stubMeetings{}, horizon), scopeMine)

	if page.NextMeeting != nil {
		t.Errorf("next_meeting = %+v after a failed read, want absent", page.NextMeeting)
	}
	entry, named := unavailableEntry(page, sourceNextMeeting)
	if !named {
		t.Fatalf("sources_unavailable = %+v, want %q named", page.SourcesUnavailable, sourceNextMeeting)
	}
	if entry.Reason != crmcontracts.WorklistSourceUnavailableReasonFailed {
		t.Errorf("reason = %q, want failed", entry.Reason)
	}
	if entry.Category == nil || *entry.Category != "meetings" {
		t.Errorf("category = %v, want meetings", entry.Category)
	}
}

// A side read puts no row on the queue, so its failure leaves the queue's
// figures exact. A lane that did not answer may hide rows and floors them.
func TestOnlyAMissingLaneMakesTheQueueFiguresAFloor(t *testing.T) {
	broken := errors.New("connection reset")
	for _, tc := range []struct {
		name     string
		meetings *stubMeetings
		horizon  *stubHorizon
		source   string
		lane     bool
	}{
		{"calendar read", &stubMeetings{}, &stubHorizon{calendarErr: broken}, sourceCalendar, false},
		{"next-meeting read", &stubMeetings{}, &stubHorizon{nextErr: broken}, sourceNextMeeting, false},
		{"withheld meetings lane", &stubMeetings{err: apperrors.ErrPermissionDenied}, &stubHorizon{}, sourceMeeting, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := readHorizonPage(t, horizonService(tc.meetings, tc.horizon), scopeMine)

			entry, named := unavailableEntry(page, tc.source)
			if !named {
				t.Fatalf("sources_unavailable = %+v, want %q named", page.SourcesUnavailable, tc.source)
			}
			if entry.ContributesRows == nil || *entry.ContributesRows != tc.lane {
				t.Errorf("contributes_rows = %v, want %v", entry.ContributesRows, tc.lane)
			}
			if page.Readings.MoreAvailable != tc.lane {
				t.Errorf("readings.more_available = %v with only the %s missing, want %v",
					page.Readings.MoreAvailable, tc.name, tc.lane)
			}
		})
	}
}

func unavailableEntry(page crmcontracts.Worklist, source string) (crmcontracts.WorklistSourceUnavailable, bool) {
	for _, entry := range page.SourcesUnavailable {
		if entry.Source == source {
			return entry, true
		}
	}
	return crmcontracts.WorklistSourceUnavailable{}, false
}
