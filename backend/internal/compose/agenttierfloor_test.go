// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A tier the contract declares binds every door that can reach the effect.
//
// #982 was not a missing line; it was a vocabulary mismatch nothing could see.
// The contract tightens per OPERATION, a tool declares per VERB, and where one
// verb serves seven record types the tightening had nowhere to live on the tool
// door — so `createProject` was confirm-first on one door and auto-execute on the
// other, and no test failed.
//
// These gates are derived from the generated policy table rather than from a list
// of today's affected pairs. A new `x-mcp-tool` annotation that tightens a record
// type a generic verb already performs regenerates that table and fails here until
// the floor and the staging behind it exist.
//
// What they do NOT prove is that the floor is WIRED into the served registry, or
// that it runs before admission. No unit test in this package can: the floor is a
// function the composition passes and the tier is decided inside Admit. The
// integration lane proves both, for a create and for an update — see
// agenttierfloor_integration_test.go.

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/ports/mcp"
)

// tightenedPairs are the (verb, record type) pairs whose tier the contract
// tightens BEYOND what the verb itself declares, for an effect that verb actually
// performs. That is the whole subject of #982 and the set these gates walk.
//
// Both filters matter. A pair whose verb is already confirm-first needs no floor.
// A route that is not the record's own collection route — a fact write, a
// stakeholder membership, a retire action — is a different operation borrowing
// this verb's annotation, and inheriting its tightening would stage every ordinary
// patch of that record type. A record type the verb cannot serve is not a tightening
// at all: the call is refused at the provider, and staging it first would spend a
// human's yes on a call that was never going to run.
func tightenedPairs(t *testing.T) map[toolRecordType]string {
	t.Helper()
	registry := NewRegistry(nil, SendPath{})
	routes := routesPerToolRecordType()
	pairs := map[toolRecordType]string{}
	for route, pol := range agentPolicies {
		if pol.Access != accessTool || pol.RecordType == "" || pol.Tier != tierConfirmationRequired {
			continue
		}
		spec, registered := registry.Spec(pol.Tool)
		if !registered || spec.Tier == mcp.TierConfirmationRequired {
			continue
		}
		if routes[toolRecordType{tool: pol.Tool, recordType: string(pol.RecordType)}] > 1 &&
			!isCanonicalRecordRoute(route) {
			continue
		}
		if !registry.Performs(pol.Tool, string(pol.RecordType)) {
			continue
		}
		pairs[toolRecordType{tool: pol.Tool, recordType: string(pol.RecordType)}] = route
	}
	return pairs
}

func TestEveryTierTheContractTightensReachesTheToolDoor(t *testing.T) {
	// An empty set is now a legitimate state, not a broken derivation. The
	// verbs that used to be floored per record type execute directly by
	// default, so today's contract tightens none of the canonical record
	// routes — and the MECHANISM this gate protects is what makes that safe:
	// an installation that wants a verb confirmed declares it, and the floor
	// must still reach the tool door. The two gates below assert the mechanism
	// holds for whatever set exists, which is the property #982 is about.
	pairs := tightenedPairs(t)
	for pair, route := range pairs {
		if _, declared := tierFloor(pair.tool, pair.recordType); !declared {
			t.Errorf("%s tightens %s to confirm-first for record type %q, and the tool door's floor "+
				"does not know it — an agent refused on that route performs the same write by calling "+
				"the verb instead", route, pair.tool, pair.recordType)
		}
	}
}

// A floor with nowhere to land is a refusal wearing an approval's clothes: the
// gate answers "this needs a human" and no inbox row is ever minted, so the
// action is not confirm-first at all — it is impossible, which is a different
// promise from the one the contract made.
func TestEveryVerbTheFloorTightensCanStage(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	for pair, route := range tightenedPairs(t) {
		if !registry.Stageable(pair.tool) {
			t.Errorf("%s tightens %s to confirm-first for %q, but %s cannot stage — the refusal "+
				"would have nowhere to land, so the call is refused outright rather than put to a human",
				route, pair.tool, pair.recordType, pair.tool)
		}
	}
}

// The floor is only reachable through a tool that can say which record type a
// call names. A verb the contract tightens per record type whose tool cannot
// answer that question would take the floor for no call at all — every other gate
// green, and the bypass still open.
func TestEveryVerbTheFloorTightensNamesItsRecordType(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	for pair, route := range tightenedPairs(t) {
		if !registry.NamesRecordType(pair.tool) {
			t.Errorf("%s tightens %s for record type %q, but %s does not report the record type of "+
				"a call, so the floor is never consulted for it", route, pair.tool, pair.recordType, pair.tool)
		}
	}
}

// The floor must not reach an operation the verb does not perform. This is the
// regression that shipped in this change's first draft: every confirm-first route
// riding `update_record` for an organization was collapsed onto the pair, so an
// ordinary organization patch — auto-execute on REST, by `updateOrganization` —
// became confirm-first on the tool door. That is #982 pointing the other way, and
// it is invisible to every gate above, all of which only ask whether the floor
// knows ENOUGH.
func TestTheFloorTightensNothingTheContractLeavesAutomatic(t *testing.T) {
	routes := routesPerToolRecordType()
	for route, pol := range agentPolicies {
		if pol.Access != accessTool || pol.RecordType == "" || pol.Tier != tierAutoExecute {
			continue
		}
		if routes[toolRecordType{tool: pol.Tool, recordType: string(pol.RecordType)}] > 1 &&
			!isCanonicalRecordRoute(route) {
			continue
		}
		if _, declared := tierFloor(pol.Tool, string(pol.RecordType)); declared {
			t.Errorf("%s is auto-execute — it IS the operation %s performs for %q — and the floor "+
				"tightens that pair anyway, so the tool door stages what the REST door runs unattended",
				route, pol.Tool, pol.RecordType)
		}
	}
}

// Every verb that CAN stage must also be reachable by a floor.
//
// This is the gate the tier-parity change was missing, and its absence is what
// made a claim in that change's own commit message untrue. Ten consequential
// verbs stopped staging by default — archive, the three sends, the booking, the
// two merges, both lead transitions, the import commit — on the argument that a
// passport carries its holder's own authority, and that an installation wanting
// one confirmed sets a tier floor for it (ADR-0055).
//
// The floor could not reach nine of them. Registry.tightened resolves a floor only
// through recordTypedTool, and only the two generic verbs implemented it: nothing
// else had ever needed to, because nothing else had ever been anything but
// confirm-first. So "an installation can floor it back" was documentation, not
// behaviour, and every gate above stayed green because each one walks the pairs
// the contract tightens TODAY — an empty set.
//
// This walks the other direction. A verb that stages is a verb an installation may
// want confirmed; if the floor cannot resolve a call to it, that wish has no way
// to be expressed, and the default becomes the only setting there is.
func TestEveryStageableVerbCanBeFlooredBack(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	seen := map[string]bool{}
	for _, pol := range agentPolicies {
		if pol.Access != accessTool || pol.RecordType == "" || seen[pol.Tool] {
			continue
		}
		spec, registered := registry.Spec(pol.Tool)
		if !registered || !registry.Stageable(pol.Tool) {
			continue
		}
		seen[pol.Tool] = true
		if spec.Tier == mcp.TierConfirmationRequired {
			// Already confirm-first for every call, so there is nothing a floor
			// could add and nothing an installation has to reach for.
			continue
		}
		if !registry.NamesRecordType(pol.Tool) {
			t.Errorf("%s executes directly and can stage, so an installation may want it confirmed — "+
				"but it does not report the record type of a call, so no tier floor can reach it and "+
				"the default is the only setting there is", pol.Tool)
			continue
		}
		// Whether the verb PERFORMS this particular record type is deliberately not
		// asserted. A route may borrow a verb's annotation for an effect the verb
		// does not have — `replyDealRoomThread` is annotated `create_record` for
		// `deal_room_comment`, which create_record does not create — and
		// Registry.tightened is built to leave such a pair alone rather than stage
		// a call that would die at the provider. What this gate is about is
		// narrower and unconditional: can a floor be resolved for this verb AT
		// ALL.
	}
}
