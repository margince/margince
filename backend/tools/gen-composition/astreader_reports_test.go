// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func reportUnitSource(body string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
` + body + `
	}
}
`
}

func TestReportDeclarationsDeriveIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", reportUnitSource(`
		ReportCounts: []extension.ReportCount{{Name: "visits", Label: "Visits", Population: "visit"}},
		ReportBreakdowns: []extension.ReportBreakdown{
			{Name: "by_site", Label: "Visits by site", Population: "visit", By: "site"},
		},`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{`"report_counts"`, `"report_breakdowns"`, `"name": "by_site"`, `"by": "site"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the manifest misses %s:\n%s", want, s)
		}
	}
}

func TestAReportEntryTheReaderCannotDeriveIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"a count with a field": `ReportCounts: []extension.ReportCount{{Name: "v", Label: "V", Population: "visit", By: "site"}},`,
		"a computed name":      `ReportCounts: []extension.ReportCount{{Name: name(), Label: "V", Population: "visit"}},`,
		"a name used twice":    `ReportCounts: []extension.ReportCount{{Name: "v", Label: "V", Population: "visit"}, {Name: "v", Label: "W", Population: "visit"}},`,
	} {
		if _, err := deriveSynthetic(t, "x", reportUnitSource(body)); err == nil {
			t.Errorf("%s: generation accepted what it cannot derive", name)
		}
	}
}
