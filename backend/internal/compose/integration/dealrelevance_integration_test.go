// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A colleague's word that an activity on a deal is not about it. The meetings
// land through the calendar sink, and the mark goes through the activities
// store. Every reading of whether the deal is alive leaves a marked row out.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// dealMarkPerms is a rep who may change activities and deals in their team.
var dealMarkPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":  {Read: true},
		"company":  {Read: true},
		"project":  {Read: true},
		"deal":     {Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

// owner is the deal's owner, Rep1, with the rights a mark needs.
func (w dealEvidenceWorld) owner() context.Context {
	return w.e.As(w.e.Rep1, []ids.UUID{w.e.Team1}, dealMarkPerms)
}

// mark sets the link's mark as the deal's owner, failing on any refusal.
func (w dealEvidenceWorld) mark(t *testing.T, activity ids.UUID, relevance string) {
	t.Helper()
	if _, err := w.e.Activities.SetDealRelevance(w.owner(), ids.From[ids.ActivityKind](activity), w.deal.UUID, relevance); err != nil {
		t.Fatalf("marking %s %s: %v", activity, relevance, err)
	}
}

// markOf is the link's mark and its source, or "" for a link nobody judged.
func (w dealEvidenceWorld) markOf(t *testing.T, activity ids.UUID) string {
	t.Helper()
	return w.e.WsScalar(t, `SELECT coalesce(deal_relevance || '/' || relevance_source, '') FROM activity_link
		WHERE activity_id = $1 AND deal_id = $2`, activity, w.deal.UUID)
}

func (w dealEvidenceWorld) wantClock(t *testing.T, want time.Time, why string) {
	t.Helper()
	if clock := w.dealClock(t); clock == nil || !clock.Equal(want) {
		t.Fatalf("deal last_activity_at = %v %s, want %v", clock, why, want)
	}
}

// A dinner marked not about the deal stops warming it. The clock moves back to
// the business meeting, a forum ahead defers nothing, and health cites it. The mark is one audited write with its event. Undo restores all.
func TestAMeetingMarkedNotAboutTheDealStopsWarmingIt(t *testing.T) {
	w := newDealEvidenceWorld(t)
	reviewAt, dinnerAt := w.now.AddDate(0, 0, -10), w.now.AddDate(0, 0, -2)
	review := w.meeting(t, "budget review", reviewAt, false)
	dinner := w.meeting(t, "dinner", dinnerAt, false)
	forum := w.meeting(t, "peer forum", w.now.AddDate(0, 0, 5), false)
	w.wantClock(t, dinnerAt, "before any mark")
	if !w.meetingAhead(t) {
		t.Fatal("the forum ahead does not defer the deal before the mark")
	}

	w.mark(t, dinner, "unrelated")
	w.mark(t, forum, "unrelated")
	if got := w.markOf(t, dinner); got != "unrelated/human" {
		t.Fatalf("the dinner's link is marked %q, want unrelated/human", got)
	}
	w.wantClock(t, reviewAt, "after the dinner was marked")
	if w.meetingAhead(t) {
		t.Error("a forum marked not about the deal still defers it")
	}
	if cited := w.healthEvidence(t); cited == nil || *cited != review {
		t.Errorf("deal health cites %v, want the budget review %s", cited, review)
	}
	if n := w.e.WsCount(t, `SELECT count(*) FROM audit_log au
		JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = au.id::text
		WHERE au.entity_id = $1 AND au.action = 'activity_relink' AND au.actor_type = 'human'
		  AND au.after->>'deal_relevance' = 'unrelated' AND au.after->>'declined' = 'true'`, dinner); n != 1 {
		t.Errorf("%d audited marks with their event, want one", n)
	}

	w.mark(t, dinner, "relevant")
	w.mark(t, forum, "relevant")
	w.wantClock(t, dinnerAt, "after the undo")
	if !w.meetingAhead(t) {
		t.Error("the forum marked about the deal again defers nothing")
	}
	if got := w.markOf(t, dinner); got != "relevant/human" {
		t.Errorf("the dinner's link is marked %q after the undo, want relevant/human", got)
	}
}

// A link the classifier is unsure of keeps the deal no warmer than one marked
// unrelated. A colleague's "about this deal" makes it count.
func TestALinkTheClassifierDoubtsDoesNotWarmTheDeal(t *testing.T) {
	w := newDealEvidenceWorld(t)
	reviewAt, dinnerAt := w.now.AddDate(0, 0, -10), w.now.AddDate(0, 0, -2)
	w.meeting(t, "budget review", reviewAt, false)
	dinner := w.meeting(t, "dinner", dinnerAt, false)
	system := principal.SystemActing(w.e.Admin(), "system:dealrelevance-test")
	if err := database.WithTx(system, w.e.Pool, func(tx pgx.Tx) error {
		changed, err := activities.MarkDealLinkTx(system, tx, ids.From[ids.ActivityKind](dinner), w.deal.UUID, "uncertain", nil)
		if err == nil && !changed {
			t.Error("the classifier's first mark changed nothing")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.wantClock(t, reviewAt, "with the dinner uncertain")
	w.mark(t, dinner, "relevant")
	w.wantClock(t, dinnerAt, "once a colleague said it is about the deal")
}

// A second pull of the same calendar entry is a re-import. It keeps the
// colleague's mark and adds no second link.
func TestAReimportKeepsAColleaguesMark(t *testing.T) {
	w := newDealEvidenceWorld(t)
	reviewAt := w.now.AddDate(0, 0, -10)
	w.meeting(t, "budget review", reviewAt, false)
	entry := CalendarMeeting{
		Event: "dinner-" + ids.NewV7().String(), At: w.now.AddDate(0, 0, -2), Seat: w.e.Rep1, DeliveredTo: w.calendar,
		Links: []datasource.EntityRef{{Type: datasource.EntityDeal, ID: w.deal.UUID}, {Type: datasource.EntityContact, ID: w.contact}},
	}
	dinner := entry.Capture(t, w.e)
	w.mark(t, dinner, "unrelated")
	if again := entry.Capture(t, w.e); again != dinner {
		t.Fatalf("the re-import landed %s, want the same row %s", again, dinner)
	}
	if got := w.markOf(t, dinner); got != "unrelated/human" {
		t.Errorf("the re-imported dinner is marked %q, want unrelated/human", got)
	}
	if n := w.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, dinner); n != 1 {
		t.Errorf("the re-imported dinner has %d deal links, want one", n)
	}
	w.wantClock(t, reviewAt, "after the re-import")
}

// The mark and its undo answer by the caller's rights. Three callers are
// refused, and none changes the mark. One may change the activity but not the
// deal, one lacks the deal grant, and one sits in another workspace.
func TestADealMarkAnswersByTheCallersRights(t *testing.T) {
	w := newDealEvidenceWorld(t)
	w.contact = w.e.SeedContact(t, "Ada Shared", &w.e.Rep3)
	dinner := w.meeting(t, "dinner", w.now.AddDate(0, 0, -2), false)

	otherTeam := w.e.As(w.e.Rep3, []ids.UUID{w.e.Team2}, dealMarkPerms)
	elsewhere, ws := w.otherWorkspace(t)
	noDealUpdate := dealMarkPerms
	noDealUpdate.Objects = map[string]principal.ObjectGrant{
		"contact": {Read: true}, "deal": {Read: true}, "activity": {Read: true, Update: true},
	}
	refused := map[string]struct {
		ctx  context.Context
		want error
	}{
		"a seat that may not change the deal":  {otherTeam, apperrors.ErrPermissionDenied},
		"a seat without the deal update grant": {w.e.As(w.e.Rep1, []ids.UUID{w.e.Team1}, noDealUpdate), apperrors.ErrPermissionDenied},
		"a seat of another workspace":          {elsewhere, apperrors.ErrPermissionDenied},
	}
	// Each refusal is asked of the mark and of its undo, and leaves the link as it was.
	for _, step := range []struct{ relevance, before string }{{"unrelated", ""}, {"relevant", "unrelated/human"}} {
		if step.relevance == "relevant" {
			w.mark(t, dinner, "unrelated")
		}
		for name, c := range refused {
			store := w.e.Activities
			if c.ctx == elsewhere {
				store = activities.NewStore(w.e.DBFor(ws))
			}
			_, err := store.SetDealRelevance(c.ctx, ids.From[ids.ActivityKind](dinner), w.deal.UUID, step.relevance)
			if !errors.Is(err, c.want) {
				t.Errorf("%s, %s: the mark answered %v, want %v", name, step.relevance, err, c.want)
			}
		}
		if got := w.markOf(t, dinner); got != step.before {
			t.Fatalf("a refused %s mark left the link marked %q, want %q", step.relevance, got, step.before)
		}
	}
	if _, err := w.e.Activities.SetDealRelevance(w.owner(), ids.From[ids.ActivityKind](dinner), ids.NewV7(), "unrelated"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a deal the activity has no link to answered %v, want not found", err)
	}
}

// otherWorkspace is a rep of a second workspace, seeded beside this one, and
// that workspace. Tables carry no workspace, so the rep's row scope is what
// keeps the deal from them.
func (w dealEvidenceWorld) otherWorkspace(t *testing.T) (context.Context, ids.UUID) {
	t.Helper()
	ws, user := ids.NewV7(), ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding the second workspace: %v", err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, Permissions: dealMarkPerms,
	}), ws
}

// A deal link inserted already marked moves the deal clock no further, though
// the activity's contact link would. The insert's advance reads that one link.
func TestADealLinkInsertedAlreadyMarkedMovesNoClock(t *testing.T) {
	w := newDealEvidenceWorld(t)
	dinner := CalendarMeeting{
		Event: "dinner-" + ids.NewV7().String(), At: w.now.AddDate(0, 0, -2), Seat: w.e.Rep1, DeliveredTo: w.calendar,
		Links: []datasource.EntityRef{{Type: datasource.EntityContact, ID: w.contact}},
	}.Capture(t, w.e)
	for _, relevance := range []string{"unrelated", "uncertain"} {
		w.e.WsExec(t, `DELETE FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`, dinner)
		w.e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id, deal_relevance, relevance_source)
			VALUES ($1, 'deal', $2, $3, 'classifier')`, dinner, w.deal.UUID, relevance)
		if clock := w.dealClock(t); clock != nil {
			t.Errorf("a deal link inserted %s moved the deal clock to %v, want none", relevance, clock)
		}
	}
}
