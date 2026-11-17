// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Done for you is the reader's own work: the records they, a teammate or a
// grant to them stand behind. A deal is readable by every seat, so read access
// alone would hand a rep every machine change on every team's pipeline.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func doneEntities(t *testing.T, e *Env, reader ids.UUID, teams []ids.UUID, perms principal.Permissions) map[ids.UUID]string {
	t.Helper()
	since := time.Now().Add(-time.Hour)
	receipt, err := magic.NewService(e.Pool, nil, time.Now).Read(e.As(reader, teams, perms), &since, 50)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	shown := map[ids.UUID]string{}
	for _, line := range receipt.Done {
		if line.Entity != nil {
			shown[ids.UUID(line.Entity.Id)] = line.Summary.Key
		}
	}
	return shown
}

func TestDoneForYouLeavesOutAnotherTeamsDeal(t *testing.T) {
	e := Setup(t)
	theirs := seedMachineAction(t, e, e.Rep3, "agent", "agent:auto-apply", "advance_stage")

	for name, perms := range map[string]principal.Permissions{"rep": RepPerms, "admin": AdminPerms} {
		if _, shown := doneEntities(t, e, e.Rep1, []ids.UUID{e.Team1}, perms)[theirs]; shown {
			t.Errorf("%s: a machine change on another team's deal was listed as done for them", name)
		}
	}
}

func TestDoneForYouShowsATeammatesDeal(t *testing.T) {
	e := Setup(t)
	teammates := seedMachineAction(t, e, e.Rep2, "agent", "agent:auto-apply", "advance_stage")

	if _, shown := doneEntities(t, e, e.Rep1, []ids.UUID{e.Team1}, RepPerms)[teammates]; !shown {
		t.Fatal("a machine change on a teammate's deal was left out")
	}
}

func TestDoneForYouShowsADealSharedWithTheReader(t *testing.T) {
	e := Setup(t)
	shared := seedMachineAction(t, e, e.Rep3, "agent", "agent:auto-apply", "advance_stage")
	e.WsExec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ('deal', $1, 'user', $2, 'read', $3)`, shared, e.Rep1, e.Rep3)

	if _, shown := doneEntities(t, e, e.Rep1, []ids.UUID{e.Team1}, RepPerms)[shared]; !shown {
		t.Fatal("a machine change on a deal shared with the reader was left out")
	}
}

func TestDoneForYouShowsOnlyMailOnTheReadersRecordsAndNamesIt(t *testing.T) {
	e := Setup(t)
	mineContact, theirContact := e.SeedContact(t, "Ana Mine", &e.Rep1), e.SeedContact(t, "Tom Theirs", &e.Rep3)
	savedMail := func(contact ids.UUID, kind string) ids.UUID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, $2, 'Offer', now(), 'connector:gmail', 'connector:gmail')`, id, kind)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
			VALUES ($1, 'contact', $2)`, id, contact)
		e.WsExec(t, `INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ('connector', 'connector:gmail', 'create', 'activity', $1, now())`, id)
		return id
	}
	mine, theirs := savedMail(mineContact, "email"), savedMail(theirContact, "meeting")
	shown := doneEntities(t, e, e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)
	if got := shown[mine]; got != "magic.action.create_activity_email" {
		t.Errorf("mail saved on the reader's contact reads %q, want it named as an email", got)
	}
	if _, listed := shown[theirs]; listed {
		t.Error("a meeting saved on another team's contact was listed as done for the reader")
	}
}
