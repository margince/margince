// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedRoles lays down the shipped roles and gives each member one of them.
func (e *flushEnv) seedRoles(t *testing.T, roles map[ids.UUID]string) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.SeedSystemRoles(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for user, key := range roles {
		e.exec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $1 FROM role WHERE key = $2 AND is_system`, user, key)
	}
}

// A new channel is opened to the members who may open the deal, the owner
// first, at most fifty. A member whose role grants no deal read is left out,
// and the members over the cap are counted.
func TestANewChannelInvitesWhoMaySeeTheDealUpToTheCap(t *testing.T) {
	e := setupFlush(t)
	e.exec(t, `DELETE FROM chat_link`)
	outsider := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'outsider@chat.test', 'Otto Outsider')`, outsider)
	roles := map[ids.UUID]string{e.rep: "rep"}
	for i := range 51 {
		lead := ids.NewV7()
		e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Lead')`, lead, fmt.Sprintf("lead%02d@chat.test", i))
		roles[lead] = "management"
	}
	e.exec(t, `INSERT INTO app_user (email, display_name, is_agent) VALUES ('agent@chat.test', 'Agent', true)`)
	e.seedRoles(t, roles)

	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}}},
	})
	state, err := e.store.Link(principal.WithCorrelationID(ctx, ids.NewV7()), chatlinks.RecordDeal, e.deal,
		chatlinks.LinkRequest{Mode: chatlinks.ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Invites; got == nil || got.Invited != 50 || got.SkippedOverCap != 2 {
		t.Fatalf("invites = %+v to %v, want 50 invited and 2 over the cap", got, e.chat.invited)
	}
	if e.chat.invited[0] != "rep@chat.test" {
		t.Errorf("first invited = %q, want the owner", e.chat.invited[0])
	}
	if slices.Contains(e.chat.invited, "outsider@chat.test") || slices.Contains(e.chat.invited, "agent@chat.test") {
		t.Fatalf("invited %v, which holds a member who may not see the deal or an agent", e.chat.invited)
	}
}
