// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// hooksUnitSource is a unit declaring the given hook fields.
func hooksUnitSource(fields string) string {
	return `package x

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

func onWrite(context.Context, extension.Runtime, extension.Write) error { return nil }

func onOperation(context.Context, extension.Runtime, *extension.Operation) error { return nil }

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
` + fields + `
	}
}
`
}

// What an operator needs from a hook is what it intercepts, so the writes and
// operations derive, sorted, and the handler does not.
func TestHooksDeriveIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", hooksUnitSource(`
		WriteHooks: []extension.WriteHook{
			{Entity: "deal", Actions: []string{"update", "create"}, Handle: onWrite},
		},
		OperationHooks: []extension.OperationHook{
			{Operations: []string{"createDeal"}, Handle: onOperation},
		},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"write_hooks": [`, `"deal.create"`, `"deal.update"`, `"operation_hooks": [`, `"createDeal"`} {
		if !strings.Contains(s, want) {
			t.Errorf("derived manifest misses %s:\n%s", want, s)
		}
	}
	if strings.Index(s, "deal.update") < strings.Index(s, "deal.create") {
		t.Errorf("write hooks are not sorted:\n%s", s)
	}
	if strings.Contains(s, "onWrite") || strings.Contains(s, "onOperation") {
		t.Errorf("a handler reached the manifest:\n%s", s)
	}
}

func TestAHookMissingAFieldIsRefusedByName(t *testing.T) {
	_, err := deriveSynthetic(t, "x", hooksUnitSource(`
		WriteHooks: []extension.WriteHook{
			{Actions: []string{"create"}, Handle: onWrite},
		},`))
	if err == nil || !strings.Contains(err.Error(), "WriteHook needs Entity") {
		t.Fatalf("err = %v, want WriteHook needs Entity", err)
	}
}
