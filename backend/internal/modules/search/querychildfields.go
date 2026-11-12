// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Fields a record keeps in a child table rather than a column of its own.
//
// The storage filter publishes a field only when the record's table holds a
// column for it. A contact's LinkedIn link has none: the contract names it on
// the contact, and the value lives in contact_social. This declaration
// publishes it and compiles it to an EXISTS over the child rows. The record's
// own row scope still decides which records come back.

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/socialplatform"
)

// childField is one field held in a child table. match renders the test that
// a record's child rows hold one of the bound operands.
type childField struct {
	kind  FieldKind
	match func(alias string, operands []string) string
}

// childFields are the child-held fields, by record type and wire name.
var childFields = map[string]map[string]childField{
	entityContact: {"linkedin_url": {kind: KindText, match: contactLinkedInMatch}},
}

// contactLinkedInMatch finds a contact whose LinkedIn link is the operand as
// stored, or names the same profile: https://linkedin.com/in/x/ finds
// https://www.linkedin.com/in/x. The profile key is the one the duplicate
// detection compares, and the index on contact_social holds it.
func contactLinkedInMatch(alias string, operands []string) string {
	keys := make([]string, len(operands))
	for i, operand := range operands {
		keys[i] = storekit.LinkedInProfileKeySQL(operand)
	}
	return "EXISTS (SELECT 1 FROM contact_social social WHERE social.contact_id = " + alias + ".id" +
		" AND social.platform = '" + socialplatform.LinkedIn + "'" +
		" AND (social.handle IN (" + strings.Join(operands, ", ") + ")" +
		" OR " + storekit.LinkedInProfileKeySQL("social.handle") + " IN (" + strings.Join(keys, ", ") + ")))"
}

// withChildren is this storage with the child fields of one record type.
func (s *storage) withChildren(entity string) *storage {
	children := childFields[entity]
	if len(children) == 0 {
		return s
	}
	widened := *s
	widened.children = children
	return &widened
}

// child answers the child-held field of this name, if the record has one.
func (s *storage) child(name string) (childField, bool) {
	field, ok := s.children[name]
	return field, ok
}
