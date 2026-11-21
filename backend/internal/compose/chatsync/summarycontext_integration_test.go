// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatsync

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/auth"
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

// parties reads the deal's parties for the members who may read its summary
// note. It answers the widest statement that took, and how many readers there were.
func (e *partyEnv) parties(t *testing.T) (facts []string, widestArgs, readerCount int) {
	t.Helper()
	ws := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), chatLinksPrincipal)
	widest := &widestStatement{}
	err := database.WithWorkspaceTx(ws, e.pool, func(tx pgx.Tx) error {
		readers, err := noteReaders(ws, tx, chatlinks.Link{DealID: &e.deal, ChannelSlug: "acme"})
		if err != nil {
			return err
		}
		widest.Tx, readerCount = tx, len(readers)
		facts, err = dealParties(ws, widest, e.deal, readers)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return facts, widest.args, readerCount
}

// members adds n members who hold role.
func (e *partyEnv) members(t *testing.T, n int, role string) {
	t.Helper()
	roles := map[ids.UUID]string{}
	for range n {
		id := ids.NewV7()
		e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $2)`, id, id.String()+"@chat.test")
		roles[id] = role
	}
	e.assignRoles(t, roles)
}

// Every member of a workspace may read a summary note. One statement judging
// them all at once grows with the workspace until the database runs out of memory.
func TestTheStatementsThatJudgeSummaryReadersDoNotGrowWithTheWorkspace(t *testing.T) {
	e := setupParties(t)
	_, _, seeded := e.parties(t)
	e.members(t, auth.MaxReadersPerClause-seeded, "management")
	one, oneWidth, _ := e.parties(t)
	e.members(t, 2*auth.MaxReadersPerClause, "management")
	all, allWidth, live := e.parties(t)
	want := []string{"Company: Acme GmbH", "Contacts: Clara Buyer"}
	if !slices.Equal(one, want) || !slices.Equal(all, want) {
		t.Errorf("parties = %q for one group and %q for all, want %q for both", one, all, want)
	}
	if allWidth > oneWidth {
		t.Errorf("a statement for %d members carried %d arguments, and %d for %d; it grows with the workspace",
			live, allWidth, oneWidth, auth.MaxReadersPerClause)
	}
}

// A reader in a later group still bounds the names. One refused the
// stakeholder edge leaves the contacts out, though every earlier group may read them.
func TestAReaderInTheLastGroupStillBoundsTheNames(t *testing.T) {
	e := setupParties(t)
	e.members(t, 2*auth.MaxReadersPerClause, "management")
	e.denyRead(t, "rep", "relationship")
	managers, _, _ := e.parties(t)
	e.members(t, 1, "rep")
	everyone, _, _ := e.parties(t)
	if want := []string{"Company: Acme GmbH", "Contacts: Clara Buyer"}; !slices.Equal(managers, want) {
		t.Fatalf("parties for the managers = %q, want %q", managers, want)
	}
	if want := []string{"Company: Acme GmbH"}; !slices.Equal(everyone, want) {
		t.Errorf("parties = %q, want %q: the last reader may not read the stakeholder edge", everyone, want)
	}
}
