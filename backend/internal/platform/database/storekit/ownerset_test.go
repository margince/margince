// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOwnerSelectionKeepsUnrestrictedEmptyAndUnassignedReadsDistinct(t *testing.T) {
	member := ids.NewV7()
	for _, check := range []struct {
		name   string
		owners *OwnerSet
		want   string
		args   int
	}{
		{"all", nil, "", 0},
		{"none", &OwnerSet{}, "false", 0},
		{"unassigned", &OwnerSet{Unassigned: true}, "(d.owner_id IS NULL)", 0},
		{"named", &OwnerSet{IDs: []ids.UUID{member}}, "(d.owner_id = ANY($1::uuid[]))", 1},
		{"team with unassigned", &OwnerSet{IDs: []ids.UUID{member}, Unassigned: true}, "(d.owner_id = ANY($1::uuid[]) OR d.owner_id IS NULL)", 1},
	} {
		t.Run(check.name, func(t *testing.T) {
			var args []any
			got := OwnerSetClause("d.owner_id", check.owners, func(v any) int { args = append(args, v); return len(args) })
			if got != check.want || len(args) != check.args {
				t.Fatalf("selection=%q, args=%v", got, args)
			}
			if len(args) > 0 && !reflect.DeepEqual(args[0], []ids.UUID{member}) {
				t.Fatalf("owner selection did not bind the selected members: %v", args)
			}
		})
	}
	zero := ids.From[ids.UserKind](ids.Nil)
	got, err := OwnershipClause(&zero, nil, nil, func(any) int { return 1 })
	if err != nil || strings.Contains(got, "IS NULL") {
		t.Fatalf("a zero named owner became unassigned: %q, %v", got, err)
	}
}
