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
		{"a role that is not a system role", func(v *Verb) {
			v.RbacDefaults = []RbacDefault{{Role: "field_partner", Actions: []RbacAction{RbacRead}}}
		}, "not a system role"},
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

func TestDefaultGrantsAreComparedAsSets(t *testing.T) {
	a := []RbacDefault{
		{Role: "admin", Actions: []RbacAction{RbacRead, RbacCreate}},
		{Role: "rep", Actions: []RbacAction{RbacRead}},
	}
	reordered := []RbacDefault{
		{Role: "rep", Actions: []RbacAction{RbacRead}},
		{Role: "admin", Actions: []RbacAction{RbacCreate, RbacRead}},
	}
	if !RbacDefaultsEqual(a, reordered) {
		t.Error("the same defaults in another order were read as different")
	}
	narrower := []RbacDefault{{Role: "admin", Actions: []RbacAction{RbacRead}}, {Role: "rep", Actions: []RbacAction{RbacRead}}}
	if RbacDefaultsEqual(a, narrower) {
		t.Error("a narrower default was read as the same")
	}
}

func TestOperationsOnOneObjectMustAgreeOnItsDefaults(t *testing.T) {
	first, second := wellFormed(), wellFormed()
	second.OperationID = "crmDemoRead"
	first.RbacDefaults = []RbacDefault{{Role: "admin", Actions: []RbacAction{RbacRead, RbacCreate}}}
	second.RbacDefaults = []RbacDefault{{Role: "admin", Actions: []RbacAction{RbacCreate, RbacRead}}}
	bare := wellFormed()
	bare.OperationID = "crmDemoPeek"
	if err := CheckRbacDefaultsAgree([]Verb{first, second, bare}); err != nil {
		t.Fatalf("matching blocks and an operation with none were refused: %v", err)
	}
	second.RbacDefaults = []RbacDefault{{Role: "admin", Actions: []RbacAction{RbacRead}}}
	err := CheckRbacDefaultsAgree([]Verb{first, second})
	if err == nil {
		t.Fatal("conflicting defaults for one object were accepted")
	}
	for _, want := range []string{first.RbacObject, first.OperationID, second.OperationID} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
}
