// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// answeringCalendar records what an answer asked of the provider. The embedded
// interface stays nil: an answer that reached any other verb would panic.
type answeringCalendar struct {
	SchedulingCalendar
	host     ids.UserID
	uid      string
	response string
	refusal  error
}

func (c *answeringCalendar) Respond(_ context.Context, host ids.UserID, uid, response string) error {
	c.host, c.uid, c.response = host, uid, response
	return c.refusal
}

// emailedInvitation seeds an invitation the rep's mailbox received and the
// meeting their calendar synced from it, starting in startsIn.
func emailedInvitation(t *testing.T, e *loadEnv, startsIn time.Duration) (message, meeting ids.UUID, uid string) {
	t.Helper()
	message, meeting, uid = ids.NewV7(), ids.NewV7(), "kickoff-"+ids.NewV7().String()+"@google.com"
	now := clocktest.Now(t)
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by, calendar_method, calendar_uid)
		VALUES ($1, 'email', 'inbound', 'Invitation: Kickoff', $3, 'seed', 'system', 'REQUEST', $2)`,
		message, uid, now.Add(-time.Hour))
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, message, e.rep)
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Kickoff', $2, 'seed', 'system')`,
		meeting, now.Add(startsIn))
	e.exec(t, `INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ('meeting', $1, $2, 'seed')`, MeetingIdentityKey(uid, now.Add(startsIn).Format(time.RFC3339)), meeting)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, user_id, response)
		VALUES ($1, 'attendee', $2, 'needs_action')`, meeting, e.rep)
	return message, meeting, uid
}

func openInvitationsFor(t *testing.T, e *loadEnv, reader ids.UUID, message ids.UUID) map[ids.UUID]openInvitation {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	//craft:ignore swallowed-errors a read-only test transaction has nothing to keep
	defer func() { _ = tx.Rollback(ctx) }()
	open, err := openInvitationsTx(ctx, tx, []ids.UUID{message}, reader, clocktest.Now(t))
	if err != nil {
		t.Fatalf("reading open invitations: %v", err)
	}
	return open
}

// answerOverHTTP sends the answer through the route a client calls.
func answerOverHTTP(t *testing.T, handlers Handlers, ctx context.Context, message ids.UUID, response string) crmcontracts.EmailCalendarInvitation {
	t.Helper()
	body, err := json.Marshal(map[string]string{"response": response})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/calendar-response", bytes.NewReader(body)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	handlers.AnswerCalendarInvitation(recorder, request, openapi_types.UUID(message))
	var answer crmcontracts.EmailCalendarInvitation
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &answer) != nil {
		t.Fatalf("answering: %d %s", recorder.Code, recorder.Body.String())
	}
	return answer
}

func TestAnInvitationIsOpenOnlyToTheSeatItWasSentTo(t *testing.T) {
	e := setupLoad(t)
	message, _, _ := emailedInvitation(t, e, 24*time.Hour)
	mine := openInvitationsFor(t, e, e.rep, message)
	if got, ok := mine[message]; !ok || got.response != crmcontracts.EmailCalendarInvitationResponseNeedsAction || got.startsAt == nil {
		t.Errorf("the invited rep reads %+v (found %v), want an unanswered invitation with its start", got, ok)
	}
	if _, ok := openInvitationsFor(t, e, e.other, message)[message]; ok {
		t.Error("a colleague whose mailbox never received the invitation is offered an answer to it")
	}
}

func TestAnInvitationWhoseEventHasStartedAsksNothing(t *testing.T) {
	e := setupLoad(t)
	message, _, _ := emailedInvitation(t, e, -time.Hour)
	if _, ok := openInvitationsFor(t, e, e.rep, message)[message]; ok {
		t.Error("an invitation to a meeting that already began is still offered as open")
	}
}

func TestAnsweringAnInvitationRecordsItAndTakesItOffTheReadersList(t *testing.T) {
	e := setupLoad(t)
	message, meeting, uid := emailedInvitation(t, e, 24*time.Hour)
	calendar := &answeringCalendar{}
	store := handledStore(e).WithSchedulingCalendar(calendar).WithClock(func() time.Time { return clocktest.Now(t) })

	got := answerOverHTTP(t, Handlers{store: store}, e.asSeat(e.rep), message, "tentative")
	if got.Response != crmcontracts.EmailCalendarInvitationResponseTentative {
		t.Errorf("answered %q, want tentative", got.Response)
	}
	if calendar.uid != uid || calendar.response != "tentative" || calendar.host.UUID != e.rep {
		t.Errorf("the calendar was asked %+v, want the rep's tentative answer to %s", calendar, uid)
	}
	var stored string
	if err := e.owner.QueryRow(context.Background(), `SELECT response FROM activity_participant
		 WHERE activity_id = $1 AND user_id = $2`, meeting, e.rep).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "tentative" {
		t.Errorf("the meeting holds %q for the rep, want tentative", stored)
	}
	if audited, announced := handledTrail(t, e, message); audited != 1 || announced != 1 {
		t.Errorf("the answer left %d Done audit rows and %d marked_handled events, want one of each", audited, announced)
	}
	var answered int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'activity.invitation_answered' AND envelope->'entity'->>'id' = $1::text`,
		message.String()).Scan(&answered); err != nil {
		t.Fatal(err)
	}
	if answered != 1 {
		t.Errorf("%d activity.invitation_answered events, want one", answered)
	}
}

func TestAColleagueCannotAnswerSomebodyElsesInvitation(t *testing.T) {
	e := setupLoad(t)
	message, _, _ := emailedInvitation(t, e, 24*time.Hour)
	calendar := &answeringCalendar{}
	_, err := handledStore(e).WithSchedulingCalendar(calendar).WithClock(func() time.Time { return clocktest.Now(t) }).AnswerCalendarInvitation(e.asSeat(e.other),
		ids.From[ids.ActivityKind](message), crmcontracts.AnswerCalendarInvitationRequestResponseAccepted)
	var refusal *values.ParseError
	if !errors.As(err, &refusal) || refusal.Code != "not_an_open_invitation" {
		t.Errorf("a colleague's answer = %v, want not_an_open_invitation", err)
	}
	if calendar.uid != "" {
		t.Error("the colleague's refused answer still reached a calendar")
	}
}

// An invitation that asks us is answered on the calendar. The request pass
// files no reminder task for it, and a plain request beside it still gets one.
func TestAnInvitationThatAsksUsFilesNoReminderTask(t *testing.T) {
	e := setupLoad(t)
	invitation := seedEmailRequest(t, e, "Invitation: Kickoff", "meeting", OwedVerdictAsksUs)
	e.exec(t, `UPDATE activity SET calendar_method = 'REQUEST', calendar_uid = 'kickoff@google.com' WHERE id = $1`, invitation)
	request := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	if err := storeKnowing(e).CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for message, want := range map[ids.UUID]int{invitation: 0, request: 1} {
		var got int
		if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, message).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("message %s filed %d reminder tasks, want %d", message, got, want)
		}
	}
}

// A refused answer names what the reader can do, and records nothing.
func TestARefusedAnswerTellsTheReaderWhyAndRecordsNothing(t *testing.T) {
	cases := map[string]struct {
		refusal  error
		response crmcontracts.AnswerCalendarInvitationRequestResponse
		code     string
	}{
		"event on another calendar": {connector.ErrCalendarEventMissing, "accepted", "event_not_on_calendar"},
		"no calendar connected":     {connector.ErrAuthRejected, "accepted", codeRequired},
		"an answer with no meaning": {nil, "perhaps", "unknown_response"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupLoad(t)
			message, meeting, _ := emailedInvitation(t, e, 24*time.Hour)
			store := handledStore(e).WithSchedulingCalendar(&answeringCalendar{refusal: c.refusal}).WithClock(func() time.Time { return clocktest.Now(t) })
			_, err := store.AnswerCalendarInvitation(e.asSeat(e.rep), ids.From[ids.ActivityKind](message), c.response)
			if code := refusalCode(err); code != c.code {
				t.Errorf("answer = %v (code %q), want %q", err, code, c.code)
			}
			var stored string
			if err := e.owner.QueryRow(context.Background(), `SELECT response FROM activity_participant
				 WHERE activity_id = $1 AND user_id = $2`, meeting, e.rep).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if stored != "needs_action" {
				t.Errorf("a refused answer still stored %q on the meeting", stored)
			}
		})
	}
}

func refusalCode(err error) string {
	if parse, ok := errors.AsType[*values.ParseError](err); ok {
		return parse.Code
	}
	if scheduling, ok := errors.AsType[*SchedulingArgumentError](err); ok {
		return scheduling.Code
	}
	return ""
}

// The mail drawer offers the invited reader the answers, and nobody else.
func TestTheMailDrawerOffersAnswersOnlyToTheInvitedReader(t *testing.T) {
	e := setupLoad(t)
	message, _, _ := emailedInvitation(t, e, 24*time.Hour)
	store := storeKnowing(e).WithClock(func() time.Time { return clocktest.Now(t) })
	for reader, want := range map[ids.UUID]bool{e.rep: true, e.other: false} {
		drawer, err := store.GetEmailPresentation(e.asSeat(reader), ids.From[ids.ActivityKind](message), nil)
		if err != nil {
			t.Fatalf("reading the drawer: %v", err)
		}
		if got := drawer.Summary.CalendarInvitation != nil; got != want {
			t.Errorf("reader %s sees an invitation to answer: %v, want %v", reader, got, want)
		}
	}
}
