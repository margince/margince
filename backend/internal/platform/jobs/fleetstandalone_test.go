// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"slices"
	"testing"
)

// TestStandaloneFleetKindsSelectsExactlyTheCollapsedPasses — statsBySweep
// must read a Fleet kind that fans out to nothing PER KIND rather than per
// workspace_id, because none of its rows will ever carry one (margince#4983:
// the query filtered every one of them out, forever, and the gauge read
// greenest exactly when such a pass was failing). A Fleet kind that DOES
// declare a FanOutTo must NOT be in this list — its own row still carries
// no workspace, but its children (a different kind) are the real signal,
// and counting the dispatcher's row too would double the pass.
func TestStandaloneFleetKindsSelectsExactlyTheCollapsedPasses(t *testing.T) {
	var dispatchers, standalone int
	for kind, spec := range specs {
		if !spec.Fleet {
			continue
		}
		if spec.FanOutTo != "" {
			dispatchers++
			if slices.Contains(standaloneFleetKinds(), kind) {
				t.Errorf("%s fans out to %s but is read as a standalone pass; its own row would double-count the fleet its children already report", kind, spec.FanOutTo)
			}
			continue
		}
		standalone++
		if !slices.Contains(standaloneFleetKinds(), kind) {
			t.Errorf("%s is a Fleet kind with no fan-out but is missing from standaloneFleetKinds; its tagged row carries no workspace_id and would be silently excluded from every sweep read", kind)
		}
	}
	// Both sides must be exercised by real declarations, or this gate passes
	// on nothing: dispatchers because they are the excluded arm, standalone
	// passes because ADR-0103 made them the common shape.
	if dispatchers == 0 {
		t.Fatal("no Fleet dispatcher declared; the excluded side of the partition is unexercised")
	}
	if standalone == 0 {
		t.Fatal("no standalone Fleet pass declared; the fixed side of the partition is unexercised")
	}
}

// TestStandaloneFleetKindsOfIgnoresANonFleetKind — against a table this test
// builds: a kind that owns a tenant (Fleet: false) is never tagged sweep by
// periodicInsertOpts regardless of whether it happens to declare no
// FanOutTo, so it must never appear here either.
func TestStandaloneFleetKindsOfIgnoresANonFleetKind(t *testing.T) {
	t.Parallel()

	kinds := standaloneFleetKindsOf(map[string]Spec{
		"a_collapsed_pass": {Kind: "a_collapsed_pass", Fleet: true, FanOutTo: ""},
		"a_dispatcher":     {Kind: "a_dispatcher", Fleet: true, FanOutTo: "a_dispatcher_child"},
		"a_tenant_worker":  {Kind: "a_tenant_worker", Fleet: false},
	})
	if !slices.Contains(kinds, "a_collapsed_pass") {
		t.Error("a Fleet kind with no fan-out is missing from the selection")
	}
	if slices.Contains(kinds, "a_dispatcher") {
		t.Error("a Fleet kind that fans out was included; its children carry the real signal, not its own row")
	}
	if slices.Contains(kinds, "a_tenant_worker") {
		t.Error("a non-Fleet kind was included; it is never tagged sweep in the first place")
	}
}
