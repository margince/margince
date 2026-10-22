// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestSourceKeyClausesNarrowByWhicheverHalvesAreSent(t *testing.T) {
	for name, tc := range map[string]struct {
		system, id *string
		wantClause []string
	}{
		"neither":     {},
		"system only": {system: new("hubspot"), wantClause: []string{"source_system = $1"}},
		"id only":     {id: new("42"), wantClause: []string{"source_id = $1"}},
		"both":        {system: new("hubspot"), id: new("42"), wantClause: []string{"source_system = $1", "source_id = $2"}},
	} {
		var args []any
		got, err := SourceKeyClauses(tc.system, tc.id, func(v any) int { args = append(args, v); return len(args) })
		if err != nil || strings.Join(got, ",") != strings.Join(tc.wantClause, ",") {
			t.Errorf("%s: clauses %v, err %v; want %v", name, got, err, tc.wantClause)
		}
		if len(args) != len(tc.wantClause) {
			t.Errorf("%s: bound %d args for %d clauses", name, len(args), len(tc.wantClause))
		}
	}
}

func TestSourceKeyClausesRefuseWhatTheCreateWireRefuses(t *testing.T) {
	tooLong := strings.Repeat("x", provenance.SourceIDMax+1)
	for name, args := range map[string][2]*string{
		"a system over the cap": {&tooLong, nil},
		"an empty id":           {nil, new("")},
		"an id over the cap":    {nil, &tooLong},
	} {
		_, err := SourceKeyClauses(args[0], args[1], func(any) int { return 1 })
		if _, ok := errors.AsType[*provenance.SourceIDError](err); !ok {
			t.Errorf("%s = %v, want a *provenance.SourceIDError", name, err)
		}
	}
}
