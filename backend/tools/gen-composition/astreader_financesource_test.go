// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// financeUnitSource is a unit declaring the given FinanceSources elements.
func financeUnitSource(entries string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		FinanceSources: []extension.FinanceSource{
` + entries + `
		},
	}
}
`
}

func financeEntry(provider string) string {
	return "\t\t\t{Provider: \"" + provider + "\", Check: check, Customers: customers, InvoicesFor: invoices},\n"
}

func TestFinanceSourcesDeriveIntoManifestSorted(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", financeUnitSource(financeEntry("zbooks")+financeEntry("abooks")))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	if !strings.Contains(s, `"finance_sources"`) {
		t.Fatalf("the manifest misses finance_sources:\n%s", s)
	}
	if strings.Index(s, "abooks") > strings.Index(s, "zbooks") {
		t.Errorf("finance sources are not sorted by provider:\n%s", s)
	}
}

func TestAFinanceSourceDeclaredTwiceIsRefused(t *testing.T) {
	_, err := deriveSynthetic(t, "x", financeUnitSource(financeEntry("abooks")+financeEntry("abooks")))
	if err == nil || !strings.Contains(err.Error(), "declared twice") {
		t.Fatalf("err = %v, want the duplicate-provider refusal", err)
	}
}

// Generation refuses what the boot would, through the same published Validate.
func TestAFinanceSourceTheBootWouldRefuseIsRefusedAtGeneration(t *testing.T) {
	for name, entry := range map[string]string{
		"kebab provider":   financeEntry("a-books"),
		"no InvoicesFor":   "\t\t\t{Provider: \"abooks\", Check: check, Customers: customers},\n",
		"a nil Check":      "\t\t\t{Provider: \"abooks\", Check: nil, Customers: customers, InvoicesFor: invoices},\n",
		"an unknown field": "\t\t\t{Provider: \"abooks\", Check: check, Customers: customers, InvoicesFor: invoices, Write: w},\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", financeUnitSource(entry)); err == nil {
				t.Fatal("generation accepted a finance source the boot refuses")
			}
		})
	}
}
