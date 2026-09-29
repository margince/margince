// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// What a record's TRAIL carries beyond the record's own fields. A one-to-one
// extension audits its images onto the host — a partner's terms are recorded as
// a change to the company — so the host's history serves the extension's fields
// under the host's object, and a mask configured on the extension never meets
// them.
//
// It is declared here because a history read resolves masks for record types it
// does not own, and a module may not ask its siblings what their images carry.

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditImageFacets names, per audit entity type, the OBJECTS whose fields that
// entity's audit images also carry. A facet left undeclared is a mask that
// reaches the register and not the trail behind it, which
// TestEveryOfferedMaskObjectIsWithheldFromSomeRecordsHistory fails on: every
// maskable object is a record with a history of its own, or a facet of one.
var auditImageFacets = map[string][]string{
	"company": {"partner"},
}

// MaskedHistoryFields answers the fields of one record's trail the principal
// reads as withheld: what MaskedFields withholds on the record's own object,
// and what it withholds on every facet that record's images carry.
//
// A facet is asked UNLIFTED whatever the host's write authority, because
// lifting is a question about the facet's own row — its owner, its grants — and
// the host's write arm does not answer it.
func MaskedHistoryFields(p principal.Principal, entityType string, writable bool) []string {
	withheld := MaskedFields(p, entityType, writable)
	for _, facet := range auditImageFacets[entityType] {
		for _, field := range MaskedFields(p, facet, false) {
			if !slices.Contains(withheld, field) {
				withheld = append(withheld, field)
			}
		}
	}
	return withheld
}

// HistoryFacets is the declaration above, for the gate holding it against the
// maskable-field catalog — which cannot read an unexported table and must not
// keep a second copy of one.
func HistoryFacets() map[string][]string {
	facets := make(map[string][]string, len(auditImageFacets))
	for entityType, objects := range auditImageFacets {
		facets[entityType] = slices.Clone(objects)
	}
	return facets
}
