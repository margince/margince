// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// withDefaults gates the shared fixture operation on ext_u_widget and gives it
// the default-grant block under test.
func withDefaults(block string) string {
	return strings.Replace(yogiOperation, "      operationId: uQuote",
		"      operationId: uQuote\n      x-rbac-object: ext_u_widget\n      x-rbac-action: read\n"+block, 1)
}

func TestDefaultGrantsAreReadInRoleOrderWhateverTheFragmentOrder(t *testing.T) {
	block := "      x-rbac-defaults:\n" +
		"        rep: [read, create]\n" +
		"        admin: [read, create, update, delete]\n"
	got, err := verbsInContract("crm.yaml", oneUnit(), contractWith(withDefaults(block)))
	if err != nil {
		t.Fatal(err)
	}
	want := []extension.RbacDefault{
		{Role: "admin", Actions: []extension.RbacAction{extension.RbacRead, extension.RbacCreate, extension.RbacUpdate, extension.RbacDelete}},
		{Role: "rep", Actions: []extension.RbacAction{extension.RbacRead, extension.RbacCreate}},
	}
	// Sorted, because they are emitted into generated Go and hashed into the
	// manifest: a reflowed fragment must not look like a changed declaration.
	if !slices.EqualFunc(got[0].verb.RbacDefaults, want, func(a, b extension.RbacDefault) bool {
		return a.Role == b.Role && slices.Equal(a.Actions, b.Actions)
	}) {
		t.Fatalf("RbacDefaults = %+v, want %+v", got[0].verb.RbacDefaults, want)
	}
}

func TestDefaultGrantsTheGeneratorRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		pathItem string
		wantErr  string
	}{
		"an action outside the four verbs": {
			pathItem: withDefaults("      x-rbac-defaults:\n        rep: [write]\n"),
			wantErr:  `"write"`,
		},
		"defaults on an operation that gates on no object": {
			pathItem: strings.Replace(yogiOperation, "      operationId: uQuote",
				"      operationId: uQuote\n      x-rbac-defaults:\n        rep: [read]", 1),
			wantErr: "no RBAC object",
		},
		"a role given no action": {
			pathItem: withDefaults("      x-rbac-defaults:\n        rep: []\n"),
			wantErr:  "no action",
		},
		"a block that is not a role map": {
			pathItem: withDefaults("      x-rbac-defaults: [read]\n"),
			wantErr:  "x-rbac-defaults",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := verbsInContract("crm.yaml", oneUnit(), contractWith(tc.pathItem))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestTheFragmentHashCoversTheDefaultGrants(t *testing.T) {
	read := func(block string) string {
		got, err := verbsInContract("crm.yaml", oneUnit(), contractWith(withDefaults(block)))
		if err != nil {
			t.Fatal(err)
		}
		return got[0].fragmentHash
	}
	// An operator resolution recorded against one starting posture must not
	// carry to a fragment that widens it.
	narrow := read("      x-rbac-defaults:\n        rep: [read]\n")
	wide := read("      x-rbac-defaults:\n        rep: [read, create]\n")
	if narrow == wide {
		t.Fatal("widening a default grant left the fragment hash unchanged")
	}
}
