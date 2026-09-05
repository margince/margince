// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"slices"
	"testing"
)

// TestEveryDeclaredFanOutUnitNamesAnArgsKey — ArgsKey is what the sweep-unit
// gauges group on, and a unit answering the empty string is SKIPPED by
// subWorkspaceFanOuts, so every kind declared with it drops silently out of the
// count. Go's switch gives no exhaustiveness against that; this gate does.
//
// The units come from the COMPILED CONTRACT rather than from a list here.
// Enumerating the enum by hand is the failure mode this replaces: a fourth
// constant, or a fourth `fan_out_unit:` in api/jobs.yaml, would not enter a
// hand-written loop, and the gate would stay green about exactly the change it
// claims to catch. Reading what the file declares means the declaration itself
// enrols the new unit.
func TestEveryDeclaredFanOutUnitNamesAnArgsKey(t *testing.T) {
	declaredUnits := map[FanOutUnit]string{}
	for kind, spec := range specs {
		if spec.FanOutTo != "" {
			declaredUnits[spec.FanOutUnit] = kind
		}
	}
	if len(declaredUnits) < 2 {
		t.Fatalf("only %d distinct fan-out unit(s) are declared; this gate would pass on a single grain and prove nothing about a second", len(declaredUnits))
	}

	keys := map[string]FanOutUnit{}
	for unit, declaringKind := range declaredUnits {
		key := unit.ArgsKey()
		if key == "" {
			t.Errorf("%s declares fan-out unit %d, which names no args key — every child of it would be grouped on nothing and vanish from the unit gauges", declaringKind, unit)
			continue
		}
		if other, taken := keys[key]; taken {
			t.Errorf("units %d and %d both name %q; two grains grouped on one key report one number for both", other, unit, key)
		}
		keys[key] = unit
	}
	// The zero unit is a kind that fans out to nothing, and it must answer no
	// key at all rather than default to the workspace's — a default would make
	// a missing declaration read as a declared one.
	if key := FanOutUnit(0).ArgsKey(); key != "" {
		t.Errorf("the zero fan-out unit answers %q; a kind that fans out to nothing has no unit to name", key)
	}
}

// TestFanOutUnitsReadsTheEdgeBackwards — the unit is declared on the
// DISPATCHER, so every consumer holding a child row needs the edge walked for
// it. A map built the other way round (child → its own zero unit) would answer
// plausibly and wrongly for every kind.
func TestFanOutUnitsReadsTheEdgeBackwards(t *testing.T) {
	units := FanOutUnits()
	if len(units) == 0 {
		t.Fatal("no fan-out edges at all — every check below would pass by iterating nothing")
	}
	for _, spec := range specs {
		if spec.FanOutTo == "" {
			if _, present := units[spec.Kind]; present && spec.Role == Dispatcher {
				t.Errorf("%s fans out to nothing but is answered as a fan-out child", spec.Kind)
			}
			continue
		}
		got, present := units[spec.FanOutTo]
		if !present {
			t.Errorf("%s declares it fans out to %s, which is not answered as a fan-out child", spec.Kind, spec.FanOutTo)
			continue
		}
		if got != spec.FanOutUnit {
			t.Errorf("%s stands for unit %d, want %d — the dispatcher %s declares it",
				spec.FanOutTo, got, spec.FanOutUnit, spec.Kind)
		}
	}
}

// TestSubWorkspaceFanOutsSelectsExactlyTheFinerGrains — the unit pair reports a
// kind only where its grain differs from the workspace pair's. A
// workspace-grained kind read here would restate margince_sweep_workspaces_*
// value for value, which is one number published twice; a finer-grained kind
// NOT read here stays masked by a healthy sibling, which is the whole defect
// the pair exists to remove.
//
// The two families still overlap — a per-connection kind's rows carry a
// workspace id too, so it is reported by both, at two grains. That is
// deliberate and is why the selection is asserted rather than assumed.
func TestSubWorkspaceFanOutsSelectsExactlyTheFinerGrains(t *testing.T) {
	kinds, argsKeys := subWorkspaceFanOuts()
	if len(kinds) != len(argsKeys) {
		t.Fatalf("%d kinds against %d args keys; the arrays are joined by index and would pair a kind with another's key",
			len(kinds), len(argsKeys))
	}
	if !slices.IsSorted(kinds) {
		t.Errorf("the kinds are unsorted (%v); a scrape target's series order must not flap between reads", kinds)
	}

	var workspaceGrain, finerGrain int
	for kind, unit := range FanOutUnits() {
		if unit == FanOutWorkspace {
			workspaceGrain++
			if slices.Contains(kinds, kind) {
				t.Errorf("%s fans out per workspace but is read by the unit pair, where it would restate margince_sweep_workspaces exactly", kind)
			}
			continue
		}
		finerGrain++
		i := slices.Index(kinds, kind)
		if i < 0 {
			t.Errorf("%s fans out per unit %d but the unit pair does not read it, so a failed unit stays masked by a healthy sibling", kind, unit)
			continue
		}
		if argsKeys[i] != unit.ArgsKey() {
			t.Errorf("%s is grouped on %q, want its declared unit's key %q", kind, argsKeys[i], unit.ArgsKey())
		}
	}
	// The FINER side must be exercised by real declarations: it is the side the
	// loop above actually checks, and an empty one would mean this gate passed
	// on nothing.
	if finerGrain == 0 {
		t.Fatalf("no finer-grain fan-outs declared; the partition's checked side is unexercised")
	}
	// The WORKSPACE side no longer is, and that is the collapse rather than a
	// gap. ADR-0103 retired every workspace fan-out: a scheduled pass walks the
	// tenants itself, so no kind declares FanOutWorkspace any more. The unit
	// survives — an extension may still declare one (compose/extjobs.go) — so
	// the partition still has to hold, and it is exercised below against a
	// table this test builds rather than against a live declaration that no
	// longer exists.
	if workspaceGrain != 0 {
		t.Errorf("%d workspace-grain fan-out(s) declared; ADR-0103 retired them, so a new one is a decision somebody should have to make deliberately", workspaceGrain)
	}
}

// The workspace side of the partition, against a table this test builds.
//
// It stopped being reachable from the live declarations when ADR-0103 retired
// the workspace fan-outs, and the rule it proves did not stop mattering: a
// workspace-grain fan-out must NOT be read by the unit pair, because
// margince_sweep_workspaces already states exactly that number and a second
// series carrying the same one is two answers to one question.
func TestAWorkspaceGrainFanOutIsNotReadByTheUnitPair(t *testing.T) {
	t.Parallel()

	kinds, _ := subWorkspaceFanOutsOf(map[string]FanOutUnit{
		"a_workspace_child":  FanOutWorkspace,
		"a_connection_child": FanOutConnection,
	})
	if slices.Contains(kinds, "a_workspace_child") {
		t.Error("a workspace-grain fan-out reached the unit pair, where it would restate margince_sweep_workspaces exactly")
	}
	if !slices.Contains(kinds, "a_connection_child") {
		t.Error("a connection-grain fan-out was dropped from the unit pair, so a failed connection would stay masked by a healthy sibling")
	}
}
