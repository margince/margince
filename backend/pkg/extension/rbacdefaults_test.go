// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"strings"
	"testing"
)

// A default is a starting grant on the object the operation already gates on.
// Everything it can get wrong is refused where the declaration is read, so the
// generator and the boot refuse the same fragments.

func TestVerbValidateAcceptsDefaultsOnTheOperationsOwnObject(t *testing.T) {
	v := wellFormed()
	v.RbacDefaults = []RbacDefault{
		{Role: "admin", Actions: []RbacAction{RbacRead, RbacCreate, RbacUpdate, RbacDelete}},
		{Role: "read_only", Actions: []RbacAction{RbacRead}},
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("defaults on a gated operation must validate: %v", err)
	}
}

func TestVerbValidateRefusesDefaultsThatCannotBeApplied(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Verb)
		contains string
	}{
		{"no object to grant on", func(v *Verb) {
			v.RequestedScope, v.RbacObject, v.RbacAction = ScopeRead, "", ""
			v.RbacDefaults = []RbacDefault{{Role: "rep", Actions: []RbacAction{RbacRead}}}
		}, "no RBAC object"},
		{"an action outside the four verbs", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{{Role: "rep", Actions: []RbacAction{"write"}}}
		}, `"write"`},
		{"a role listed twice", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{
				{Role: "rep", Actions: []RbacAction{RbacRead}},
				{Role: "rep", Actions: []RbacAction{RbacCreate}},
			}
		}, `"rep" twice`},
		{"an action listed twice", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{{Role: "rep", Actions: []RbacAction{RbacRead, RbacRead}}}
		}, `"read" twice`},
		{"a role with no action", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{{Role: "rep"}}
		}, "no action"},
		{"a blank role key", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{{Role: "", Actions: []RbacAction{RbacRead}}}
		}, "no role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := wellFormed()
			tc.mutate(&v)
			err := v.Validate()
			if err == nil {
				t.Fatalf("%s: Validate accepted defaults that no role document could carry", tc.name)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("%s: refusal %q does not say %q — the author must be told what to change", tc.name, err, tc.contains)
			}
		})
	}
}

func TestVerbValidateRefusesDefaultsOnAHumanOnlyOperationWithNoObject(t *testing.T) {
	v := wellFormed()
	v.HumanOnly = true
	v.Tier, v.RequestedScope = "", ""
	v.Method = "GET"
	v.RbacObject, v.RbacAction = "", ""
	v.RbacDefaults = []RbacDefault{{Role: "rep", Actions: []RbacAction{RbacRead}}}
	if err := v.Validate(); err == nil {
		t.Fatal("defaults with no object were accepted on a human-only operation")
	}
}
