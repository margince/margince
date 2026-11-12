// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func linkTypeUnitSource(types string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		LinkTypes: []extension.LinkType{
` + types + `
		},
	}
}
`
}

func TestLinkTypesDeriveIntoTheManifestSortedByType(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", linkTypeUnitSource(
		"\t\t\t{Type: \"renewal_of\", Label: \"Renewal of\", From: []string{\"deal\"}, To: []string{\"deal\"}},\n"+
			"\t\t\t{Type: \"follow_up_of\", Label: \"Follow-up of\"},\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	follow, renewal := strings.Index(s, `"type": "follow_up_of"`), strings.Index(s, `"type": "renewal_of"`)
	if follow < 0 || renewal < 0 || follow > renewal {
		t.Fatalf("the manifest misses a link type or does not sort them by type:\n%s", s)
	}
	if !strings.Contains(s, `"from": [`) {
		t.Errorf("the manifest misses the declared ends:\n%s", s)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestALinkTypeTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, types := range map[string]string{
		"a capital":      "\t\t\t{Type: \"Follow\", Label: \"x\"},\n",
		"no label":       "\t\t\t{Type: \"follow\"},\n",
		"an unknown end": "\t\t\t{Type: \"follow\", Label: \"x\", To: []string{\"project\"}},\n",
		"one type twice": "\t\t\t{Type: \"follow\", Label: \"A\"},\n\t\t\t{Type: \"follow\", Label: \"B\"},\n",
		"unknown field":  "\t\t\t{Type: \"follow\", Label: \"R\", Colour: \"red\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", linkTypeUnitSource(types)); err == nil {
				t.Fatal("generation accepted a link type the boot refuses")
			}
		})
	}
}
