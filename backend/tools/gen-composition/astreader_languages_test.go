// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func languageUnitSource(languages string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Languages: []extension.Language{
` + languages + `
		},
	}
}
`
}

func TestLanguagesDeriveIntoTheManifestSortedByCode(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", languageUnitSource(
		"\t\t\t{Code: \"ar\", Label: \"Arabic\"},\n"+
			"\t\t\t{Code: \"fr\", Label: \"French\"},\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	arabic, french := strings.Index(s, `"code": "ar"`), strings.Index(s, `"code": "fr"`)
	if arabic < 0 || french < 0 || arabic > french {
		t.Fatalf("the manifest misses a language or does not sort them by code:\n%s", s)
	}
	if !strings.Contains(s, `"label": "Arabic"`) {
		t.Errorf("the manifest misses the label:\n%s", s)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestALanguageTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, languages := range map[string]string{
		"a capital":        "\t\t\t{Code: \"AR\", Label: \"Arabic\"},\n",
		"a spelled name":   "\t\t\t{Code: \"arabic\", Label: \"Arabic\"},\n",
		"no label":         "\t\t\t{Code: \"ar\"},\n",
		"one code twice":   "\t\t\t{Code: \"ar\", Label: \"A\"},\n\t\t\t{Code: \"ar\", Label: \"B\"},\n",
		"an unknown field": "\t\t\t{Code: \"ar\", Label: \"Arabic\", Script: \"arab\"},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", languageUnitSource(languages)); err == nil {
				t.Fatal("generation accepted a language the boot refuses")
			}
		})
	}
}
