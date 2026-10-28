// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"testing"
)

// foldBound is the value the patch binds for column, and whether it binds one.
func foldBound(t *testing.T, p *Patch, column string) (*string, bool) {
	t.Helper()
	i, ok := p.slotBySQLName[column]
	if !ok {
		return nil, false
	}
	bound, isText := p.args[i].(*string)
	if !isText {
		t.Fatalf("the patch binds %s as a %T, want a *string", column, p.args[i])
	}
	return bound, true
}

func TestAPatchStampsTheFoldOfTheAddressItWrites(t *testing.T) {
	p := NewPatch()
	p.Set("email", "ben@old.example", "ben@Bücher.example")
	if err := p.stampFoldedDomain("lead"); err != nil {
		t.Fatal(err)
	}
	if fold, ok := foldBound(t, p, "email_domain_folded"); !ok || fold == nil || *fold != "xn--bcher-kva.example" {
		t.Fatalf("the patch binds %v for the fold, want xn--bcher-kva.example", fold)
	}
	if _, audited := p.After()["email_domain_folded"]; audited {
		t.Error("the fold reached the audit image; a derived column stays out of it")
	}
}

func TestAClearedAddressClearsItsFold(t *testing.T) {
	p := NewPatch()
	p.Set("email", "ben@acme.example", nil)
	if err := p.stampFoldedDomain("lead"); err != nil {
		t.Fatal(err)
	}
	if fold, ok := foldBound(t, p, "email_domain_folded"); !ok || fold != nil {
		t.Fatalf("a cleared address binds the fold %v (staged %v), want NULL", fold, ok)
	}
}

func TestAPatchThatWritesNoAddressStampsNothing(t *testing.T) {
	for _, tc := range []struct{ table, column string }{{"lead", "full_name"}, {"deal", "email"}} {
		p := NewPatch()
		p.Set(tc.column, nil, "x@acme.example")
		if err := p.stampFoldedDomain(tc.table); err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(p.sets, func(set string) bool { return set != tc.column+" = $1" }) {
			t.Errorf("a %s write of %s staged %v, want only its own column", tc.table, tc.column, p.sets)
		}
	}
}

func TestAnAddressOfAnUnknownTypeIsRefused(t *testing.T) {
	p := NewPatch()
	p.Set("email", nil, 42)
	if err := p.stampFoldedDomain("lead"); err == nil {
		t.Fatal("an address bound as an int was folded; it must be refused")
	}
}
