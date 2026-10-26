// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy_test

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

func valid() privacy.Table {
	return privacy.Table{
		Name:      "ext_demo_note",
		Links:     []privacy.Link{{Column: "activity_id", Record: privacy.Activity}},
		Personal:  []string{"body"},
		Retention: jurisdiction.CommercialCorrespondence,
	}
}

func TestTableValidate(t *testing.T) {
	cases := map[string]func(*privacy.Table){
		"another unit's namespace":    func(x *privacy.Table) { x.Name = "ext_other_note" },
		"personal, company link only": func(x *privacy.Table) { x.Links = []privacy.Link{{Column: "company_id", Record: privacy.Company}} },
		"personal but no link":        func(x *privacy.Table) { x.Links = nil },
		"unknown record kind":         func(x *privacy.Table) { x.Links[0].Record = "deal" },
		"link column not an ident":    func(x *privacy.Table) { x.Links[0].Column = "a;drop" },
		"personal column duplicate":   func(x *privacy.Table) { x.Personal = []string{"body", "body"} },
		"personal column is a link":   func(x *privacy.Table) { x.Personal = []string{"activity_id"} },
		"personal column is id":       func(x *privacy.Table) { x.Personal = []string{"id"} },
		"retention class invented":    func(x *privacy.Table) { x.Retention = "forever" },
		"age column not an ident":     func(x *privacy.Table) { x.AgeColumn = "created;drop" },
		"age column is personal":      func(x *privacy.Table) { x.AgeColumn = "body" },
	}
	if err := valid().Validate("ext_demo"); err != nil {
		t.Fatalf("a well-formed declaration was refused: %v", err)
	}
	aged := valid()
	aged.AgeColumn = "created_at"
	if err := aged.Validate("ext_demo"); err != nil {
		t.Fatalf("a declaration with an age column was refused: %v", err)
	}
	for name, mutate := range cases {
		x := valid()
		x.Links = append([]privacy.Link(nil), x.Links...)
		mutate(&x)
		if err := x.Validate("ext_demo"); err == nil {
			t.Errorf("%s: want a refusal, got none", name)
		}
	}
}

func TestDigestIgnoresOrderButNotContent(t *testing.T) {
	a := valid()
	a.Personal = []string{"body", "subject"}
	b := valid()
	b.Personal = []string{"subject", "body"}
	da, _ := privacy.Digest([]privacy.Table{a})
	db, _ := privacy.Digest([]privacy.Table{b})
	if da != db {
		t.Error("reordering columns changed the digest")
	}
	b.Personal = []string{"body"}
	dc, _ := privacy.Digest([]privacy.Table{b})
	if da == dc {
		t.Error("dropping a personal column left the digest unchanged")
	}
}
