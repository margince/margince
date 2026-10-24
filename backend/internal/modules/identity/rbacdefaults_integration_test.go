// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A unit's starting grants over a real migrated Postgres. A database shows that
// the fill writes the entry the authorization path reads, and writes it once.
// It keeps an admin's revocation and leaves a custom role alone. A new
// installation starts with the grants.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var rolesitDefaults = []RbacDefault{
	{Object: extObject, Role: "admin", Actions: []string{"read", "create", "update", "delete"}},
	{Object: extObject, Role: "rep", Actions: []string{"read", "create"}},
	{Object: extObject, Role: "read_only", Actions: []string{"read"}},
}

// registerRolesitDefaults registers the object and its defaults for one test.
func registerRolesitDefaults(t *testing.T) {
	t.Helper()
	registerExtObject(t)
	if err := RegisterRbacDefaults(rolesitDefaults...); err != nil {
		t.Fatalf("registering the defaults: %v", err)
	}
}

// bootCtx is the context the boot binds for the fill: the workspace and a
// system actor, as compose's bootLedgerScope produces.
func (e *revocationEnv) bootCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws.UUID)
	return principal.SystemActing(ctx, "system:rbac-defaults")
}

// storedObjectGrant reads one role's stored entry for the object; present is
// false when the document has none.
func (e *revocationEnv) storedObjectGrant(t *testing.T, roleKey string) (g storedGrant, present bool) {
	t.Helper()
	var raw []byte
	err := e.owner.QueryRow(context.Background(),
		`SELECT permissions->'objects'->$2::text FROM role WHERE key = $1`, roleKey, extObject).Scan(&raw)
	if err != nil {
		t.Fatalf("reading %s's entry: %v", roleKey, err)
	}
	if raw == nil {
		return storedGrant{}, false
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("decoding %s's entry: %v", roleKey, err)
	}
	return g, true
}

func (e *revocationEnv) reportFence(t *testing.T) int64 {
	t.Helper()
	var generation int64
	if err := e.owner.QueryRow(context.Background(),
		`SELECT generation FROM report_projection_fence`).Scan(&generation); err != nil {
		t.Fatalf("reading the report fence: %v", err)
	}
	return generation
}

func (e *revocationEnv) defaultAuditRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log
		  WHERE entity_type = 'role' AND action = 'update' AND actor_type = 'system'
		    AND after->'objects' ? $1`, extObject).Scan(&n); err != nil {
		t.Fatalf("counting the fill's audit rows: %v", err)
	}
	return n
}

func TestTheFillGivesEachSystemRoleItsDefaultOnceAndAuditsIt(t *testing.T) {
	e := setupRevocationEnv(t, "rbac-defaults-fill")
	// Registered after the installation exists, so the seed did not write them.
	// This is the boot of an installation that predates the unit.
	registerRolesitDefaults(t)

	filled, err := e.svc.FillRbacDefaults(e.bootCtx())
	if err != nil {
		t.Fatalf("FillRbacDefaults: %v", err)
	}
	if filled != len(rolesitDefaults) {
		t.Fatalf("filled %d entries, want %d", filled, len(rolesitDefaults))
	}
	for _, want := range []struct {
		role string
		g    storedGrant
	}{
		{"admin", storedGrant{Read: true, Create: true, Update: true, Delete: true}},
		{"rep", storedGrant{Read: true, Create: true}},
		{"read_only", storedGrant{Read: true}},
	} {
		got, present := e.storedObjectGrant(t, want.role)
		if !present || got != want.g {
			t.Errorf("%s holds %+v (present %v), want %+v", want.role, got, present, want.g)
		}
	}
	// A role the unit named no default for starts with no entry, as before.
	if _, present := e.storedObjectGrant(t, "manager"); present {
		t.Error("manager got an entry though no default named it")
	}
	if got := e.defaultAuditRows(t); got != len(rolesitDefaults) {
		t.Errorf("%d audit rows, want one per entry written (%d)", got, len(rolesitDefaults))
	}

	// A second boot finds every entry present: nothing written, nothing audited.
	// A statement on role bumps the report fence, so a boot that fills nothing
	// must send none.
	fenceBefore := e.reportFence(t)
	again, err := e.svc.FillRbacDefaults(e.bootCtx())
	if err != nil {
		t.Fatalf("second FillRbacDefaults: %v", err)
	}
	if again != 0 {
		t.Errorf("a second boot filled %d entries, want 0", again)
	}
	if got := e.defaultAuditRows(t); got != len(rolesitDefaults) {
		t.Errorf("a second boot left %d audit rows, want still %d", got, len(rolesitDefaults))
	}
	if fenceAfter := e.reportFence(t); fenceAfter != fenceBefore {
		t.Errorf("a boot that filled nothing moved the report fence from %d to %d", fenceBefore, fenceAfter)
	}
}

func TestAnAdminsRevocationSurvivesTheNextBoot(t *testing.T) {
	e := setupRevocationEnv(t, "rbac-defaults-revoked")
	registerRolesitDefaults(t)
	if _, err := e.svc.FillRbacDefaults(e.bootCtx()); err != nil {
		t.Fatalf("first fill: %v", err)
	}

	// The admin turns every verb off for rep. The entry stays, all four verbs
	// false, which is what keeps it from being refilled.
	if _, err := e.svc.SetRoleObjectGrant(e.wsCtx(e.admin), e.admin, "rep", extObject, storedGrant{}, nil); err != nil {
		t.Fatalf("revoking rep's grant: %v", err)
	}
	filled, err := e.svc.FillRbacDefaults(e.bootCtx())
	if err != nil {
		t.Fatalf("second fill: %v", err)
	}
	if filled != 0 {
		t.Fatalf("the next boot filled %d entries over an admin's revocation", filled)
	}
	if got, present := e.storedObjectGrant(t, "rep"); !present || got != (storedGrant{}) {
		t.Fatalf("rep holds %+v (present %v) after the next boot, want the revocation to hold", got, present)
	}
}

func TestACustomRoleAndTheRestOfTheDocumentAreLeftAlone(t *testing.T) {
	e := setupRevocationEnv(t, "rbac-defaults-custom")
	ctx := e.wsCtx(e.admin)
	custom, err := e.svc.CreateRole(ctx, e.admin, "rep", "Field partner")
	if err != nil {
		t.Fatalf("creating a custom role: %v", err)
	}
	// A grant on an object nothing knows about, as a removed unit leaves behind.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE role SET permissions = jsonb_set(permissions, '{objects,ext_gone_thing}', '{"read":true}'::jsonb, true)
		  WHERE key = 'rep'`); err != nil {
		t.Fatalf("planting the orphaned grant: %v", err)
	}
	registerRolesitDefaults(t)

	if _, err := e.svc.FillRbacDefaults(e.bootCtx()); err != nil {
		t.Fatalf("FillRbacDefaults: %v", err)
	}
	var customEntry, orphan bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT jsonb_exists(permissions->'objects', $2::text)
		  FROM role WHERE key = $1`, custom.Key, extObject).Scan(&customEntry); err != nil {
		t.Fatalf("reading the custom role: %v", err)
	}
	if customEntry {
		t.Error("a custom role was given a default; it starts with no grant")
	}
	if err := e.owner.QueryRow(context.Background(),
		`SELECT jsonb_exists(permissions->'objects', 'ext_gone_thing') FROM role WHERE key = 'rep'`).Scan(&orphan); err != nil {
		t.Fatalf("reading rep: %v", err)
	}
	if !orphan {
		t.Error("the fill erased a grant it did not own; the write is not targeted")
	}
}

func TestAnInstallationCreatedWhileTheUnitIsComposedStartsWithItsGrants(t *testing.T) {
	// Registered before the installation, so the seed writes the entries.
	registerRolesitDefaults(t)
	e := setupRevocationEnv(t, "rbac-defaults-seed")

	for _, d := range rolesitDefaults {
		want, err := d.grant()
		if err != nil {
			t.Fatal(err)
		}
		if got, present := e.storedObjectGrant(t, d.Role); !present || got != want {
			t.Errorf("%s was seeded with %+v (present %v), want %+v", d.Role, got, present, want)
		}
	}
	// The seed writes the role documents unaudited, and so are their defaults;
	// the first boot after it must therefore find nothing left to fill.
	filled, err := e.svc.FillRbacDefaults(e.bootCtx())
	if err != nil {
		t.Fatalf("FillRbacDefaults: %v", err)
	}
	if filled != 0 {
		t.Errorf("the boot after a seed filled %d entries, want 0", filled)
	}
}

// A seeded default reads back through the grant editor's own decode, so it is
// not a second spelling of the document.
func TestASeededDefaultIsTheEntryTheEditorReads(t *testing.T) {
	registerRolesitDefaults(t)
	e := setupRevocationEnv(t, "rbac-defaults-editor")

	roles, err := e.svc.ListRoles(e.wsCtx(e.admin), e.admin, false)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	for _, r := range roles {
		if r.Key != "rep" {
			continue
		}
		if got := r.Objects[extObject]; !got.Read || !got.Create || got.Update || got.Delete {
			t.Fatalf("the editor reads rep's default as %+v, want read+create", got)
		}
		return
	}
	t.Fatal("rep is missing from the role list")
}
