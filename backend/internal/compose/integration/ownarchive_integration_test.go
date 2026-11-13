// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ownArchivePerms is the rep's seeded shape on the archived objects: write
// without delete. Team scope makes a teammate's records writable, so a refusal
// below comes from the archive rule and not from the row write gate.
var ownArchivePerms = principal.Permissions{
	RoleKeys: []string{roleRep},
	Objects: map[string]principal.ObjectGrant{
		objContact:         {Create: true, Read: true, Update: true},
		objDeal:            {Create: true, Read: true, Update: true},
		"activity":         {Create: true, Read: true, Update: true},
		objPipeline:        {Read: true},
		objRelationship:    {Create: true, Read: true, Update: true},
		objInstallSettings: {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// asTeam1Member is a Team1 member on a full seat, which the read's writable and
// archivable flags ask for before any grant.
func asTeam1Member(e *Env, user ids.UUID, perms principal.Permissions) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs: []ids.UUID{e.Team1}, SeatType: principal.SeatFull, Permissions: perms,
	})
}

func logOwnActivity(as context.Context, t *testing.T, e *Env, kind string, on ids.UUID) ids.ActivityID {
	t.Helper()
	subject := "Spoke on LinkedIn"
	row, _, err := e.Activities.LogActivity(as, activities.LogActivityInput{
		Kind: kind, Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: on}},
	})
	if err != nil {
		t.Fatalf("logging a %s: %v", kind, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(row.Id))
}

func archiveWroteTheShape(t *testing.T, e *Env, entity ids.UUID, eventType string) {
	t.Helper()
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'archive'`, entity); n != 1 {
		t.Errorf("archive audit rows for %s = %d, want 1", entity, n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = $2 AND envelope->'entity'->>'id' = $1::text`,
		entity, eventType); n != 1 {
		t.Errorf("%s events for %s = %d, want 1", eventType, entity, n)
	}
}

func TestAMemberArchivesTheNotesTasksCallsAndMeetingsTheyLogged(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)

	for _, kind := range []string{"note", "task", "meeting", "call"} {
		own := logOwnActivity(rep, t, e, kind, mine)
		read, err := e.Activities.GetActivity(rep, own, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if !*read.Archivable {
			t.Errorf("my own hand-logged %s reads as not archivable", kind)
		}
		if _, err := e.Activities.ArchiveActivity(rep, own, nil); err != nil {
			t.Errorf("archiving my own hand-logged %s → %v, want allowed", kind, err)
			continue
		}
		archiveWroteTheShape(t, e, own.UUID, "activity.archived")
	}
}

func TestAMemberCannotArchiveAColleaguesHandLoggedMeeting(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)
	theirs := logOwnActivity(asTeam1Member(e, e.Rep2, ownArchivePerms), t, e, "meeting", mine)

	read, err := e.Activities.GetActivity(rep, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !*read.Writable || *read.Archivable {
		t.Errorf("teammate's meeting on my contact: writable=%v archivable=%v, want true/false", *read.Writable, *read.Archivable)
	}
	if _, err := e.Activities.ArchiveActivity(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a teammate's hand-logged meeting → %v, want ErrPermissionDenied", err)
	}
}

func TestCapturedEntriesStayTheManagersToArchive(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)
	mail := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Re: terms', 'body', now(), 'gmail', 'connector:gmail:`+e.Rep1.String()+`')`)
	LinkActivity(t, OwnerConn(t), mail, objContact, mine)
	hosted := CalendarMeeting{
		Event: "rep-hosted-" + ids.NewV7().String(), At: clocktest.Now(t).Add(-time.Hour), Seat: e.Rep1,
		Links: []datasource.EntityRef{{Type: datasource.EntityContact, ID: mine}},
	}.Capture(t, e)
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND host_user_id = $2`, hosted, e.Rep1); n != 1 {
		t.Fatalf("the synced meeting is not hosted by the rep (%d rows)", n)
	}

	manager := asTeam1Member(e, e.Rep1, withActivityDelete(ownArchivePerms))
	for name, id := range map[string]ids.UUID{"captured mail": mail, "calendar-synced meeting they host": hosted} {
		activity := ids.From[ids.ActivityKind](id)
		if _, err := e.Activities.ArchiveActivity(rep, activity, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a rep archiving the %s → %v, want ErrPermissionDenied", name, err)
		}
		if _, err := e.Activities.ArchiveActivity(manager, activity, nil); err != nil {
			t.Errorf("a seat holding activity.delete archiving the %s → %v, want allowed", name, err)
		}
	}
}

// A calendar copy that folds onto a hand-logged meeting brings its import
// with it. The meeting is then the provider's record too.
func TestAHandLoggedMeetingTheCalendarCopyFoldedOntoIsNotTheAuthorsToArchive(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	contact := e.SeedContact(t, "Fold Buyer", &e.Rep1)
	at := clocktest.Now(t).UTC().Add(-time.Hour).Truncate(time.Second)
	series := "fold-" + ids.NewV7().String() + "@calendar.test"
	address := "rep-" + ids.NewV7().String() + "@fold.test"
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, account_label) VALUES ($1, $2, 'connected', $3)`,
		CalendarSystem, e.Rep1, address)
	calendar := CalendarMeeting{
		Event: "fold-" + ids.NewV7().String(), At: at, Seat: e.Rep1, DeliveredTo: address,
		Links: []datasource.EntityRef{{Type: datasource.EntityContact, ID: contact}},
	}
	copyID := calendar.Capture(t, e)

	subject := "Quarterly review"
	logged, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
		ICalUID: series, ICalInstance: at.Format(time.RFC3339),
		Links: []activities.ActivityLinkInput{{EntityType: objContact, EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the meeting: %v", err)
	}
	meeting := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	read, err := e.Activities.GetActivity(rep, meeting, storekit.LiveOnly)
	if err != nil || !*read.Archivable {
		t.Fatalf("before the fold the author reads the meeting as not archivable (%v), want archivable", err)
	}

	calendar.Identity = connector.CrossDoorIdentity{Series: series, Occurrence: at}
	if landed := calendar.CaptureThrough(t, e, compose.NewCaptureSink(e.Pool, compose.CaptureConfig{})); landed != meeting.UUID {
		t.Fatalf("the calendar's next sync landed on %s, want the hand-logged meeting %s", landed, meeting)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, copyID) != 0 ||
		e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, meeting.UUID) == 0 {
		t.Fatal("the calendar copy did not fold onto the hand-logged meeting")
	}

	if read, err = e.Activities.GetActivity(rep, meeting, storekit.LiveOnly); err != nil || *read.Archivable {
		t.Errorf("after the fold the author reads the meeting as archivable (%v), want not archivable", err)
	}
	if _, err := e.Activities.ArchiveActivity(rep, meeting, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the author archiving the folded meeting → %v, want ErrPermissionDenied", err)
	}
}

func TestANoteAnAgentLoggedForAMemberIsTheirsToArchive(t *testing.T) {
	e := Setup(t)
	elsewhere := e.SeedContact(t, "Other team's", &e.Rep3)
	forMe := logOwnActivity(e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, ownArchivePerms), t, e, "note", elsewhere)
	forTeammate := logOwnActivity(e.AgentFor(t, e.Rep2, []ids.UUID{e.Team1}, ownArchivePerms), t, e, "note", elsewhere)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)

	if _, err := e.Activities.ArchiveActivity(rep, forTeammate, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a note an agent logged for a teammate → %v, want ErrPermissionDenied", err)
	}
	if _, err := e.Activities.ArchiveActivity(rep, forMe, nil); err != nil {
		t.Errorf("archiving a note my own agent logged → %v, want allowed", err)
	}
}

func TestTheOwnerOfADealArchivesIt(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	mine := ids.From[ids.DealKind](e.SeedDeal(t, "Mine", pipeline, open, &e.Rep1))
	theirs := ids.From[ids.DealKind](e.SeedDeal(t, "Teammate's", pipeline, open, &e.Rep2))

	read, err := e.Deals.GetDeal(rep, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !*read.Writable || *read.Archivable {
		t.Errorf("teammate's deal: writable=%v archivable=%v, want true/false", *read.Writable, *read.Archivable)
	}
	if _, err := e.Deals.ArchiveDeal(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a teammate's deal → %v, want ErrPermissionDenied", err)
	}
	if read, err = e.Deals.GetDeal(rep, mine, storekit.LiveOnly); err != nil || !*read.Archivable {
		t.Fatalf("my own deal: archivable=%v err=%v, want true", read.Archivable, err)
	}
	if _, err := e.Deals.ArchiveDeal(rep, mine, nil); err != nil {
		t.Fatalf("archiving my own deal → %v, want allowed", err)
	}
	archiveWroteTheShape(t, e, mine.UUID, "deal.archived")
}

func TestADealTheRepNamedThemselvesOwnerOfStaysTheManagersToArchive(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	me := ids.From[ids.UserKind](e.Rep1)
	taken := ids.From[ids.DealKind](e.SeedDeal(t, "Teammate's", pipeline, open, &e.Rep2))
	handed := ids.From[ids.DealKind](e.SeedDeal(t, "Handed over", pipeline, open, &e.Rep2))

	if _, err := e.Deals.UpdateDeal(rep, taken, deals.UpdateDealInput{OwnerID: &me}); err != nil {
		t.Fatalf("a teammate naming themselves owner: %v", err)
	}
	if _, err := e.Deals.UpdateDeal(e.Admin(), handed, deals.UpdateDealInput{OwnerID: &me}); err != nil {
		t.Fatalf("an admin handing the deal over: %v", err)
	}
	read, err := e.Deals.GetDeal(rep, taken, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if *read.Archivable {
		t.Error("a deal the rep named themselves owner of reads as archivable")
	}
	if _, err := e.Deals.ArchiveDeal(rep, taken, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a deal the rep took over themselves → %v, want ErrPermissionDenied", err)
	}
	if _, err := e.Deals.ArchiveDeal(rep, handed, nil); err != nil {
		t.Errorf("archiving a deal an admin handed to the rep → %v, want allowed", err)
	}
}

func withActivityDelete(base principal.Permissions) principal.Permissions {
	objects := make(map[string]principal.ObjectGrant, len(base.Objects))
	maps.Copy(objects, base.Objects)
	grant := objects["activity"]
	grant.Delete = true
	objects["activity"] = grant
	base.Objects = objects
	return base
}

// deleteOnlyPerms is a custom role that may read and archive but not edit.
var deleteOnlyPerms = principal.Permissions{
	RoleKeys: []string{roleRep},
	Objects: map[string]principal.ObjectGrant{
		objContact:         {Read: true},
		objDeal:            {Read: true, Delete: true},
		"activity":         {Read: true, Delete: true},
		objPipeline:        {Read: true},
		objInstallSettings: {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func TestADeleteOnlyRoleReadsArchivableWhereItMayArchive(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	archiver := asTeam1Member(e, e.Rep1, deleteOnlyPerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)
	note := logOwnActivity(asTeam1Member(e, e.Rep2, ownArchivePerms), t, e, "note", mine)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Teammate's", pipeline, open, &e.Rep2))

	activity, err := e.Activities.GetActivity(archiver, note, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if *activity.Writable || !*activity.Archivable {
		t.Errorf("note: writable=%v archivable=%v, want false/true", *activity.Writable, *activity.Archivable)
	}
	read, err := e.Deals.GetDeal(archiver, deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if *read.Writable || !*read.Archivable {
		t.Errorf("deal: writable=%v archivable=%v, want false/true", *read.Writable, *read.Archivable)
	}
	if _, err := e.Activities.ArchiveActivity(archiver, note, nil); err != nil {
		t.Errorf("archiving the note it reads as archivable → %v", err)
	}
	if _, err := e.Deals.ArchiveDeal(archiver, deal, nil); err != nil {
		t.Errorf("archiving the deal it reads as archivable → %v", err)
	}
}

// handOn names owner on deal as the caller does.
func handOn(as context.Context, t *testing.T, e *Env, deal ids.DealID, owner ids.UUID) {
	t.Helper()
	next := ids.From[ids.UserKind](owner)
	if _, err := e.Deals.UpdateDeal(as, deal, deals.UpdateDealInput{OwnerID: &next}); err != nil {
		t.Fatalf("handing the deal to %s: %v", owner, err)
	}
}

func assertDealArchive(as context.Context, t *testing.T, e *Env, deal ids.DealID, allowed bool, what string) {
	t.Helper()
	read, err := e.Deals.GetDeal(as, deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if *read.Archivable != allowed {
		t.Errorf("%s: archivable=%v, want %v", what, *read.Archivable, allowed)
	}
	_, err = e.Deals.ArchiveDeal(as, deal, nil)
	if allowed && err != nil {
		t.Errorf("%s: archive → %v, want allowed", what, err)
	}
	if !allowed && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("%s: archive → %v, want ErrPermissionDenied", what, err)
	}
}

func TestTheLatestOwnerChangeDecidesWhetherTheOwnerArchives(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	seed := func() ids.DealID {
		return ids.From[ids.DealKind](e.SeedDeal(t, "Deal "+ids.NewV7().String()[:8], pipeline, open, &e.Rep2))
	}

	retaken := seed()
	handOn(e.Admin(), t, e, retaken, e.Rep1)
	handOn(e.Admin(), t, e, retaken, e.Rep2)
	handOn(rep, t, e, retaken, e.Rep1)
	assertDealArchive(rep, t, e, retaken, false, "handed over, then taken back after a colleague held it")

	handedBack := seed()
	handOn(rep, t, e, handedBack, e.Rep1)
	handOn(e.Admin(), t, e, handedBack, e.Rep2)
	handOn(e.Admin(), t, e, handedBack, e.Rep1)
	assertDealArchive(rep, t, e, handedBack, true, "taken, then handed over by an admin")

	byAgent := seed()
	handOn(e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, ownArchivePerms), t, e, byAgent, e.Rep1)
	assertDealArchive(rep, t, e, byAgent, false, "the rep's agent named the rep owner")

	claimed := seed()
	e.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, claimed.UUID)
	if _, err := e.Deals.ClaimDeal(rep, claimed, nil); err != nil {
		t.Fatalf("claiming an ownerless deal: %v", err)
	}
	assertDealArchive(rep, t, e, claimed, false, "claimed while ownerless")

	reassigned := seed()
	if err := e.Deals.Tx(rep, func(tx pgx.Tx) error {
		return e.Deals.ReassignDealTx(rep, tx, reassigned, ids.From[ids.UserKind](e.Rep1), nil)
	}); err != nil {
		t.Fatalf("a bulk reassign to the rep: %v", err)
	}
	assertDealArchive(rep, t, e, reassigned, false, "reassigned to self in bulk")
}
