// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "testing"

type kindDecl = RecordKindDeclaration

func validKind() kindDecl {
	return kindDecl{
		Kind:        "visit",
		DisplayName: "Visit",
		Table:       "ext_visit_log_visit",
		Fields:      []RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site"}},
		NaturalKey:  []string{"code"},
	}
}

func TestRecordKindNameNamesTheDeclaringUnit(t *testing.T) {
	if got := RecordKindName("visit-log", "visit"); got != "ext:visit-log:visit" {
		t.Errorf("RecordKindName = %q", got)
	}
	if got := RecordKindObject("visit-log", "visit"); got != "ext_visit_log_visit" {
		t.Errorf("RecordKindObject = %q", got)
	}
}

func TestParseRecordKindName(t *testing.T) {
	cases := []struct {
		in         string
		unit, kind string
		ok         bool
	}{
		{"ext:visit-log:visit", "visit-log", "visit", true},
		{"ext:a:b_2", "a", "b_2", true},
		{"contact", "", "", false},
		{"ext:", "", "", false},
		{"ext:a", "", "", false},
		{"ext:a:", "", "", false},
		{"ext::b", "", "", false},
		{"ext:A:b", "", "", false},
		{"ext:a:B", "", "", false},
		{"ext:a:1b", "", "", false},
		{"ext:a:b:c", "", "", false},
		{"ext:a:b-c", "", "", false},
		{"ext:a:_b", "", "", false},
		{"ext:a:abcdefghijklmnopqrstuvwxyz0123456", "", "", false},
		{"xext:a:b", "", "", false},
	}
	for _, c := range cases {
		unit, kind, ok := ParseRecordKindName(c.in)
		if ok != c.ok || string(unit) != c.unit || kind != c.kind {
			t.Errorf("ParseRecordKindName(%q) = %q, %q, %v; want %q, %q, %v", c.in, unit, kind, ok, c.unit, c.kind, c.ok)
		}
	}
}

func TestRecordKindValidate(t *testing.T) {
	mutate := func(f func(*kindDecl)) kindDecl {
		k := validKind()
		f(&k)
		return k
	}
	grant := func(role string, a ...RbacAction) []RbacDefault { return []RbacDefault{{Role: role, Actions: a}} }
	cases := []struct {
		name string
		in   kindDecl
		ok   bool
	}{
		{"complete", validKind(), true},
		{"bad kind name", mutate(func(k *kindDecl) { k.Kind = "Visit" }), false},
		{"no display name", mutate(func(k *kindDecl) { k.DisplayName = " " }), false},
		{"no fields", mutate(func(k *kindDecl) { k.Fields = nil }), false},
		{"field twice", mutate(func(k *kindDecl) { k.Fields = append(k.Fields, k.Fields[0]) }), false},
		{"unlabelled field", mutate(func(k *kindDecl) { k.Fields[0].Label = "" }), false},
		{"no natural key", mutate(func(k *kindDecl) { k.NaturalKey = nil }), false},
		{"natural key names no field", mutate(func(k *kindDecl) { k.NaturalKey = []string{"nope"} }), false},
		{"natural key repeats a field", mutate(func(k *kindDecl) { k.NaturalKey = []string{"code", "code"} }), false},
		{"two part natural key", mutate(func(k *kindDecl) { k.NaturalKey = []string{"code", "site"} }), true},
		{"no table", mutate(func(k *kindDecl) { k.Table = "" }), false},
		{"core table", mutate(func(k *kindDecl) { k.Table = "contact" }), false},
		{"other schema", mutate(func(k *kindDecl) { k.Table = "public.ext_visit_log_visit" }), false},
		{"another unit's table", mutate(func(k *kindDecl) { k.Table = "ext_other_unit_visit" }), false},
		{"delete policy unknown", mutate(func(k *kindDecl) { k.DeletePolicy = "shred" }), false},
		{"delete policy archive only", mutate(func(k *kindDecl) { k.DeletePolicy = DeleteArchiveOnly }), true},
		{"default with no role", mutate(func(k *kindDecl) { k.Defaults = grant("", RbacRead) }), false},
		{"default with unknown action", mutate(func(k *kindDecl) { k.Defaults = grant("rep", "drop") }), false},
		{"default grants read", mutate(func(k *kindDecl) { k.Defaults = grant("rep", RbacRead) }), true},
	}
	for _, c := range cases {
		err := c.in.Validate("visit-log")
		if (err == nil) != c.ok {
			t.Errorf("%s: Validate() = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
