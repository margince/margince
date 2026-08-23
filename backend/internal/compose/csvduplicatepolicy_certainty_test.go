// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What `on_duplicate: update` will and will not overwrite, stated as pairs.
//
// collision.writable() requires the incumbent to share the WHOLE name, legal
// suffix included. That is stricter than the dedupe ladder, which strips the
// suffix before scoring — and the gap between the two is the point of this test.
//
// It matters because the two acts differ in what they cost when wrong. The
// ladder PROPOSES: a bad proposal wastes a human's glance. An update OVERWRITES,
// and an import cannot be reversed onto a company it did not create, so a bad
// overwrite loses the incumbent's address, size and industry with no call that
// puts them back.

import (
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/people"
)

func TestUpdateOverwritesOnlyARecordSharingTheWholeName(t *testing.T) {
	// Folds that never change which company is meant.
	same := [][2]string{
		{"Kestrel Data GmbH", "kestrel data gmbh"}, // casefold
		{"Kestrel Data", "Kestrel Dätä"},           // unaccent
		{"Straße Co", "STRASSE Co"},                // ß folds to ss — a DACH pair daily
		{"Acme, Inc", "Acme Inc"},                  // a typist's comma
		{"Acme GmbH.", "Acme GmbH"},                // a trailing period
		{"Acme   GmbH", "Acme GmbH"},               // collapsed whitespace
	}
	for _, pair := range same {
		if !people.SameOrganizationName(pair[0], pair[1]) {
			t.Errorf("%q and %q are the same name under folds that never change which company is "+
				"meant, and an update run refuses them — that refuses the case the mode exists for",
				pair[0], pair[1])
		}
	}

	different := [][2]string{
		// Merely similar. 0.89 is a high score and still the wrong company.
		{"Kestrel Data Systems", "Kestrel Data Solutions"},
		{"Bauer GmbH", "Bauer Metallbau GmbH"},
		{"Nordwind Logistik", "Nordwind Energie"},
		// DIFFERENT LEGAL ENTITIES sharing a stem. These score 1.0 on the dedupe
		// ladder, which strips the suffix before scoring — and fuzzyOrganization
		// says in as many words that they are a human's call rather than a
		// merge. An update run taking that score would perform the merge the
		// ladder deliberately refused to, silently.
		{"Acme Inc", "Acme GmbH"},
		{"Acme Inc", "Acme AG"},
		{"Bauer GmbH", "Bauer KG"},
		// One name is the other plus a suffix: still two entities to a human.
		{"Acme", "Acme Inc"},
	}
	for _, pair := range different {
		if people.SameOrganizationName(pair[0], pair[1]) {
			t.Errorf("%q and %q are treated as one name, and an update run would overwrite one "+
				"company's record with another's — permanently, since an import cannot be reversed "+
				"onto a company it did not create", pair[0], pair[1])
		}
	}

	// The 256-rune cap inside nameSimilarity makes two long names sharing a
	// prefix SCORE 1.0. That is a capped score, not an identity, and an overwrite
	// rule must not read it as one.
	long := strings.Repeat("a", 300)
	if people.SameOrganizationName(long+"Alpha", long+"Beta") {
		t.Error("two names differing only past the similarity cap are treated as one name; the cap " +
			"bounds how similar things are SAID to be and cannot stand in for identity")
	}
}
