// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// widestStatement passes every statement on and keeps the most arguments one
// of them carried, which grows with the clauses it was built from.
type widestStatement struct {
	pgx.Tx
	args int
}

//craft:ignore naked-any pgx passes a statement's arguments as any
func (w *widestStatement) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	w.args = max(w.args, len(args))
	return w.Tx.Query(ctx, sql, args...)
}

//craft:ignore naked-any pgx passes a statement's arguments as any
func (w *widestStatement) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	w.args = max(w.args, len(args))
	return w.Tx.QueryRow(ctx, sql, args...)
}

// partiesFor reads the deal's parties for the given members, and the widest
// statement that took.
func (e *partyEnv) partiesFor(t *testing.T, members []ids.UUID) ([]string, int) {
	t.Helper()
	ws := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), chatLinksPrincipal)
	var facts []string
	widest := &widestStatement{}
	err := database.WithWorkspaceTx(ws, e.pool, func(tx pgx.Tx) error {
		byID, err := identity.MemberPrincipalsTx(ws, tx, members)
		if err != nil {
			return err
		}
		readers := make([]principal.Principal, 0, len(members))
		for _, id := range members {
			reader, live := byID[id]
			if !live {
				return fmt.Errorf("member %s has no principal", id)
			}
			readers = append(readers, reader)
		}
		widest.Tx = tx
		facts, err = dealParties(ws, widest, e.deal, readers)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return facts, widest.args
}

func (e *partyEnv) member(t *testing.T, n int, role string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $2)`, id, fmt.Sprintf("member%d@chat.test", n))
	e.exec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT id, $2 FROM role WHERE key = $1 AND is_system`, role, id)
	return id
}

// Every member of a workspace may read a summary note. One statement judging
// them all at once grows with the workspace until the database runs out of memory.
func TestTheStatementsThatJudgeSummaryReadersDoNotGrowWithTheWorkspace(t *testing.T) {
	e := setupParties(t)
	members := make([]ids.UUID, 0, 3*readersPerStatement)
	for n := range cap(members) {
		members = append(members, e.member(t, n, "management"))
	}
	group, groupWidth := e.partiesFor(t, members[:readersPerStatement])
	all, allWidth := e.partiesFor(t, members)
	want := []string{"Company: Acme GmbH", "Contacts: Clara Buyer"}
	if !slices.Equal(group, want) || !slices.Equal(all, want) {
		t.Errorf("parties = %q for one group and %q for all, want %q for both", group, all, want)
	}
	if allWidth > groupWidth {
		t.Errorf("a statement for %d readers carried %d arguments, and %d for %d readers; it grows with the workspace",
			len(members), allWidth, groupWidth, readersPerStatement)
	}
}

// A reader in a later group still bounds the names. One refused the
// stakeholder edge leaves the contacts out, though every earlier group may read them.
func TestAReaderInTheLastGroupStillBoundsTheNames(t *testing.T) {
	e := setupParties(t)
	members := make([]ids.UUID, 0, 2*readersPerStatement+1)
	for n := range 2 * readersPerStatement {
		members = append(members, e.member(t, n, "management"))
	}
	e.denyRead(t, "rep", "relationship")
	managers, _ := e.partiesFor(t, members)
	members = append(members, e.member(t, len(members), "rep"))
	everyone, _ := e.partiesFor(t, members)
	if want := []string{"Company: Acme GmbH", "Contacts: Clara Buyer"}; !slices.Equal(managers, want) {
		t.Fatalf("parties for the managers = %q, want %q", managers, want)
	}
	if want := []string{"Company: Acme GmbH"}; !slices.Equal(everyone, want) {
		t.Errorf("parties = %q, want %q: the last reader may not read the stakeholder edge", everyone, want)
	}
}
