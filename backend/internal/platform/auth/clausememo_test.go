// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// statementArgs is the registrar production callers pass, seeded with the
// arguments a statement already bound before it asks for the gate.
func statementArgs(already int) (*[]any, func(any) int) {
	args := make([]any, already)
	for i := range args {
		args[i] = i
	}
	return &args, func(v any) int { args = append(args, v); return len(args) }
}

func contentGateReaders() map[string]principal.Principal {
	grants := map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true, Update: true}}
	rep := human(principal.RowScopeOwn)
	rep.Permissions.Objects = grants
	repWithoutContact := rep
	repWithoutContact.Permissions.Objects = map[string]principal.ObjectGrant{"activity": {Read: true}}
	mailbox := ids.NewV7()
	otherTeam := rep
	otherTeam.TeamIDs = []ids.UUID{ids.NewV7()}
	connector := principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: mailbox, OnBehalfOf: mailbox, TeamIDs: []ids.UUID{ids.NewV7(), ids.NewV7()}, Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: grants}}
	forSomeoneElse := connector
	forSomeoneElse.OnBehalfOf = ids.NewV7()
	return map[string]principal.Principal{
		"same rep in another team":   otherTeam,
		"connector for someone else": forSomeoneElse,
		"own scope":                  human(principal.RowScopeOwn),
		"team scope":                 human(principal.RowScopeTeam),
		"all scope":                  human(principal.RowScopeAll),
		"no team":                    {Type: principal.PrincipalHuman, ID: "human:solo", UserID: ids.NewV7(), Permissions: principal.Permissions{RowScope: principal.RowScopeTeam}},
		"rep with grants":            rep,
		"same rep without contact":   repWithoutContact,
		"mailbox connector":          {Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: mailbox, OnBehalfOf: mailbox, TeamIDs: []ids.UUID{ids.NewV7(), ids.NewV7()}, Permissions: principal.Permissions{RowScope: principal.RowScopeOwn, Objects: grants}},
		"agent on a passport":        {Type: principal.PrincipalAgent, ID: "agent:x", UserID: mailbox, PassportID: ids.NewV7(), OnBehalfOf: mailbox, Permissions: principal.Permissions{RowScope: principal.RowScopeTeam, Objects: grants}},
		"system":                     {Type: principal.PrincipalSystem, ID: "system", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}},
	}
}

type rememberedGate struct {
	remembered func(ctx context.Context, alias string, arg func(any) int) (string, error)
	built      clauseBuilder
}

// The memo must be invisible. Every reader, alias and argument offset gets the
// text and argument list a direct build gives, on a miss and on a hit. No
// reader receives a neighbour's rendering.
func TestTheRememberedActivityGatesAreTheBuiltOnes(t *testing.T) {
	gates := map[string]rememberedGate{
		"content":  {ActivityContentClause, contentGateFor},
		"discover": {ActivityDiscoverClause, discoverGateFor},
	}
	for gateName, gate := range gates {
		t.Run(gateName, func(t *testing.T) { checkRememberedGate(t, gate) })
	}
}

func checkRememberedGate(t *testing.T, gate rememberedGate) {
	t.Helper()
	for round := range 2 {
		for name, p := range contentGateReaders() {
			for _, alias := range []string{"a", "readable", "a$1000000001", "a$1000000099"} {
				for _, already := range []int{0, 1, 7} {
					wantArgs, wantArg := statementArgs(already)
					want := gate.built(p, alias, wantArg)
					gotArgs, gotArg := statementArgs(already)
					got, err := gate.remembered(principal.WithActor(context.Background(), p), alias, gotArg)
					if err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Fatalf("round %d, %s, alias %s, %d bound: the remembered gate differs from the built one\ngot:  %s\nwant: %s", round, name, alias, already, got, want)
					}
					if !reflect.DeepEqual(*gotArgs, *wantArgs) {
						t.Fatalf("round %d, %s, alias %s, %d bound: arguments differ\ngot:  %v\nwant: %v", round, name, alias, already, *gotArgs, *wantArgs)
					}
				}
			}
		}
	}
}
