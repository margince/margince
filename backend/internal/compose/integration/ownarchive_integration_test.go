// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

func logOwnActivity(t *testing.T, e *Env, as context.Context, kind string, on ids.UUID) ids.ActivityID {
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

func TestAMemberArchivesTheNotesAndTasksTheyWrote(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	teammate := asTeam1Member(e, e.Rep2, ownArchivePerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)

	note, task := logOwnActivity(t, e, rep, "note", mine), logOwnActivity(t, e, rep, "task", mine)
	theirs := logOwnActivity(t, e, teammate, "note", mine)

	read, err := e.Activities.GetActivity(rep, theirs, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !*read.Writable || *read.Archivable {
		t.Errorf("teammate's note on my contact: writable=%v archivable=%v, want true/false", *read.Writable, *read.Archivable)
	}
	if _, err := e.Activities.ArchiveActivity(rep, theirs, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving a teammate's note → %v, want ErrPermissionDenied", err)
	}
	for _, own := range []ids.ActivityID{note, task} {
		read, err := e.Activities.GetActivity(rep, own, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if !*read.Archivable {
			t.Errorf("my own %s reads as not archivable", read.Kind)
		}
		if _, err := e.Activities.ArchiveActivity(rep, own, nil); err != nil {
			t.Fatalf("archiving my own %s → %v, want allowed", read.Kind, err)
		}
		archiveWroteTheShape(t, e, own.UUID, "activity.archived")
	}
}

func TestOnlyANoteOrTaskIsTheAuthorsToArchive(t *testing.T) {
	e := Setup(t)
	rep := asTeam1Member(e, e.Rep1, ownArchivePerms)
	mine := e.SeedContact(t, "Mine", &e.Rep1)
	mail := SeedIDRow(t, OwnerConn(t), `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Re: terms', 'body', now(), 'gmail', 'connector:gmail:`+e.Rep1.String()+`')`)
	LinkActivity(t, OwnerConn(t), mail, objContact, mine)
	call := logOwnActivity(t, e, rep, "call", mine)

	manager := asTeam1Member(e, e.Rep1, withActivityDelete(ownArchivePerms))
	for name, id := range map[string]ids.ActivityID{"captured mail": ids.From[ids.ActivityKind](mail), "logged call": call} {
		if _, err := e.Activities.ArchiveActivity(rep, id, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a rep archiving their own %s → %v, want ErrPermissionDenied", name, err)
		}
		if _, err := e.Activities.ArchiveActivity(manager, id, nil); err != nil {
			t.Errorf("a seat holding activity.delete archiving the %s → %v, want allowed", name, err)
		}
	}
}

func TestANoteAnAgentLoggedForAMemberIsTheirsToArchive(t *testing.T) {
	e := Setup(t)
	elsewhere := e.SeedContact(t, "Other team's", &e.Rep3)
	forMe := logOwnActivity(t, e, e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, ownArchivePerms), "note", elsewhere)
	forTeammate := logOwnActivity(t, e, e.AgentFor(t, e.Rep2, []ids.UUID{e.Team1}, ownArchivePerms), "note", elsewhere)
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

func withActivityDelete(base principal.Permissions) principal.Permissions {
	objects := make(map[string]principal.ObjectGrant, len(base.Objects))
	maps.Copy(objects, base.Objects)
	grant := objects["activity"]
	grant.Delete = true
	objects["activity"] = grant
	base.Objects = objects
	return base
}
