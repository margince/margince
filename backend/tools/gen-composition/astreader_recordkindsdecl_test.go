// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func recordKindUnitSource(kinds string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		RecordKinds: []extension.RecordKindDeclaration{
` + kinds + `
		},
	}
}
`
}

const visitDecl = `{
	Kind: "visit", DisplayName: "Visit", Table: "ext_x_visit",
	Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site"}},
	NaturalKey: []string{"code"},
	Defaults:   []extension.RbacDefault{{Role: "rep", Actions: []extension.RbacAction{extension.RbacRead}}},
	DeletePolicy: extension.DeleteErasable,
},`

func TestRecordKindDeclarationsDeriveIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", recordKindUnitSource(visitDecl+`
{Kind: "account", DisplayName: "Account", Table: "ext_x_account",
	Fields: []extension.RecordKindField{{Name: "code", Label: "Code"}}, NaturalKey: []string{"code"}},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	account, visit := strings.Index(s, `"kind": "account"`), strings.Index(s, `"kind": "visit"`)
	if account < 0 || visit < 0 || account > visit {
		t.Fatalf("the manifest misses a record kind or does not sort them by kind:\n%s", s)
	}
	for _, want := range []string{`"table": "ext_x_visit"`, `"natural_key"`, `"role": "rep"`, `"read"`, `"delete_policy": "erasable"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestARecordKindTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, kinds := range map[string]string{
		"a capital":        `{Kind: "Visit", DisplayName: "V", Table: "ext_x_v", Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}, NaturalKey: []string{"a"}},`,
		"no natural key":   `{Kind: "visit", DisplayName: "V", Table: "ext_x_v", Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}},`,
		"another table":    `{Kind: "visit", DisplayName: "V", Table: "contact", Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}, NaturalKey: []string{"a"}},`,
		"one kind twice":   visitDecl + visitDecl,
		"unknown field":    `{Kind: "visit", DisplayName: "V", Colour: "red"},`,
		"computed table":   `{Kind: "visit", DisplayName: "V", Table: tableName(), Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}, NaturalKey: []string{"a"}},`,
		"unknown delete":   `{Kind: "visit", DisplayName: "V", Table: "ext_x_v", Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}, NaturalKey: []string{"a"}, DeletePolicy: "shred"},`,
		"unknown role act": `{Kind: "visit", DisplayName: "V", Table: "ext_x_v", Fields: []extension.RecordKindField{{Name: "a", Label: "A"}}, NaturalKey: []string{"a"}, Defaults: []extension.RbacDefault{{Role: "rep", Actions: []extension.RbacAction{"drop"}}}},`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", recordKindUnitSource(kinds)); err == nil {
				t.Fatal("generation accepted a record kind the boot refuses")
			}
		})
	}
}
