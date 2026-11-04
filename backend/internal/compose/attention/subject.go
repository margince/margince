// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// subjectOf names the record an item concerns, when the producer named one.
// It carries no label: a display name is a read of that record under grants
// this feed does not hold. The client asks the record's own endpoint for it.
func subjectOf(entityType string, id ids.UUID) *crmcontracts.AttentionSubject {
	kind, ok := subjectKinds[entityType]
	if !ok {
		return nil
	}
	return &crmcontracts.AttentionSubject{Type: kind, Id: openapi_types.UUID(id)}
}

// subjectKinds maps a staged target's entity type onto the contract's subject
// enum. A type absent here yields no subject, never a guessed one.
var subjectKinds = map[string]crmcontracts.AttentionSubjectType{
	subjectCompany: subjectCompany,
	"contact":      "contact",
	"deal":         "deal",
	"lead":         "lead",
	"activity":     "activity",
	"project":      "project",
}

// openableSubject reports whether a subject names a record with its own page.
// Only those may offer `open`. An activity is a timeline entry with no page,
// and the client's router answers this same set.
func openableSubject(subject *crmcontracts.AttentionSubject) bool {
	if subject == nil {
		return false
	}
	switch subject.Type {
	case subjectCompany, subjectContact, subjectDeal, "lead", "project":
		return true
	}
	return false
}
