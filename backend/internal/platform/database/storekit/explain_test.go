// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "testing"

// The explained tree folds unknown leaves the way SQL does, so a record whose
// field is unset is explained as neither in nor out of a group that needs it.
func TestAnExplainedGroupFoldsUnknownLeavesAsSQLDoes(t *testing.T) {
	yes, no := new(true), new(false)
	for name, tc := range map[string]struct {
		join   string
		leaves []*bool
		want   *bool
	}{
		"and with an unknown":         {"and", []*bool{yes, nil}, nil},
		"and with a false":            {"and", []*bool{nil, no}, no},
		"or with an unknown and true": {"or", []*bool{nil, yes}, yes},
		"or with an unknown":          {"or", []*bool{no, nil}, nil},
		"or all false":                {"or", []*bool{no, no}, no},
	} {
		t.Run(name, func(t *testing.T) {
			node := ExplainNode{Join: tc.join}
			for _, leaf := range tc.leaves {
				node.Children = append(node.Children, ExplainNode{Result: leaf})
			}
			combine(&node)
			if (node.Result == nil) != (tc.want == nil) || (node.Result != nil && *node.Result != *tc.want) {
				t.Fatalf("folded to %v, want %v", describe(node.Result), describe(tc.want))
			}
		})
	}
}

func describe(b *bool) string {
	switch {
	case b == nil:
		return "unknown"
	case *b:
		return "true"
	default:
		return "false"
	}
}

// A withheld field selects no rows whatever its operator, negated or not.
func TestAWithheldFieldSelectsNothing(t *testing.T) {
	fields := map[string]Field{"tag": {Expr: "tg.tag_id", Type: FieldID, Link: TagLinkTemplate("contact"), Withheld: true}}
	for _, op := range []string{OpEq, OpNeq} {
		arg := func(any) int { return 1 }
		sql, err := CompilePredicate(leaf("tag", op, ownerUUID), fields, arg)
		if err != nil || sql != "FALSE" {
			t.Fatalf("%s on a withheld field compiled to %q, %v; want FALSE", op, sql, err)
		}
	}
}
