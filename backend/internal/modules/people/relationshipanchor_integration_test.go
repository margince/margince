// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The generic relationship surface against a real database, because the rule it
// broke is a SQL predicate and nothing else.
//
// An edge annotates its anchor, so writing one writes that record. The surface
// asked the anchor's OBJECT grant and whether the endpoints were VISIBLE, and on
// an anchor object visibility decides nothing: person, organization, deal and
// project are identity tables, so their owner arm renders TRUE for every seat.
// An ordinary rep could therefore demote another team's real primary employer,
// forge a partner edge on their company, or staff their project — through
// POST/PATCH/DELETE /v1/relationships and through the MCP record verbs that
// reach the same three store methods.
//
// Every test here carries a positive control on a record the caller DOES own.
// A refusal test alone would pass just as happily against a surface that had
// stopped working altogether.

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
)

// edgeAnchorEnv is one workspace holding two reps who share no team, and one record
// of each anchor kind on either side of the boundary. Both reps hold the same
// grants: what separates them is row authority alone, which is the whole of what
// this gate decides.
type edgeAnchorEnv struct {
	store *Store
	owner *pgx.Conn
	ws    ids.UUID
	me    ids.UUID
	them  ids.UUID

	// Mine — owned by the acting rep.
	myPerson ids.PersonID
	myOrg    ids.OrganizationID

	// Theirs, and readable: promoted records the other rep owns. These are the
	// subjects the old gate could not refuse.
	theirPerson  ids.PersonID
	theirOrg     ids.OrganizationID
	theirPartner ids.OrganizationID
	theirProject ids.ProjectID

	// Theirs and UNreadable: a capture-private contact. It is here to prove the
	// added gate did not turn an existence-hiding 404 into a 403 that discloses
	// the record exists.
	theirHiddenPerson ids.PersonID
}

func setupEdgeAnchor(t *testing.T) *edgeAnchorEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	// Before anything else touches this database, for the reason the capture
	// privacy harness gives: testdb.Pool refuses until EnsureSchema has run, and
	// EnsureSchema rebuilds whenever it cannot prove the database is a fresh
	// clone, so a seed written first would be dropped rather than reset.
	if err := testdb.EnsureSchema(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, conn); err != nil {
		t.Fatal(err)
	}

	e := &edgeAnchorEnv{
		owner: conn, ws: ids.NewV7(), me: ids.NewV7(), them: ids.NewV7(),
		myPerson: ids.New[ids.PersonKind](), myOrg: ids.New[ids.OrganizationKind](),
		theirPerson: ids.New[ids.PersonKind](), theirOrg: ids.New[ids.OrganizationKind](),
		theirPartner: ids.New[ids.OrganizationKind](), theirProject: ids.New[ids.ProjectKind](),
		theirHiddenPerson: ids.New[ids.PersonKind](),
	}
	if _, err := conn.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	// No team_membership for either: the reps are on no shared team, so `own`
	// really is own. A shared team would make every refusal below an admission.
	for _, u := range []struct {
		id   ids.UUID
		name string
	}{{e.me, "Acting Rep"}, {e.them, "Other Rep"}} {
		if _, err := conn.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name, status) VALUES ($1, $2, $3, 'active')`,
			u.id, "anchor-"+u.id.String()+"@example.test", u.name); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []struct {
		id         ids.PersonID
		owner      ids.UUID
		visibility string
	}{
		{e.myPerson, e.me, "workspace"},
		{e.theirPerson, e.them, "workspace"},
		{e.theirHiddenPerson, e.them, "owner"},
	} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO person (id, owner_id, visibility, full_name, source, captured_by)
			VALUES ($1, $2, $3, 'Anna Muster', 'manual', 'human:seed')`,
			p.id, p.owner, p.visibility); err != nil {
			t.Fatal(err)
		}
	}
	for _, o := range []struct {
		id    ids.OrganizationID
		owner ids.UUID
		name  string
	}{
		{e.myOrg, e.me, "My Company"},
		{e.theirOrg, e.them, "Their Company"},
		{e.theirPartner, e.them, "Their Partner"},
	} {
		if _, err := conn.Exec(ctx, `
			INSERT INTO organization (id, owner_id, display_name, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'human:seed')`,
			o.id, o.owner, o.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO project (id, owner_id, organization_id, name, source, captured_by)
		VALUES ($1, $2, $3, 'Their Delivery', 'manual', 'human:seed')`,
		e.theirProject, e.them, e.theirOrg); err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))
	return e
}

// as binds one rep at row_scope=own, holding the ordinary grid a seat that
// manages relationships carries. Both reps get the identical grant set, so the
// only thing that ever differs between an admission and a refusal below is who
// owns the anchor row.
func (e *edgeAnchorEnv) as(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"relationship": {Create: true, Read: true, Update: true, Delete: true},
				"person":       {Create: true, Read: true, Update: true},
				"organization": {Create: true, Read: true, Update: true},
				"project":      {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeOwn,
		},
	})
}

// seedEdge writes one edge as the rep who owns its anchor, through the store —
// so the row under test is the row the product writes, not a shape only this
// test knows how to make.
func (e *edgeAnchorEnv) seedEdge(t *testing.T, in CreateRelationshipInput) relationshipRow {
	t.Helper()
	row, err := e.store.CreateRelationship(e.as(e.them), in)
	if err != nil {
		t.Fatalf("seeding a %s edge as its anchor's owner: %v", in.Kind, err)
	}
	return row
}

// liveAndPrimary reads the two facts a hijacked employment edge would move, so
// a refusal is checked against the surviving state rather than the error alone.
func (e *edgeAnchorEnv) liveAndPrimary(t *testing.T, id ids.UUID) (live, primary bool) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT archived_at IS NULL, is_current_primary FROM relationship WHERE id = $1`, id,
	).Scan(&live, &primary); err != nil {
		t.Fatalf("reading the edge back: %v", err)
	}
	return live, primary
}

// Creating an edge is writing its anchor, so it takes the anchor's row
// authority — for every kind, not only the two the dedicated verbs cover.
//
// ErrPermissionDenied exactly, per anchor object. "Not nil" would be satisfied
// by a 404 from a scope miss, by a shape refusal, or by the surface being
// broken, and none of those is the gate under test.
func TestCreatingAnEdgeTakesTheAnchorsRowAuthority(t *testing.T) {
	e := setupEdgeAnchor(t)
	me := e.as(e.me)

	for _, tc := range []struct {
		anchor string
		in     CreateRelationshipInput
	}{
		{"person", CreateRelationshipInput{
			Kind: employmentKind, PersonID: &e.theirPerson, OrganizationID: &e.myOrg,
			IsCurrentPrimary: pointerTo(true), Source: "manual",
		}},
		{"project", CreateRelationshipInput{
			Kind: ProjectStakeholderKind, ProjectID: &e.theirProject, PersonID: &e.myPerson,
			Role: pointerTo("sponsor"), Source: "manual",
		}},
		{"organization", CreateRelationshipInput{
			Kind: "partner_of", OrganizationID: &e.theirOrg, CounterpartyOrgID: &e.theirPartner,
			Source: "manual",
		}},
	} {
		_, err := e.store.CreateRelationship(me, tc.in)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("creating a %s edge anchored on another rep's %s = %v, want ErrPermissionDenied",
				tc.in.Kind, tc.anchor, err)
		}
	}

	var edges int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM relationship`).Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 0 {
		t.Errorf("%d edges were written by a caller with no authority over any anchor", edges)
	}

	// The positive control: the same kind, the same surface, the same grants —
	// on an anchor this rep owns. The gate narrowed the scope; it did not close
	// the surface.
	if _, err := e.store.CreateRelationship(me, CreateRelationshipInput{
		Kind: employmentKind, PersonID: &e.myPerson, OrganizationID: &e.theirOrg,
		IsCurrentPrimary: pointerTo(true), Source: "manual",
	}); err != nil {
		t.Fatalf("an edge on the caller's OWN person was refused: %v", err)
	}
}

// Patching and archiving an edge are writes on its anchor too, and the patch is
// the sharper of the two: it demotes the incumbent primary employer as a side
// effect, so an ungated patch rewrites who the CRM says somebody works for.
func TestPatchingAndArchivingAnEdgeTakeTheAnchorsRowAuthority(t *testing.T) {
	e := setupEdgeAnchor(t)
	me := e.as(e.me)
	edge := e.seedEdge(t, CreateRelationshipInput{
		Kind: employmentKind, PersonID: &e.theirPerson, OrganizationID: &e.theirOrg,
		IsCurrentPrimary: pointerTo(true), Source: "manual",
	})

	if _, err := e.store.UpdateRelationship(me, edge.ID, UpdateRelationshipInput{
		Role: pointerTo("former"),
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("patching another rep's employment edge = %v, want ErrPermissionDenied", err)
	}
	if _, err := e.store.ArchiveRelationship(me, edge.ID, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving another rep's employment edge = %v, want ErrPermissionDenied", err)
	}
	// The stage-time refusal answers the same way, or an approval could be
	// staged against a write the store was always going to refuse.
	if err := e.store.RefuseArchiveRelationship(me, edge.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the archive's stage-time probe = %v, want ErrPermissionDenied", err)
	}

	// What the refusal was FOR: the edge is untouched, and this person still
	// works where they work.
	live, primary := e.liveAndPrimary(t, edge.ID)
	if !live || !primary {
		t.Errorf("edge live=%t primary=%t after three refused writes; want it exactly as its owner left it", live, primary)
	}

	// Positive control on both verbs, on an anchor the caller owns.
	mine := e.seedEdgeAsMe(me, t)
	if _, err := e.store.UpdateRelationship(me, mine.ID, UpdateRelationshipInput{Role: pointerTo("founder")}); err != nil {
		t.Fatalf("patching an edge on the caller's own person was refused: %v", err)
	}
	if _, err := e.store.ArchiveRelationship(me, mine.ID, nil); err != nil {
		t.Fatalf("archiving an edge on the caller's own person was refused: %v", err)
	}
}

// seedEdgeAsMe writes the acting rep's own employment edge, which is the
// positive control's subject.
func (e *edgeAnchorEnv) seedEdgeAsMe(me context.Context, t *testing.T) relationshipRow {
	t.Helper()
	row, err := e.store.CreateRelationship(me, CreateRelationshipInput{
		Kind: employmentKind, PersonID: &e.myPerson, OrganizationID: &e.myOrg,
		IsCurrentPrimary: pointerTo(true), Source: "manual",
	})
	if err != nil {
		t.Fatalf("seeding the caller's own edge: %v", err)
	}
	return row
}

// The added arm narrows authority; it must not widen disclosure. A subject the
// caller cannot SEE at all still answers not-found, because 403 on an invisible
// record says the record is there.
func TestAnAnchorTheCallerCannotSeeStillAnswersNotFound(t *testing.T) {
	e := setupEdgeAnchor(t)

	_, err := e.store.CreateRelationship(e.as(e.me), CreateRelationshipInput{
		Kind: employmentKind, PersonID: &e.theirHiddenPerson, OrganizationID: &e.myOrg,
		Source: "manual",
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("anchoring an edge on a capture-private contact = %v, want ErrNotFound — a 403 "+
			"would confirm the record exists to a caller who may not know it does", err)
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Error("the write-authority arm overtook the visibility arm, and existence-hiding is gone")
	}
}

// pointerTo is the optional-field spelling this file needs across three types.
// The package's own `ptr` takes a string only, and widening it would touch every
// caller of it for the sake of one test file.
func pointerTo[T any](v T) *T { return &v }
