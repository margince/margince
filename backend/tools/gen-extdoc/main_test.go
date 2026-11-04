// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"
)

func TestTheRecordTableHasTheDoorsTheCodeOffers(t *testing.T) {
	src, err := parseDir("../../..")
	if err != nil {
		t.Fatal(err)
	}
	d := deriveDoors(src)
	for kind, door := range map[string]int{"deal": doorMove, "activity": doorCreate, "contact": doorNotice} {
		if d[kind] == nil || !d[kind][door] {
			t.Errorf("%s has no %q door", kind, doorLabels[door])
		}
	}
	if d["contact"][doorLand] {
		t.Error("a contact is never landed by a unit")
	}
	if _, found := d["customfield"]; found {
		t.Error("the custom-field catalog is not a record")
	}
}

func TestACensusOfNothingFails(t *testing.T) {
	if err := census(doors{}, nil, nil); err == nil {
		t.Fatal("an empty scan passed the census")
	}
}

func TestSpliceKeepsTheProseAroundTheMarkers(t *testing.T) {
	page := "head\n" + pageBegin + "\nold\n" + pageEnd + "\ntail\n"
	got, err := splice(page, "new\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "head\n" + pageBegin + "\nnew\n" + pageEnd + "\ntail\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := splice("no markers", "x"); err == nil {
		t.Fatal("a page without markers was accepted")
	}
}
