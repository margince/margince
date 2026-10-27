// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordlinks

// Typed links against a real database: the two-ended access rule, the write
// shape and what archiving an end does to a read.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

type linkEnv struct {
	owner   *pgx.Conn
	store   *Store
	ws      ids.UUID
	rep     ids.UUID
	other   ids.UUID
	deal    ids.UUID
	renewal ids.UUID
	company ids.UUID
	theirs  ids.UUID
}

func setupLinks(t *testing.T) *linkEnv {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}
	e := &linkEnv{
		owner: owner, ws: ids.NewV7(), rep: ids.NewV7(), other: ids.NewV7(),
		deal: ids.NewV7(), renewal: ids.NewV7(), company: ids.NewV7(), theirs: ids.NewV7(),
	}
	e.exec(t, `INSERT INTO workspace (id) VALUES ($1)`, e.ws)
	for _, u := range []ids.UUID{e.rep, e.other} {
		e.exec(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, u, "rep-"+u.String()+"@links.test")
	}
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Links')`, pipeline)
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	for _, d := range []struct {
		id    ids.UUID
		name  string
		owner ids.UUID
	}{{e.deal, "First deal", e.rep}, {e.renewal, "Renewal", e.rep}, {e.theirs, "Not mine", e.other}} {
		e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
			VALUES ($1, $2, 'open', $3, $4, $5, 'seed', 'system')`, d.id, d.name, d.owner, pipeline, stage)
	}
	e.exec(t, `INSERT INTO company (id, display_name, owner_id, source, captured_by)
		VALUES ($1, 'Acme', $2, 'seed', 'system')`, e.company, e.rep)
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	return e
}

func (e *linkEnv) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *linkEnv) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seat holds the given grants and sees only its own rows unless all is set.
func (e *linkEnv) seat(user ids.UUID, all bool, grants map[string]principal.ObjectGrant) context.Context {
	rowScope := principal.RowScopeOwn
	if all {
		rowScope = principal.RowScopeAll
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, Objects: grants, RowScope: rowScope},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func full() map[string]principal.ObjectGrant {
	g := principal.ObjectGrant{Read: true, Update: true}
	return map[string]principal.ObjectGrant{"deal": g, "company": g, "contact": g}
}

func (e *linkEnv) dealLink(to ids.UUID, linkType string) NewLink {
	return NewLink{FromType: "deal", FromID: e.renewal, ToType: "deal", ToID: to, LinkType: linkType}
}

func TestALinkIsReadFromBothEndsAndWritesTheWriteShape(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, true, full())
	link, err := e.store.Create(ctx, e.dealLink(e.deal, "follow_up_of"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, end := range []ids.UUID{e.renewal, e.deal} {
		got, err := e.store.List(ctx, "deal", end)
		if err != nil || len(got) != 1 || got[0].Id != link.Id {
			t.Fatalf("List(%s) = %+v, %v; want the one link", end, got, err)
		}
		if got[0].FromName == nil || *got[0].FromName != "Renewal" || *got[0].ToName != "First deal" {
			t.Errorf("names = %v / %v", got[0].FromName, got[0].ToName)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1`, e.renewal); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'record_link.changed'`); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}

func TestLinkingAPairTwiceWritesOnce(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, true, full())
	first, err := e.store.Create(ctx, e.dealLink(e.deal, "renews"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.store.Create(ctx, e.dealLink(e.deal, "renews"))
	if err != nil || second.Id != first.Id {
		t.Fatalf("repeat = %+v, %v; want the first link", second, err)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, e.renewal); n != 1 {
		t.Errorf("a repeat wrote %d audit rows, want 1", n)
	}
	// The reverse of a pair is a different link.
	if _, err := e.store.Create(ctx, NewLink{FromType: "deal", FromID: e.deal, ToType: "deal", ToID: e.renewal, LinkType: "renews"}); err != nil {
		t.Errorf("the reverse link was refused: %v", err)
	}
}

func TestALinkTheTypeOrTheShapeForbidsIsRefused(t *testing.T) {
	e := setupLinks(t)
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.RegisterLinkTypes("links-test", []extension.LinkType{
		{Type: "follow_up_of", Label: "Follow-up", From: []string{"deal"}, To: []string{"deal"}},
	})
	ctx := e.seat(e.rep, true, full())
	cases := map[string]NewLink{
		"unknown type": e.dealLink(e.deal, "ext:nobody:follow_up_of"),
		"a type of core's own name that is not core's": e.dealLink(e.deal, "follows"),
		"to itself":                    e.dealLink(e.renewal, "related_to"),
		"ends the type does not allow": {FromType: "deal", FromID: e.renewal, ToType: "company", ToID: e.company, LinkType: "ext:links-test:follow_up_of"},
		"an end that is not linkable":  {FromType: "project", FromID: e.deal, ToType: "deal", ToID: e.renewal, LinkType: "related_to"},
	}
	for name, in := range cases {
		var parse *values.ParseError
		if _, err := e.store.Create(ctx, in); !errors.As(err, &parse) {
			t.Errorf("%s: err = %v, want a parse error", name, err)
		}
	}
	if _, err := e.store.Create(ctx, e.dealLink(e.deal, "ext:links-test:follow_up_of")); err != nil {
		t.Errorf("a deal-to-deal link of the extension's type was refused: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM record_link`); n != 1 {
		t.Errorf("record_link holds %d rows, want only the one valid link", n)
	}
}

func TestBothEndsGateALink(t *testing.T) {
	e := setupLinks(t)
	own := e.seat(e.rep, false, full())

	// Reading a deal is open to every seat; changing one is the owner's. So a
	// link from a colleague's deal is refused, and a link to it is a filing.
	fromTheirs := NewLink{FromType: "deal", FromID: e.theirs, ToType: "deal", ToID: e.renewal, LinkType: "related_to"}
	if _, err := e.store.Create(own, fromTheirs); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a link from a deal the caller does not own gave %v, want permission denied", err)
	}
	if _, err := e.store.Create(own, e.dealLink(e.theirs, "related_to")); err != nil {
		t.Errorf("a link to a colleague's deal was refused: %v", err)
	}
	if _, err := e.store.Create(own, e.dealLink(ids.NewV7(), "related_to")); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a link to a deal that does not exist gave %v, want not found", err)
	}
	readOnly := e.seat(e.rep, true, map[string]principal.ObjectGrant{"deal": {Read: true}})
	if _, err := e.store.Create(readOnly, e.dealLink(e.deal, "related_to")); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a link by a seat that may not update the deal gave %v, want permission denied", err)
	}
	if n := e.count(t, `SELECT count(*) FROM record_link`); n != 1 {
		t.Errorf("record_link holds %d rows, want only the link to the colleague's deal", n)
	}
}

func TestAReadDropsALinkWhoseOtherEndTheCallerMayNotRead(t *testing.T) {
	e := setupLinks(t)
	all := e.seat(e.rep, true, full())
	if _, err := e.store.Create(all, NewLink{FromType: "deal", FromID: e.renewal, ToType: "company", ToID: e.company, LinkType: "related_to"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.Create(all, e.dealLink(e.deal, "renews")); err != nil {
		t.Fatal(err)
	}
	dealsOnly := e.seat(e.rep, true, map[string]principal.ObjectGrant{"deal": {Read: true}})
	got, err := e.store.List(dealsOnly, "deal", e.renewal)
	if err != nil || len(got) != 1 || got[0].ToType != "deal" {
		t.Fatalf("a read without the right on companies = %+v, %v; want only the deal link", got, err)
	}
	if got, err := e.store.List(all, "deal", e.renewal); err != nil || len(got) != 2 {
		t.Errorf("a read with both rights = %+v, %v; want both links", got, err)
	}
	if _, err := e.store.List(e.seat(e.rep, true, map[string]principal.ObjectGrant{}), "deal", e.renewal); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a read without the right on deals gave %v, want permission denied", err)
	}
}

func TestArchivingAnEndHidesTheLinkAndUnarchivingBringsItBack(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, true, full())
	if _, err := e.store.Create(ctx, e.dealLink(e.deal, "follow_up_of")); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, e.deal)
	if got, err := e.store.List(ctx, "deal", e.renewal); err != nil || len(got) != 0 {
		t.Errorf("a link to an archived deal read %+v, %v; want none", got, err)
	}
	if _, err := e.store.Create(ctx, e.dealLink(e.deal, "renews")); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a new link to an archived deal gave %v, want not found", err)
	}
	e.exec(t, `UPDATE deal SET archived_at = NULL WHERE id = $1`, e.deal)
	if got, err := e.store.List(ctx, "deal", e.renewal); err != nil || len(got) != 1 {
		t.Errorf("after unarchiving the read is %+v, %v; want the link back", got, err)
	}
}

func TestRemovingALinkDeletesTheRowAndAuditsIt(t *testing.T) {
	e := setupLinks(t)
	ctx := e.seat(e.rep, true, full())
	link, err := e.store.Create(ctx, e.dealLink(e.deal, "related_to"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.Delete(e.seat(e.rep, true, map[string]principal.ObjectGrant{"deal": {Read: true}}), ids.UUID(link.Id)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a delete without update gave %v, want permission denied", err)
	}
	if err := e.store.Delete(ctx, ids.UUID(link.Id)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := e.store.Delete(ctx, ids.UUID(link.Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a second delete gave %v, want not found", err)
	}
	if n := e.count(t, `SELECT count(*) FROM record_link`); n != 0 {
		t.Errorf("record_link holds %d rows after the delete", n)
	}
	if n := e.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, e.renewal); n != 2 {
		t.Errorf("audit rows = %d, want one for the link and one for its removal", n)
	}
}
