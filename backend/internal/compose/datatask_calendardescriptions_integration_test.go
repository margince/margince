// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The calendar description task renders meeting descriptions that capture
// stored as raw HTML before it rendered them. The fixtures are written as the
// old capture left them, since no writer produces that shape any more.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const calendarHeader = "Organizer: host@calendar-task.test\nAttendees: guest@calendar-task.test"

// seedStoredMeeting writes a meeting row as capture stored it, and returns
// its id. Every fixture meeting occurs at calendarMeetingAt.
func seedStoredMeeting(t *testing.T, e *integration.Env, capturedBy, body string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, source_system, source_id)
		VALUES ($1, 'meeting', 'Quarterly review', $2, $3, 'gcal:seed', $4, 'gcal', $5)`,
		id, body, calendarMeetingAt, capturedBy, "calendar-task-"+id.String())
	return id
}

// calendarMeetingAt is when every fixture meeting takes place.
var calendarMeetingAt = time.Date(2026, 9, 14, 9, 30, 0, 0, time.UTC)

func meetingBodyOf(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, id)
}

func runCalendarDescriptionTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func calendarDescriptionEstimate(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := calendarDescriptionsDataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestTheCalendarDescriptionTaskRendersOnlyStoredHTML(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")

	booking := seedStoredMeeting(t, e, "connector:gcal", calendarHeader+
		"\n\n<p>Join the <b>pricing</b> call</p><br><ul><li>Agenda: renewal</li></ul>")
	outlook := seedStoredMeeting(t, e, "connector:graphcal", calendarHeader+
		"\n\n<div><p>Dial-in on the <i>bridge</i></p></div>")
	credential := seedStoredMeeting(t, e, "connector:gcal", calendarHeader+
		"\n\n<p>password: <b>hunter2secret</b></p>")
	// An older cut ended the stored body inside a tag.
	cut := seedStoredMeeting(t, e, "connector:gcal", calendarHeader+
		"\n\n<div><p>Bring the contract</p><p class=\"no…")
	escaped := seedStoredMeeting(t, e, "connector:gcal", calendarHeader+
		"\n\n<p>Use &lt;b&gt; for bold</p>")
	// A bracketed link is plain text: capture never rendered it.
	plainLink := calendarHeader + "\n\nCall <https://meet.calendar-task.test/room>"
	plain := seedStoredMeeting(t, e, "connector:gcal", plainLink)
	headerOnly := seedStoredMeeting(t, e, "connector:gcal", calendarHeader)
	// A meeting a member logged by hand holds what they typed.
	typed := "Notes\n\n<b>keep my tags</b>"
	manual := seedStoredMeeting(t, e, "human:"+e.Rep1.String(), typed)
	edited := seedStoredMeeting(t, e, "connector:gcal", calendarHeader+"\n\n<p>Original agenda</p>")
	memberText := calendarHeader + "\n\n<p>My own <b>agenda</b></p>"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](edited), activities.UpdateActivityInput{Body: &memberText}); err != nil {
		t.Fatalf("a member editing the meeting body: %v", err)
	}

	if got := calendarDescriptionEstimate(ctx, t, e); got != 7 {
		t.Errorf("the estimate counts %d meetings, want the seven calendar bodies holding a bracket", got)
	}
	versions := map[ids.UUID]int{}
	for _, id := range []ids.UUID{booking, outlook, credential, cut, escaped, plain, headerOnly, manual, edited} {
		versions[id] = e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, id)
	}

	task := calendarDescriptionsDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runCalendarDescriptionTask(ctx, t, e, task)

	for name, c := range map[string]struct {
		id         ids.UUID
		has, lacks []string
	}{
		"booking page":  {booking, []string{"Join the pricing call", "Agenda: renewal"}, []string{"<p>", "<b>", "<li>"}},
		"Outlook":       {outlook, []string{"Dial-in on the bridge"}, []string{"<div>", "<i>"}},
		"credential":    {credential, []string{"password: " + credentialmask.Marker}, []string{"hunter2secret", "<b>"}},
		"cut mid-tag":   {cut, []string{"Bring the contract"}, []string{"<div>", "<p"}},
		"escaped angle": {escaped, []string{"Use <b> for bold"}, []string{"<p>", "&lt;"}},
	} {
		body := meetingBodyOf(t, e, c.id)
		if !strings.HasPrefix(body, calendarHeader+"\n\n") {
			t.Errorf("%s lost its organizer and attendee header: %q", name, body)
		}
		for _, want := range c.has {
			if !strings.Contains(body, want) {
				t.Errorf("%s body %q lacks %q", name, body, want)
			}
		}
		for _, unwanted := range c.lacks {
			if strings.Contains(body, unwanted) {
				t.Errorf("%s body %q still holds %q", name, body, unwanted)
			}
		}
		if got := e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, c.id); got != versions[c.id]+1 {
			t.Errorf("%s version %d, want %d", name, got, versions[c.id]+1)
		}
		if got := e.WsScalar(t, `SELECT to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS')
			FROM activity WHERE id = $1`, c.id); got != "2026-09-14T09:30:00" {
			t.Errorf("%s moved to %s", name, got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
			AND actor_type = 'system' AND after ? 'description_rendered' AND NOT after ? 'body'`, c.id); got != 1 {
			t.Errorf("%s render audit rows = %d, want one without the body", name, got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM event_outbox
			WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1`, c.id.String()); got != 1 {
			t.Errorf("%s activity.updated events = %d, want 1", name, got)
		}
	}
	for name, c := range map[string]struct {
		id   ids.UUID
		body string
	}{
		"bracketed link": {plain, plainLink}, "header only": {headerOnly, calendarHeader},
		"typed by hand": {manual, typed}, "edited by a member": {edited, memberText},
	} {
		if got := meetingBodyOf(t, e, c.id); got != c.body {
			t.Errorf("%s body changed to %q", name, got)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
			AND after ? 'description_rendered'`, c.id); got != 0 {
			t.Errorf("%s carries %d render audit rows", name, got)
		}
	}
	if got := calendarDescriptionEstimate(ctx, t, e); got != 2 {
		t.Errorf("after the run the estimate counts %d meetings, want the bracketed link and the member's edit", got)
	}
	if got := e.WsCount(t, `SELECT version FROM activity WHERE id = $1`, plain); got != versions[plain] {
		t.Errorf("the bracketed link meeting changed version to %d", got)
	}

	// The escaped meeting now reads "<b>" as text. A replay from the start
	// must not render that text again.
	rendered := meetingBodyOf(t, e, escaped)
	step := calendarDescriptionsDataTask().Steps[0]
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay rendered %d more meetings", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
	if got := meetingBodyOf(t, e, escaped); got != rendered {
		t.Errorf("a replay changed the rendered text from %q to %q", rendered, got)
	}
}

func TestTheCalendarDescriptionTaskRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	stored := calendarHeader + "\n\n<p>Rollback agenda</p>"
	meeting := seedStoredMeeting(t, e, "connector:gcal", stored)

	task := calendarDescriptionsDataTask()
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runCalendarDescriptionTask(ctx, t, e, task)

	if got := meetingBodyOf(t, e, meeting); got != stored {
		t.Errorf("the render survived a rolled-back batch: %q", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`, meeting); got != 0 {
		t.Errorf("render audit rows escaped the rollback: %d", got)
	}
}
