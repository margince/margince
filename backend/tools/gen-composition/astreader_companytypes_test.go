// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func companyTypeUnitSource(types string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		CompanyTypes: []extension.CompanyType{
` + types + `
		},
	}
}
`
}

func TestCompanyTypesDeriveIntoTheManifestSortedByType(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", companyTypeUnitSource(
		"\t\t\t{Type: \"reseller\", Label: \"Reseller\"},\n"+
			"\t\t\t{Type: \"holding\", Label: \"Holding\"},\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	holding, reseller := strings.Index(s, `"type": "holding"`), strings.Index(s, `"type": "reseller"`)
	if holding < 0 || reseller < 0 || holding > reseller {
		t.Fatalf("the manifest misses a company type or does not sort them by type:\n%s", s)
	}
	if !strings.Contains(s, `"label": "Reseller"`) {
		t.Errorf("the manifest misses the label:\n%s", s)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestACompanyTypeTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, types := range map[string]string{
		"a capital":      "\t\t\t{Type: \"Reseller\", Label: \"Reseller\"},\n",
		"no label":       "\t\t\t{Type: \"reseller\"},\n",
		"one type twice": "\t\t\t{Type: \"reseller\", Label: \"A\"},\n\t\t\t{Type: \"reseller\", Label: \"B\"},\n",
		"unknown field":  "\t\t\t{Type: \"reseller\", Label: \"R\", Colour: \"red\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", companyTypeUnitSource(types)); err == nil {
				t.Fatal("generation accepted a company type the boot refuses")
			}
		})
	}
}
