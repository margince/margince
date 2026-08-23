// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"reflect"
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// The promise this file holds: an agent writes exactly what its granting human
// writes, never more. Margince states it to customers as "the AI uses your
// scope", and MCP is governed the same way, so a rep's assistant must not be
// able to change a colleague's records when the rep could not.
//
// The promise rests on one assignment — AgentIdentity.Principal() copying the
// human's Permissions and Teams onto an agent-typed principal — and on every
// store entry point enforcing that principal exactly as it enforces a human's.
// The second half is proved against a real database in
// compose/integration/agentwriteparity_integration_test.go. This file holds the
// first half, which needs no database and so fails in a second rather than a
// minute.

// permissionsFieldCount is the arity Principal() copies wholesale.
//
// It is ASSERTED rather than documented because Principal() assigns the struct
// by value: a field added to Permissions is copied to the agent for free, and
// every test here would stay green while the question that field deserves — may
// an agent inherit this? — went unasked. Bumping this constant is where that
// question gets asked.
const permissionsFieldCount = 4

func TestAnAgentPrincipalCarriesExactlyItsGrantingHumansAuthority(t *testing.T) {
	human := ids.New[ids.UserKind]()
	team := ids.New[ids.TeamKind]()
	perms := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"person": {Read: true, Update: true},
			"deal":   {Read: true},
		},
		RowScope: principal.RowScopeTeam,
		FieldMasks: []principal.FieldMask{
			{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority},
		},
	}
	agent := AgentIdentity{
		PassportID:  ids.New[ids.PassportKind](),
		WorkspaceID: ids.New[ids.WorkspaceKind](),
		OnBehalfOf:  human,
		SeatType:    string(principal.SeatFull),
		Teams:       []ids.TeamID{team},
		Permissions: perms,
	}

	got := agent.Principal()

	if got.Type != principal.PrincipalAgent {
		t.Errorf("principal type = %s, want %s", got.Type, principal.PrincipalAgent)
	}
	if got.UserID != human.UUID || got.OnBehalfOf != human.UUID {
		t.Errorf("principal acts as %s/%s, want the granting human %s on both",
			got.UserID, got.OnBehalfOf, human.UUID)
	}
	if len(got.TeamIDs) != 1 || got.TeamIDs[0] != team.UUID {
		t.Errorf("agent teams = %v, want exactly the human's %v", got.TeamIDs, team.UUID)
	}
	if got.SeatType != principal.SeatFull {
		t.Errorf("agent seat = %s, want the human's %s", got.SeatType, principal.SeatFull)
	}
	// A read seat is the case that matters, and asserting only the full one
	// would pass over a Principal() that minted SeatFull unconditionally:
	// the licensing ceiling refuses a mutation before RBAC is consulted, so an
	// agent that inherited the wrong seat would write where its human cannot.
	readSeat := agent
	readSeat.SeatType = string(principal.SeatRead)
	if seat := readSeat.Principal().SeatType; seat != principal.SeatRead {
		t.Errorf("an agent for a READ-seat human carries seat %s, want %s — the seat ceiling is the "+
			"licensing half of `agent <= human` and is not inherited", seat, principal.SeatRead)
	}
	// DeepEqual on the WHOLE struct, not field by field: a field-by-field
	// assertion passes over a field nobody has thought about yet, which is the
	// case this test exists to catch.
	if !reflect.DeepEqual(got.Permissions, perms) {
		t.Errorf("agent permissions = %+v, want its granting human's %+v", got.Permissions, perms)
	}
}

func TestPermissionsGainingAFieldIsNotInheritedByAnAgentUnasked(t *testing.T) {
	if n := reflect.TypeOf(principal.Permissions{}).NumField(); n != permissionsFieldCount {
		t.Fatalf("principal.Permissions has %d fields, this gate knows %d — a field was added and "+
			"AgentIdentity.Principal() now copies it to the agent for free. Decide whether an agent may "+
			"inherit it, then update permissionsFieldCount to record that the question was asked",
			n, permissionsFieldCount)
	}
}

func TestAnAgentPrincipalCannotWidenTheHumansRowScope(t *testing.T) {
	for _, scope := range []principal.RowScope{
		principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll,
	} {
		agent := AgentIdentity{
			PassportID:  ids.New[ids.PassportKind](),
			OnBehalfOf:  ids.New[ids.UserKind](),
			Permissions: principal.Permissions{RowScope: scope},
		}
		got := agent.Principal()
		if got.Permissions.RowScope != scope {
			t.Errorf("agent for a %s-scoped human resolves to %s", scope, got.Permissions.RowScope)
		}
		// Unbounded is what every read and write path asks before it renders a
		// row predicate at all, so a bounded human whose agent answers true here
		// would have handed the estate over whatever the scope string said.
		if want := scope == principal.RowScopeAll; auth.Unbounded(got) != want {
			t.Errorf("auth.Unbounded(agent for a %s-scoped human) = %v, want %v",
				scope, auth.Unbounded(got), want)
		}
	}
}
