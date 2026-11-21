// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asMember is the default member seat. It may create and update relationships
// but holds no relationship delete grant, so only the deal write can admit a removal.
func (e *edgeAnchorEnv) asMember(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"relationship": {Create: true, Read: true, Update: true},
				"contact":      {Create: true, Read: true, Update: true},
				"deal":         {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

func (e *edgeAnchorEnv) seedDeal(t *testing.T, owner ids.UUID) ids.DealID {
	t.Helper()
	ctx := context.Background()
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.New[ids.DealKind]()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name, is_default, position) VALUES ($1::uuid, 'Sales ' || $1::text, false, 0)`, []any{pipeline}},
		{`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, source, captured_by) VALUES ($1, $2, 'Rollout', $3, $4, 'manual', 'human:seed')`, []any{deal, owner, pipeline, stage}},
	} {
		if _, err := e.owner.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seeding the deal: %v", err)
		}
	}
	return deal
}

func (e *edgeAnchorEnv) countRows(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Whoever may write a deal may take a stakeholder off it, with no relationship
// delete grant, and the removal commits its audit and outbox rows.
func TestADealWriterRemovesAStakeholderWithoutTheDeleteGrant(t *testing.T) {
	e := setupEdgeAnchor(t)
	me := e.asMember(e.me)
	deal := e.seedDeal(t, e.me)

	edge, err := e.store.CreateRelationship(me, CreateRelationshipInput{
		Kind: dealStakeholderKind, DealID: &deal, ContactID: &e.theirContact, Source: "manual",
	})
	if err != nil {
		t.Fatalf("adding a stakeholder to the caller's own deal: %v", err)
	}
	const outboxFor = `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1::text || '%'`
	before := e.countRows(t, outboxFor, edge.ID)
	if err := e.store.RefuseArchiveRelationship(me, edge.ID); err != nil {
		t.Fatalf("the stage-time probe refused a removal the store admits: %v", err)
	}
	if _, err := e.store.ArchiveRelationship(me, edge.ID, nil); err != nil {
		t.Fatalf("removing a stakeholder from the caller's own deal: %v", err)
	}

	if live, _ := e.liveAndPrimary(t, edge.ID); live {
		t.Error("the stakeholder row is still live after its removal")
	}
	if n := e.countRows(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship' AND entity_id = $1 AND action = 'archive'`, edge.ID); n != 1 {
		t.Errorf("%d archive audit rows for the removed stakeholder, want 1", n)
	}
	if after := e.countRows(t, outboxFor, edge.ID); after <= before {
		t.Errorf("outbox rows naming the edge went %d -> %d, want the archive to add one", before, after)
	}
}

// The deal's row authority still decides. A member who cannot write the deal is
// refused, and any other kind of edge keeps needing the delete grant.
func TestStakeholderRemovalStillNeedsWriteOnTheDeal(t *testing.T) {
	e := setupEdgeAnchor(t)
	theirDeal := e.seedDeal(t, e.them)
	edge := e.seedEdgeOn(t, theirDeal, e.them)

	me := e.asMember(e.me)
	if _, err := e.store.ArchiveRelationship(me, edge.ID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("removing a stakeholder from a deal the caller cannot write = %v, want ErrPermissionDenied", err)
	}
	if err := e.store.RefuseArchiveRelationship(me, edge.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the stage-time probe = %v, want ErrPermissionDenied", err)
	}
	if live, _ := e.liveAndPrimary(t, edge.ID); !live {
		t.Error("a refused removal archived the stakeholder anyway")
	}

	employment := e.seedEdgeAsMe(e.as(e.me), t)
	if _, err := e.store.ArchiveRelationship(me, employment.ID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("removing an employment edge without the delete grant = %v, want ErrPermissionDenied", err)
	}
}

func (e *edgeAnchorEnv) seedEdgeOn(t *testing.T, deal ids.DealID, owner ids.UUID) relationshipRow {
	t.Helper()
	ctx := e.asMember(owner)
	row, err := e.store.CreateRelationship(ctx, CreateRelationshipInput{
		Kind: dealStakeholderKind, DealID: &deal, ContactID: &e.theirContact, Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding a stakeholder as the deal's owner: %v", err)
	}
	return row
}

// A seat that can write deals but read no relationship learns nothing from the
// archive. An edge that exists and an id that does not answer the same.
func TestArchiveAnswersAlikeForAnEdgeAndAMissingIdWithoutReadGrant(t *testing.T) {
	e := setupEdgeAnchor(t)
	edge := e.seedEdgeAsMe(e.as(e.me), t)

	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.me.String(), UserID: e.me,
		Permissions: principal.Permissions{
			RoleKeys: []string{"custom"},
			Objects:  map[string]principal.ObjectGrant{"deal": {Read: true, Update: true}},
			RowScope: principal.RowScopeOwn,
		},
	})
	for name, id := range map[string]ids.UUID{"existing": edge.ID, "missing": ids.NewV7()} {
		if _, err := e.store.ArchiveRelationship(ctx, id, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("archiving a %s edge without relationship read = %v, want ErrPermissionDenied", name, err)
		}
		if err := e.store.RefuseArchiveRelationship(ctx, id); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("the stage-time probe on a %s edge = %v, want ErrPermissionDenied", name, err)
		}
	}
}

// The list carries an archive answer per edge. It is true on a deal the caller
// writes and false on another rep's deal.
func TestTheListSaysWhichStakeholdersTheCallerMayRemove(t *testing.T) {
	e := setupEdgeAnchor(t)
	mine := e.seedEdgeOn(t, e.seedDeal(t, e.me), e.me)
	theirs := e.seedEdgeOn(t, e.seedDeal(t, e.them), e.them)

	rows, _, err := e.store.ListRelationships(e.asMember(e.me), ListRelationshipsInput{ContactID: &e.theirContact})
	if err != nil {
		t.Fatal(err)
	}
	got := map[ids.UUID]bool{}
	for _, row := range rows {
		if row.CanArchive == nil {
			t.Fatalf("edge %s listed without an archive answer", row.ID)
		}
		got[row.ID] = *row.CanArchive
	}
	mineAnswer, mineListed := got[mine.ID]
	theirAnswer, theirListed := got[theirs.ID]
	if !mineListed || !theirListed || !mineAnswer || theirAnswer {
		t.Errorf("can_archive = %v, want true for %s and false for %s, both listed", got, mine.ID, theirs.ID)
	}
}
