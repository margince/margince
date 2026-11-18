// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// composedOf builds a table declaring specs, failing the test when it is refused.
func composedOf(t *testing.T, declarations ...Spec) Composed {
	t.Helper()
	table, err := NewComposed(declarations, nil)
	if err != nil {
		t.Fatalf("NewComposed: %v", err)
	}
	return table
}

func extSpec(kind string) Spec {
	return Spec{
		Kind:        kind,
		GoType:      "extJobWorkspaceArgs",
		Role:        Worker,
		Queue:       "default",
		Timeout:     TimeoutPolicy{Fixed: time.Minute},
		MaxAttempts: 3,
		OptsOwner:   OptsFanOut,
	}
}

// TestComposedKindsAnswerSpecForLikeCoreOnes is the whole point of the seam: a
// composed kind is as DECLARED as a compiled one, so the wall clock Govern
// hands River, the queue a child lands on and the attempt cap all come from a
// declaration rather than from River's defaults.
func TestComposedKindsAnswerSpecForLikeCoreOnes(t *testing.T) {
	t.Parallel()
	if _, ok := SpecFor("ext_demo_refresh_ws"); ok {
		t.Fatal("the core table answered SpecFor for a composed kind")
	}
	table := composedOf(t, extSpec("ext_demo_refresh_ws"))
	spec, ok := table.SpecFor("ext_demo_refresh_ws")
	if !ok {
		t.Fatal("SpecFor does not answer for a registered composed kind")
	}
	if spec.Timeout.Duration() != time.Minute || spec.MaxAttempts != 3 {
		t.Fatalf("composed spec came back as %+v", spec)
	}
	if err := table.MustBeTotal([]string{"ext_demo_refresh_ws"}); err != nil {
		t.Fatalf("MustBeTotal refused a declared composed kind: %v", err)
	}
	// And the compiled table is still answered — merging must not shadow core.
	if _, ok := table.SpecFor("close_date_sweep"); !ok {
		t.Fatal("a core kind stopped answering once a composed set was registered")
	}
}

// TestDeclaredWalksBothTablesInKindOrder: every consumer that builds a report
// or a metric family iterates Declared, so a composed kind has to appear there
// — and in the one deterministic order, not wherever a map put it.
func TestDeclaredWalksBothTablesInKindOrder(t *testing.T) {
	t.Parallel()
	table := composedOf(t, extSpec("ext_demo_a_ws"), extSpec("ext_demo_b_ws"))
	var kinds []string
	for kind := range table.Declared() {
		kinds = append(kinds, kind)
	}
	if !slices.IsSorted(kinds) {
		t.Fatal("Declared did not walk in kind order")
	}
	for _, want := range []string{"ext_demo_a_ws", "ext_demo_b_ws", "close_date_sweep"} {
		if !slices.Contains(kinds, want) {
			t.Errorf("Declared omitted %s", want)
		}
	}
}

// TestNewComposedRefusesWhatWouldMakeItAShadowingSurface: the seam exists
// to ADD declarations. One that could name a core kind would be a way to change
// a core job's wall clock from a directory an installation dropped in, and one
// that could name anything outside ext_ would not be an extension seam at all.
func TestNewComposedRefusesWhatWouldMakeItAShadowingSurface(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		specs []Spec
		want  string
	}{
		{"outside the namespace", []Spec{extSpec("close_date_sweep_ws")}, "not an extension kind"},
		{"shadowing a core kind", []Spec{{Kind: "ext_demo_x"}, extSpec("close_date_sweep")}, "not an extension kind"},
		{"declared twice", []Spec{extSpec("ext_demo_x"), extSpec("ext_demo_x")}, "declared twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table, err := NewComposed(tc.specs, nil)
			if err == nil {
				t.Fatalf("NewComposed accepted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewComposed(%s) = %v, want a message mentioning %q", tc.name, err, tc.want)
			}
			// Validate-then-apply: a refused set declares none of itself.
			if _, ok := table.SpecFor("ext_demo_x"); ok {
				t.Fatal("a refused set still declared ext_demo_x")
			}
		})
	}
}

// TestNewComposedRefusesACoreKindByName covers the arm the namespace check
// cannot reach: a kind that IS in the ext_ namespace and IS already compiled
// would be a redefinition. No such core kind exists today, so the arm is
// exercised by asking the table directly.
func TestNewComposedRefusesACoreKindByName(t *testing.T) {
	t.Parallel()
	for kind := range specs {
		if IsExtensionKind(kind) {
			if _, err := NewComposed([]Spec{extSpec(kind)}, nil); err == nil {
				t.Fatalf("NewComposed redefined the compiled kind %s", kind)
			}
			return
		}
	}
	// Nothing compiled sits in the extension namespace, which is itself the
	// property worth holding: api/jobs.yaml must never declare an ext_ kind, or
	// the two tables would be competing for one namespace.
	for kind := range specs {
		if strings.HasPrefix(kind, "ext_") {
			t.Fatalf("api/jobs.yaml declares %s in the extension namespace", kind)
		}
	}
}

// TestIsExtensionKindAsksTheNAMESPACENotTheTable: the vanilla build scraping a
// composed database is exactly the case the metric split exists for, and there
// the composed table is empty.
func TestIsExtensionKindAsksTheNAMESPACENotTheTable(t *testing.T) {
	t.Parallel()
	if !IsExtensionKind("ext_absent_unit_job_ws") {
		t.Error("an ext_ kind this build does not compose was not recognised as an extension kind")
	}
	if IsExtensionKind("close_date_sweep") {
		t.Error("a core kind was classified as an extension kind")
	}
	if _, ok := SpecFor("ext_absent_unit_job_ws"); ok {
		t.Fatal("the core table declared an ext_ kind")
	}
}

// TestComposedFanOutUnitsCarryTheirEdge: a child's unit is declared on the
// DISPATCHER, and every consumer walks that edge backwards — so a composed
// dispatcher's edge has to be in the walk too.
func TestComposedFanOutUnitsCarryTheirEdge(t *testing.T) {
	t.Parallel()
	dispatcher := Spec{
		Kind:       "ext_demo_refresh",
		GoType:     "extJobDispatcherArgs",
		Role:       Dispatcher,
		Queue:      "default",
		Timeout:    TimeoutPolicy{Fixed: time.Minute},
		OptsOwner:  OptsArgs,
		FanOutUnit: FanOutWorkspace,
		FanOutTo:   "ext_demo_refresh_ws",
	}
	table := composedOf(t, dispatcher, extSpec("ext_demo_refresh_ws"))
	if got := table.FanOutUnits()["ext_demo_refresh_ws"]; got != FanOutWorkspace {
		t.Fatalf("composed child's fan-out unit: got %v, want workspace", got)
	}
}
