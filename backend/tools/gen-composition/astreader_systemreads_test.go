// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// systemReadsUnitSource is a unit declaring the given SystemReads elements.
func systemReadsUnitSource(reads string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		SystemReads: []extension.SystemRead{` + reads + `},
	}
}
`
}

// The manifest publishes what a unit reads, sorted, so an operator sees it
// without opening the source.
func TestSystemReadsDeriveIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", systemReadsUnitSource("extension.SystemReadDirectory, extension.SystemReadDeals"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "\"system_reads\": [\n    \"deals\",\n    \"directory\"\n  ]"; !strings.Contains(string(derived), want) {
		t.Errorf("derived manifest misses %s:\n%s", want, derived)
	}
}

func TestASystemReadNoGroupNamesIsRefusedAtGeneration(t *testing.T) {
	if _, err := deriveSynthetic(t, "x", systemReadsUnitSource(`"contacts"`)); err == nil {
		t.Error("the generator published a system read that names no group")
	}
}
