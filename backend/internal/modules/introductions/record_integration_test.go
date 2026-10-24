// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

// Recording an introduction that already happened, against a real database:
// the write shape, who may record, who may read, and that no lifecycle writer
// touches the row afterwards.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func (e *introEnv) recording() NewRecorded {
	return NewRecorded{
		ContactID:      e.contact,
		IntroducerUser: e.introducer,
		IntroducedUser: e.requester,
		On:             testNow.AddDate(0, 0, -3),
		Channel:        "in_person",
	}
}

func (e *introEnv) recorded(t *testing.T) ids.UUID {
	t.Helper()
	id, err := e.store.Record(e.asUser(e.introducer), e.recording())
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return id
}

func countWhere(t *testing.T, e *introEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Either colleague can record it, and each recording is one introduced row
// with no ask fields, one audit row and one recorded event.
func TestEitherColleagueRecordsAnIntroductionInTheWriteShape(t *testing.T) {
	e := setupIntro(t)
	byIntroducer := e.recorded(t)

	other := e.recording()
	other.ContactID = e.unseen
	byIntroduced, err := e.store.Record(e.asUser(e.requester), other)
	if err != nil {
		t.Fatalf("the introduced colleague could not record it: %v", err)
	}

	for _, id := range []ids.UUID{byIntroducer, byIntroduced} {
		var origin, status, channel, capturedBy string
		var reason *string
		var due, introducedAt *time.Time
		if err := e.owner.QueryRow(context.Background(), `
			SELECT origin, status, recorded_channel, captured_by, internal_reason, due_at, introduced_at
			  FROM intro_request WHERE id = $1`, id).
			Scan(&origin, &status, &channel, &capturedBy, &reason, &due, &introducedAt); err != nil {
			t.Fatal(err)
		}
		if origin != "recorded" || status != "introduced" || channel != "in_person" {
			t.Errorf("row is %s/%s/%s; want recorded/introduced/in_person", origin, status, channel)
		}
		if reason != nil || due != nil {
			t.Errorf("a recorded row carries reason %v and due %v", reason, due)
		}
		wantDay := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
		if introducedAt == nil || !introducedAt.Equal(wantDay) {
			t.Errorf("introduced_at is %v; want the day %v", introducedAt, wantDay)
		}
		if capturedBy == "" || capturedBy == "test" {
			t.Errorf("captured_by is %q; want the signed-in colleague", capturedBy)
		}
		if n := auditRowsFor(t, e, id); n != 1 {
			t.Errorf("%d audit rows; want one", n)
		}
	}
	if n := countWhere(t, e,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.recorded'`); n != 2 {
		t.Errorf("%d recorded events; want two", n)
	}
	if n := countWhere(t, e,
		`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'intro_request.completed'`); n != 0 {
		t.Errorf("%d completed events; recording must not look like an ask being marked", n)
	}
}

func TestOnlyTheTwoColleaguesMayRecordAndNothingElseIsWritten(t *testing.T) {
	e := setupIntro(t)
	agent := principal.WithActor(e.asUser(e.introducer), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:x", UserID: e.introducer,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"introduction": {Create: true, Read: true},
				"contact":      {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	readOnly := principal.WithActor(e.asUser(e.introducer), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.introducer.String(), UserID: e.introducer,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"introduction": {Read: true},
				"contact":      {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	cases := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"a third colleague", e.asUser(e.stranger), apperrors.ErrPermissionDenied},
		{"an agent holding the introducer's id", agent, apperrors.ErrPermissionDenied},
		{"a role without the create grant", readOnly, apperrors.ErrPermissionDenied},
	}
	for _, c := range cases {
		if _, err := e.store.Record(c.ctx, e.recording()); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v; want %v", c.name, err, c.want)
		}
	}
	if n := countWhere(t, e, `SELECT count(*) FROM intro_request`); n != 0 {
		t.Errorf("%d rows written by refused callers", n)
	}
	// The permitted positive, so the refusals above are not a store that refuses everyone.
	e.recorded(t)
}

func TestARecordingNamesRealLiveColleaguesAndARealDay(t *testing.T) {
	e := setupIntro(t)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.stranger); err != nil {
		t.Fatal(err)
	}
	departed := e.recording()
	departed.IntroducedUser = e.stranger
	unknown := e.recording()
	unknown.IntroducedUser = ids.NewV7()
	same := e.recording()
	same.IntroducedUser = e.introducer
	future := e.recording()
	future.On = testNow.AddDate(0, 0, 2)
	badChannel := e.recording()
	badChannel.Channel = "telepathy"

	for name, in := range map[string]NewRecorded{
		"a departed colleague": departed, "an unknown colleague": unknown,
		"the same colleague twice": same, "a future date": future, "an unknown channel": badChannel,
	} {
		_, err := e.store.Record(e.asUser(e.introducer), in)
		if _, ok := errors.AsType[*values.ParseError](err); !ok {
			t.Errorf("%s: got %v; want a validation error", name, err)
		}
	}
	today := e.recording()
	today.On = testNow
	if _, err := e.store.Record(e.asUser(e.introducer), today); err != nil {
		t.Errorf("an introduction from today was refused: %v", err)
	}
}

func TestARecordingNeedsAReadableLiveContactAndRefusesADuplicate(t *testing.T) {
	e := setupIntro(t)
	missing := e.recording()
	missing.ContactID = ids.NewV7()
	if _, err := e.store.Record(e.asUser(e.introducer), missing); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a missing contact gave %v; want not-found", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE contact SET archived_at = now() WHERE id = $1`, e.unseen); err != nil {
		t.Fatal(err)
	}
	archived := e.recording()
	archived.ContactID = e.unseen
	if _, err := e.store.Record(e.asUser(e.introducer), archived); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an archived contact gave %v; want not-found", err)
	}

	e.recorded(t)
	if _, err := e.store.Record(e.asUser(e.requester), e.recording()); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("recording the same pair twice gave %v; want a conflict", err)
	}
}

// A recorded introduction is a fact two colleagues vouched for, so a colleague
// who is neither can read it; an ask between the same two stays private.
func TestAnyReaderOfTheContactSeesARecordedIntroductionButNotAnAsk(t *testing.T) {
	e := setupIntro(t)
	recorded := e.recorded(t)
	ask, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}

	rows, err := e.store.ForContact(e.asUser(e.stranger), e.contact, listCap)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != recorded || rows[0].Origin != OriginRecorded {
		t.Errorf("a third colleague sees %d rows; want only the recorded introduction", len(rows))
	}
	if _, err := e.store.Get(e.asUser(e.stranger), recorded); err != nil {
		t.Errorf("a third colleague could not open the recorded introduction: %v", err)
	}
	if _, err := e.store.Get(e.asUser(e.stranger), ask); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a third colleague opening an ask gave %v; want not-found", err)
	}
}

// Every writer keyed on status must leave a recorded row alone: it is
// `introduced` from birth, which is the status the expiry sweep, the reply
// consumer and the lifecycle each act on.
func TestNoLifecycleWriterMovesARecordedIntroduction(t *testing.T) {
	e := setupIntro(t)
	id := e.recorded(t)
	versionBefore := countWhere(t, e, `SELECT version FROM intro_request WHERE id = $1`, id)

	e.store.now = func() time.Time { return testNow.AddDate(5, 0, 0) }
	if n, err := e.store.ExpireDue(e.asClock()); err != nil || n != 0 {
		t.Errorf("the sweep expired %d rows (%v); want none", n, err)
	}
	replied, err := e.store.RecordReply(e.asCapture(), id, e.evidence(t), testNow.Add(time.Hour))
	if err != nil || replied {
		t.Errorf("a captured reply moved a recorded row (%v, %v)", replied, err)
	}
	pending, err := e.store.AwaitingReply(e.asCapture(), e.contact)
	if err != nil || len(pending) != 0 {
		t.Errorf("a recorded row awaits a reply (%d, %v)", len(pending), err)
	}
	for name, err := range map[string]error{
		"cancel":   e.store.Cancel(e.asUser(e.requester), id, "", 1),
		"decide":   e.store.Decide(e.asUser(e.introducer), id, StatusDeclined, "", nil, 1),
		"complete": e.store.Complete(e.asUser(e.introducer), id, nil, 1),
	} {
		if !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("%s on a recorded row gave %v; want a conflict", name, err)
		}
	}

	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM intro_request WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "introduced" || countWhere(t, e, `SELECT version FROM intro_request WHERE id = $1`, id) != versionBefore {
		t.Errorf("the recorded row moved to %q", status)
	}
}

// The table refuses a recorded row that is not a clean introduced fact, so a
// writer that forgets the module's rules still cannot store one.
func TestTheTableRefusesAMalformedRecordedRow(t *testing.T) {
	e := setupIntro(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO intro_request (contact_id, requester_user_id, introducer_user_id,
		       route_type, origin, recorded_channel, status, introduced_at, captured_by)
		VALUES ($1,$2,$3,'direct','recorded','email','introduced',now(),'test')`,
		e.contact, e.requester, e.introducer); err != nil {
		t.Fatalf("a clean recorded row was refused: %v", err)
	}
	for name, sql := range map[string]string{
		"a reason":      `UPDATE intro_request SET internal_reason = 'why' WHERE origin = 'recorded'`,
		"an open state": `UPDATE intro_request SET status = 'requested' WHERE origin = 'recorded'`,
		"a reply":       `UPDATE intro_request SET status = 'replied', replied_at = now() WHERE origin = 'recorded'`,
		"no channel":    `UPDATE intro_request SET recorded_channel = NULL WHERE origin = 'recorded'`,
	} {
		if _, err := e.owner.Exec(context.Background(), sql); err == nil {
			t.Errorf("the table accepted a recorded row with %s", name)
		}
	}
}
