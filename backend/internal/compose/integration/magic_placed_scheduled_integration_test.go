// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A scheduled message is its author's alone until it goes out. Done for you
// places one an agent scheduled on the record it files under, for the author
// and for nobody else.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// scheduledReader is a human admin of the preflight workspace on the teams
// given, without the grants named.
func (p *preflightEnv) scheduledReader(t *testing.T, user ids.UUID, teams []ids.UUID, without ...string) context.Context {
	t.Helper()
	perms := AdminPerms
	perms.Objects = maps.Clone(AdminPerms.Objects)
	for _, object := range without {
		delete(perms.Objects, object)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), p.workspaceID(t)), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, TeamIDs: teams, Permissions: perms,
	})
}

// scheduleForTheUser has an agent acting for the workspace's user schedule a
// reply on the anchor, which stakeADeal linked to the deal.
func (p *preflightEnv) scheduleForTheUser(t *testing.T) {
	t.Helper()
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	user := uuidOf(t, p.user)
	passport := p.issuePassport(t)
	p.scheduleAsAgent(t, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + passport.String(), PassportID: passport,
		UserID: user, OnBehalfOf: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				objActivity: {Create: true, Read: true, Update: true},
				objContact:  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	}, clocktest.Now(t).Add(2*time.Hour))
}

// authorsTeam puts the workspace's user on a new team and answers it, so a
// teammate works on the deal the user owns.
func (p *preflightEnv) authorsTeam(t *testing.T) ids.UUID {
	t.Helper()
	team := ids.NewV7()
	if err := apptest.InWorkspace(p.AppEnv, t, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO team (id, name) VALUES ($1, 'Senders')`, team); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, team, uuidOf(t, p.user))
		return err
	}); err != nil {
		t.Fatalf("putting the author on a team: %v", err)
	}
	return team
}

func TestDoneForYouShowsAScheduledSendOnlyToItsAuthor(t *testing.T) {
	p := setupPreflight(t)
	p.scheduleForTheUser(t)
	deal := uuidOf(t, p.dealID)
	const key = "magic.action.schedule_send"

	team := []ids.UUID{p.authorsTeam(t)}
	author, worklist := placedLines(p.scheduledReader(t, uuidOf(t, p.user), team), t, &Env{Pool: p.Pool})
	if author[key] != deal {
		t.Fatalf("Home names %v for the author's scheduled email, want the deal %v (lines %v)", author[key], deal, author)
	}
	if worklist[key] != deal {
		t.Fatalf("the Worklist names %v for the author's scheduled email, want the deal %v", worklist[key], deal)
	}
	for name, reader := range map[string]context.Context{
		"a teammate on the deal":      p.scheduledReader(t, ids.NewV7(), team),
		"the author without activity": p.scheduledReader(t, uuidOf(t, p.user), team, objActivity),
	} {
		home, worklist := placedLines(reader, t, &Env{Pool: p.Pool})
		if _, shown := home[key]; shown {
			t.Errorf("Home shows the scheduled email to %s", name)
		}
		if _, shown := worklist[key]; shown {
			t.Errorf("the Worklist shows the scheduled email to %s", name)
		}
	}
}
