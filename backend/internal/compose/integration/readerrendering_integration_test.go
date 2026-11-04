// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Showing a message in the reader's language, over the real content read with
// the model stubbed at its one boundary, the lane's Complete.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/readerrendering"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// renderingLane answers one English rendering and counts its calls.
type renderingLane struct {
	calls int
	// during runs while the model "writes", to move the row under the call.
	during func()
}

func (l *renderingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	if l.during != nil {
		l.during()
	}
	return model.Response{ServedModel: "stub-model", Text: `{"already_in_target": false,
	  "subject": "Renewal", "body": "We talked about the renewal. They want to see the offer by Friday."}`}, nil
}

const renderingNoteBody = "Wir haben über die Verlängerung gesprochen. Sie möchten das Angebot bis Freitag sehen."

func logGermanActivity(ctx context.Context, t *testing.T, e *Env, kind string, contact ids.UUID) ids.ActivityID {
	t.Helper()
	subject, body := "Verlängerung", renderingNoteBody
	logged, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: kind, Subject: &subject, Body: &body, Direction: new("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log %s: %v", kind, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

// renderingLedgers counts what a rendering must never write.
func renderingLedgers(t *testing.T, e *Env) [3]int {
	t.Helper()
	return [3]int{
		e.WsCount(t, `SELECT count(*) FROM audit_log`),
		e.WsCount(t, `SELECT count(*) FROM event_outbox`),
		e.WsCount(t, `SELECT count(*) FROM ai_call_payload`),
	}
}

func renderingClock(t *testing.T) func() time.Time {
	now := clocktest.Now(t)
	return func() time.Time { return now }
}

func asReader(ctx context.Context, locale string) context.Context {
	actor, _ := principal.Actor(ctx)
	actor.Locale = locale
	return principal.WithActor(ctx, actor)
}

func TestTheAuthorSeesTheirNoteInTheirLanguageAndNothingIsWritten(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Lena Vogt", &e.Rep1)
	id := logGermanActivity(author, t, e, "note", contact)
	lane := &renderingLane{}
	svc := readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).WithLane(lane)

	before := renderingLedgers(t, e)
	got, err := svc.Render(asReader(author, "en"), id, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got.Unchanged || got.Body == nil || got.SourceLanguage == nil || *got.SourceLanguage != "de" || lane.calls != 1 {
		t.Fatalf("rendering = %+v after %d calls, want an English rendering of a note stored as de", got, lane.calls)
	}
	if after := renderingLedgers(t, e); after != before {
		t.Errorf("audit, outbox and payload counts moved from %v to %v; a rendering writes nothing", before, after)
	}

	unchanged, err := svc.Render(asReader(author, "de"), id, nil)
	if err != nil || !unchanged.Unchanged || lane.calls != 1 {
		t.Errorf("a German reader got %+v, %v after %d calls, want the original with no model call", unchanged, err, lane.calls)
	}
	if _, err := readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).Render(asReader(author, "en"), id, nil); !errors.Is(err, readerrendering.ErrNoModel) {
		t.Errorf("with no lane bound: %v, want ErrNoModel", err)
	}
	if _, err := svc.Render(e.AgentCtx(), id, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent asked for a rendering: %v, want ErrPermissionDenied", err)
	}
	if lane.calls != 1 {
		t.Errorf("the refusals above reached the model %d times", lane.calls-1)
	}
}

func TestATextTheCallerMayNotReadIsNotFoundAndNeverReachesTheModel(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Lena Vogt", &e.Rep1)
	lane := &renderingLane{}
	svc := readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).WithLane(lane)

	limited := logGermanActivity(author, t, e, "email", contact)
	if _, err := e.Activities.SetAudience(author, limited, activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}
	private := e.SeedContact(t, "Private Contact", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	outOfScope := logGermanActivity(author, t, e, "note", private)
	archived := logGermanActivity(author, t, e, "note", contact)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}

	// The author reads both, so a refusal below is the colleague's and not the row's.
	for _, readable := range []ids.ActivityID{limited, outOfScope} {
		if _, err := svc.Render(asReader(author, "en"), readable, nil); err != nil {
			t.Fatalf("the author's own rendering: %v", err)
		}
	}
	lane.calls = 0

	for name, read := range map[string]struct {
		ctx context.Context
		id  ids.ActivityID
	}{
		"a colleague outside a limited audience": {colleague, limited},
		"a colleague outside the record's scope": {colleague, outOfScope},
		"the author of an archived note":         {author, archived},
	} {
		if _, err := svc.Render(asReader(read.ctx, "en"), read.id, nil); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: %v, want ErrNotFound", name, err)
		}
	}
	if lane.calls != 0 {
		t.Errorf("text the caller may not read reached the model %d times", lane.calls)
	}
}

func TestAMessageHeldUnderARetentionObligationIsNotFound(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), f.contact, "test"); err != nil {
		t.Fatalf("erasing the subject, which holds the mail: %v", err)
	}
	lane := &renderingLane{}
	svc := readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).WithLane(lane)
	if _, err := svc.Render(asReader(e.Admin(), "de"), ids.From[ids.ActivityKind](f.email), nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a held mail: %v, want ErrNotFound", err)
	}
	if lane.calls != 0 {
		t.Error("a held mail reached the model")
	}
}

func TestARowThatMovedWhileTheModelWroteShowsNoRendering(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Lena Vogt", &e.Rep1)

	limited := logGermanActivity(author, t, e, "email", contact)
	limit := &renderingLane{during: func() {
		if _, err := e.Activities.SetAudience(author, limited, activities.SetAudienceInput{Audience: "participants"}); err != nil {
			t.Errorf("limiting during the call: %v", err)
		}
	}}
	svc := readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).WithLane(limit)
	if got, err := svc.Render(asReader(colleague, "en"), limited, nil); !errors.Is(err, apperrors.ErrNotFound) || got.Body != nil {
		t.Errorf("a mail limited while the model wrote: %+v, %v, want ErrNotFound", got, err)
	}

	archived := logGermanActivity(author, t, e, "note", contact)
	archive := &renderingLane{during: func() {
		if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
			t.Errorf("archiving during the call: %v", err)
		}
	}}
	svc = readerrendering.NewService(e.Pool, e.Activities, renderingClock(t)).WithLane(archive)
	if got, err := svc.Render(asReader(author, "en"), archived, nil); !errors.Is(err, apperrors.ErrNotFound) || got.Body != nil {
		t.Errorf("a note archived while the model wrote: %+v, %v, want ErrNotFound", got, err)
	}
	if limit.calls != 1 || archive.calls != 1 {
		t.Errorf("the model was asked %d and %d times, want once each, before the row moved", limit.calls, archive.calls)
	}
}
