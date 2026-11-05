// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// A bootstrap field is described by its own doc comment, else by the doc of
// the type it holds, each cut to its first paragraph.
func TestBootstrapFieldsAreDescribedByTheirDocComments(t *testing.T) {
	boot := reflect.TypeFor[deployconfig.Bootstrap]()
	docs, err := readFieldDocs("../../internal/platform/deployconfig", boot.PkgPath())
	if err != nil {
		t.Fatal(err)
	}
	workspace, _ := boot.FieldByName("Workspace")
	if got := docs.of(boot, workspace); got == "" || got != docs.docs["Workspace"] {
		t.Errorf("the workspace root is not described by its type's doc: %q", got)
	}
	country, _ := reflect.TypeFor[deployconfig.Workspace]().FieldByName("Country")
	if got := docs.of(reflect.TypeFor[deployconfig.Workspace](), country); got == "" || got != docs.docs["Workspace.Country"] {
		t.Errorf("workspace.country is not described by its own doc: %q", got)
	}
	version, _ := boot.FieldByName("Version")
	if got := docs.of(boot, version); got != "" {
		t.Errorf("an undocumented field of a builtin type got a description: %q", got)
	}
}
