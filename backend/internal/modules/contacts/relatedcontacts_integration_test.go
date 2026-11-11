// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// relatedGraph is one buyer with a current colleague, a former one, a person
// seated beside them on a deal, and a stranger at that person's company.
type relatedGraph struct {
	buyer, colleague, former, dealPeer, stranger ids.ContactID
}

func (e *privacyEnv) seedRelatedGraph(t *testing.T) relatedGraph {
	t.Helper()
	g := relatedGraph{
		buyer: ids.New[ids.ContactKind](), colleague: ids.New[ids.ContactKind](),
		former: ids.New[ids.ContactKind](), dealPeer: ids.New[ids.ContactKind](),
		stranger: ids.New[ids.ContactKind](),
	}
	buyerCo, peerCo := ids.NewV7(), ids.NewV7()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ctx := e.as(e.owner, principal.RowScopeAll)
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO company (id, display_name, owner_id, source, captured_by) VALUES ($1, 'Buyer GmbH', $2, 'manual', 'human:x'), ($3, 'Partner AG', $2, 'manual', 'human:x')`, []any{buyerCo, e.owner, peerCo}},
		{`INSERT INTO contact (id, full_name, owner_id, source, captured_by) VALUES
			($1, 'Bea Buyer', $6, 'manual', 'human:x'), ($2, 'Cole League', $6, 'manual', 'human:x'),
			($3, 'Fern Former', $6, 'manual', 'human:x'), ($4, 'Dana Peer', $6, 'manual', 'human:x'),
			($5, 'Stan Stranger', $6, 'manual', 'human:x')`, []any{g.buyer, g.colleague, g.former, g.dealPeer, g.stranger, e.owner}},
		{`INSERT INTO relationship (kind, contact_id, company_id, is_current_primary, ended_at, source, captured_by) VALUES
			('employment', $1, $4, true, NULL, 'manual', 'human:x'), ('employment', $2, $4, true, NULL, 'manual', 'human:x'),
			('employment', $3, $4, false, current_date - 30, 'manual', 'human:x'), ('employment', $5, $6, true, NULL, 'manual', 'human:x'),
			('employment', $7, $6, true, NULL, 'manual', 'human:x')`, []any{g.buyer, g.colleague, g.former, buyerCo, g.dealPeer, peerCo, g.stranger}},
		{`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by) VALUES ($1, $2, 'Rollout', $3, $4, 'manual', 'human:x')`, []any{deal, e.owner, pipeline, stage}},
		{`INSERT INTO relationship (kind, contact_id, deal_id, source, captured_by) VALUES ('deal_stakeholder', $1, $3, 'manual', 'human:x'), ('deal_stakeholder', $2, $3, 'manual', 'human:x')`, []any{g.buyer, g.dealPeer, deal}},
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for _, q := range seed {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the buyer's graph: %v", err)
	}
	return g
}

// relatedReader reads contacts and edges, and deals only when asked to.
func (e *privacyEnv) relatedReader(t *testing.T, user ids.UUID, scope principal.RowScope, deals bool) context.Context {
	t.Helper()
	ctx := e.as(user, scope)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no actor bound")
	}
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true}
	if deals {
		actor.Permissions.Objects["deal"] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(ctx, actor)
}

func relatedNames(ctx context.Context, t *testing.T, e *privacyEnv, anchor ids.ContactID) []string {
	t.Helper()
	rows, _, err := e.store.ListContacts(ctx, ListContactsInput{RelatedTo: &anchor})
	if err != nil {
		t.Fatalf("listing the people around %s: %v", anchor, err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.FullName)
	}
	slices.Sort(names)
	return names
}

func TestRelatedToListsCurrentColleaguesAndDealPeers(t *testing.T) {
	e := setupCapturePrivacy(t)
	g := e.seedRelatedGraph(t)
	got := relatedNames(e.relatedReader(t, e.owner, principal.RowScopeAll, true), t, e, g.buyer)
	if want := []string{"Cole League", "Dana Peer"}; !slices.Equal(got, want) {
		t.Errorf("related to the buyer: %v, want %v (no former colleague, no stranger, not the buyer)", got, want)
	}
}

func TestRelatedToLeavesDealPeersOutForAReaderWithoutDeals(t *testing.T) {
	e := setupCapturePrivacy(t)
	g := e.seedRelatedGraph(t)
	got := relatedNames(e.relatedReader(t, e.owner, principal.RowScopeAll, false), t, e, g.buyer)
	if want := []string{"Cole League"}; !slices.Equal(got, want) {
		t.Errorf("related to the buyer without deal read: %v, want %v", got, want)
	}
}

func TestRelatedToAContactTheReaderCannotOpenIsEmpty(t *testing.T) {
	e := setupCapturePrivacy(t)
	g := e.seedRelatedGraph(t)
	hidden := e.captureContact(t, "owner")
	ctx := e.as(e.owner, principal.RowScopeAll)
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE relationship SET contact_id = $1 WHERE contact_id = $2`, hidden, g.buyer)
		return err
	}); err != nil {
		t.Fatalf("moving the buyer's edges to a private contact: %v", err)
	}
	if got := relatedNames(e.relatedReader(t, e.admin, principal.RowScopeAll, true), t, e, hidden); len(got) != 0 {
		t.Errorf("an admin listed %v around a contact they cannot open, want nobody", got)
	}
	if got := relatedNames(e.relatedReader(t, e.owner, principal.RowScopeAll, true), t, e, hidden); len(got) != 2 {
		t.Errorf("the owner listed %v around their own contact, want the colleague and the deal peer", got)
	}
}
