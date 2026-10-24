// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The composed-extension inventory's join, exercised without a request: the
// handler is three lines of transport over composedExtensionInventory, and it
// is the JOIN that can be wrong.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// inventoryFixture is the shape a real composed set has: two units, one owning
// records and running a job, one owning neither — which is the common case and
// the one whose empty lists must not come back nil.
func inventoryFixture() ([]extension.Extension, []extension.Verb, []composedJob, map[string]string) {
	exts := []extension.Extension{
		{Name: "notes", Version: "1.0.0"},
		{Name: "de", Version: "0.1.0"},
	}
	verbs := []extension.Verb{
		{Unit: "notes", Route: "/ext/notes/add", Method: "POST", RbacObject: "ext_notes_note"},
		{Unit: "notes", Route: "/ext/notes/list", Method: "GET", RbacObject: "ext_notes_note"},
		// Same path, second method: two operations, and both must be reported —
		// collapsing them would hide a destructive verb behind a harmless one.
		{Unit: "notes", Route: "/ext/notes/list", Method: "DELETE", RbacObject: "ext_notes_note"},
		{Unit: "notes", Route: "/ext/notes/sign", Method: "POST", RbacObject: "ext_notes_signing_key"},
		// A unit that owns no records declares no object; its entry must still
		// carry an empty list rather than a null one.
		{Unit: "de", Route: "/ext/de/probe", Method: "GET"},
	}
	jobs := []composedJob{{decl: extension.JobDeclaration{
		Unit: "notes", Job: "heartbeat", Cadence: time.Minute,
	}}}
	// notes finished its install step; de has not.
	return exts, verbs, jobs, map[string]string{"notes": "1.0.0"}
}

// A unit is ready only at the composed version. No record, or a record for
// another version, is pending.
func TestExtensionInventoryReportsReadyOnlyForTheComposedVersion(t *testing.T) {
	exts, verbs, jobs, _ := inventoryFixture()
	status := func(activated map[string]string) map[string]crmcontracts.ComposedExtensionStatus {
		out := map[string]crmcontracts.ComposedExtensionStatus{}
		for _, u := range composedExtensionInventory(exts, verbs, jobs, activated) {
			out[u.Name] = u.Status
		}
		return out
	}
	got := status(map[string]string{"notes": "1.0.0"})
	if got["notes"] != crmcontracts.ComposedExtensionStatusReady || got["de"] != crmcontracts.ComposedExtensionStatusPending {
		t.Errorf("status = %v, want notes ready and de pending", got)
	}
	if got := status(map[string]string{"notes": "0.9.0"}); got["notes"] != crmcontracts.ComposedExtensionStatusPending {
		t.Errorf("an older activation reads %q, want pending", got["notes"])
	}
}

func TestExtensionInventoryGroupsEveryContributionUnderItsDeclaringUnit(t *testing.T) {
	got := composedExtensionInventory(inventoryFixture())

	if len(got) != 2 {
		t.Fatalf("inventory has %d units, want 2", len(got))
	}
	// Sorted by name, not by composition order: "de" was declared second.
	if got[0].Name != "de" || got[1].Name != "notes" {
		t.Fatalf("units = %q, %q; want them sorted by name", got[0].Name, got[1].Name)
	}
	notes := got[1]
	if notes.Version != "1.0.0" {
		t.Errorf("version = %q, want the unit's declared 1.0.0", notes.Version)
	}
	// De-duplicated (one object on three operations) and sorted.
	wantObjects := []string{"ext_notes_note", "ext_notes_signing_key"}
	if !slices.Equal(notes.RbacObjects, wantObjects) {
		t.Errorf("rbac_objects = %v, want %v", notes.RbacObjects, wantObjects)
	}
	// Sorted by path, then by method — so one path's verbs stay adjacent rather
	// than scattering across the unit.
	wantRoutes := []crmcontracts.ComposedExtensionRoute{
		{Method: "POST", Path: "/ext/notes/add"},
		{Method: "DELETE", Path: "/ext/notes/list"},
		{Method: "GET", Path: "/ext/notes/list"},
		{Method: "POST", Path: "/ext/notes/sign"},
	}
	if !slices.Equal(notes.Routes, wantRoutes) {
		t.Errorf("routes = %v, want %v", notes.Routes, wantRoutes)
	}
	if !slices.Equal(notes.Jobs, []string{"heartbeat"}) {
		t.Errorf("jobs = %v, want [heartbeat]", notes.Jobs)
	}
}

// A unit contributing nothing must come back with EMPTY lists, never nil: `[]`
// on the wire says "this unit contributes none", `null` says nothing at all,
// and a client rendering a grouping UI has to tell those apart.
func TestExtensionInventoryAnswersEmptyRatherThanNullForAUnitThatOwnsNothing(t *testing.T) {
	got := composedExtensionInventory(inventoryFixture())
	de := got[0]
	if de.Name != "de" {
		t.Fatalf("expected the record-less unit first, got %q", de.Name)
	}
	if de.RbacObjects == nil {
		t.Error("rbac_objects is nil; a unit owning no records contributes an empty list")
	}
	if de.Routes == nil {
		t.Error("routes is nil; every unit publishes at least one and the field is never absent")
	}
	if de.Jobs == nil {
		t.Error("jobs is nil; a unit running no job contributes an empty list")
	}
	if len(de.RbacObjects) != 0 || len(de.Jobs) != 0 {
		t.Errorf("de contributed %v / %v, want neither", de.RbacObjects, de.Jobs)
	}
}

// The inventory never attributes another unit's contribution. This is the
// failure the grouping UI would show as "granting notes' object turns on de's
// screen", and the reason unitRbacObjects reads the verb's Unit field rather
// than splitting the ext_<unit>_ prefix — that prefix is not injective
// (extrbac.go), so parsing it back would be a guess.
func TestExtensionInventoryNeverAttributesAnotherUnitsContribution(t *testing.T) {
	// Two unit names that derive ONE namespace token: `crm-demo` underscores to
	// `crm_demo`, which is also `crm`'s own prefix for an object named
	// `demo_widget`. A prefix-splitting implementation gets this wrong.
	exts := []extension.Extension{{Name: "crm", Version: "1"}, {Name: "crm-demo", Version: "1"}}
	verbs := []extension.Verb{
		{Unit: "crm", Route: "/ext/crm/a", Method: "GET", RbacObject: "ext_crm_demo_widget"},
		{Unit: "crm-demo", Route: "/ext/crm-demo/b", Method: "GET", RbacObject: "ext_crm_demo_thing"},
	}
	got := composedExtensionInventory(exts, verbs, nil, nil)

	if !slices.Equal(got[0].RbacObjects, []string{"ext_crm_demo_widget"}) {
		t.Errorf("crm's objects = %v, want only its own", got[0].RbacObjects)
	}
	if !slices.Equal(got[1].RbacObjects, []string{"ext_crm_demo_thing"}) {
		t.Errorf("crm-demo's objects = %v, want only its own", got[1].RbacObjects)
	}
	if !slices.Equal(got[0].Routes, []crmcontracts.ComposedExtensionRoute{{Method: "GET", Path: "/ext/crm/a"}}) {
		t.Errorf("crm's routes = %v, want only its own", got[0].Routes)
	}
}

// An installation with no units enabled — every installation but this one —
// answers an empty inventory rather than nil, so the response carries
// `{"extensions": []}` and a client needs no null branch.
func TestExtensionInventoryOfAVanillaInstallationIsEmptyNotNull(t *testing.T) {
	got := composedExtensionInventory(nil, nil, nil, nil)
	if got == nil {
		t.Fatal("inventory is nil; an installation with no extensions has an empty set, not an absent one")
	}
	if len(got) != 0 {
		t.Fatalf("inventory = %v, want empty", got)
	}
}

// The accessor and the boot must agree. RegisterExtensions records the declared
// set, and the inventory reads it back: without this, ComposedExtensions could
// silently answer the previous boot's set (or none) and the handler would report
// an installation that is not the one serving.
func TestRegisterExtensionsRecordsTheSetTheInventoryReadsBack(t *testing.T) {
	t.Cleanup(func() { setComposedExtensions(nil) })
	if err := RegisterExtensions(composableAll([]extension.Extension{{Name: "inv-unit", Version: "9.9.9"}}), nil, nil); err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	got := ComposedExtensions()
	if len(got) != 1 || got[0].Name != "inv-unit" || got[0].Version != "9.9.9" {
		t.Fatalf("ComposedExtensions() = %v, want the set RegisterExtensions was handed", got)
	}
}

// The list is gated by a grant, not by the admin role. The ops role reads it
// through the same grant.
func TestExtensionListAnswersTheHolderOfTheReadGrantAndRefusesTheRest(t *testing.T) {
	t.Parallel()
	as := func(grants map[string]principal.ObjectGrant) int {
		ctx := principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: grants},
		})
		rec := httptest.NewRecorder()
		extensionsHandlers{activated: func(context.Context) (map[string]string, error) { return map[string]string{}, nil }}.ListExtensions(rec, httptest.NewRequest(http.MethodGet, "/v1/extensions", nil).WithContext(ctx))
		return rec.Code
	}
	if got := as(map[string]principal.ObjectGrant{"extension_access": {Read: true}}); got != http.StatusOK {
		t.Errorf("a holder of extension_access.read → %d, want 200", got)
	}
	if got := as(map[string]principal.ObjectGrant{"job_health": {Read: true}}); got != http.StatusForbidden {
		t.Errorf("a caller without extension_access → %d, want 403", got)
	}
}
