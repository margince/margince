// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Writing the relationship mark: who may set it, and what an edit records.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

// fieldRelationshipMark is the wire field and the column of the mark.
const fieldRelationshipMark = "relationship_mark"

// columnRelationshipMarkSetBy records who set the mark. It rides the same UPDATE
// as the mark, because the table requires both or neither. It stays out of the
// audit image. An undo restores the mark, and whoever undoes it becomes its setter.
const columnRelationshipMarkSetBy = "relationship_mark_set_by"

// relationshipMarkSetter is the principal that stamps a mark, or "" when the edit
// neither sets nor clears one.
func relationshipMarkSetter(ctx context.Context, in UpdateContactInput) (string, error) {
	if in.RelationshipMark == nil {
		return "", nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return "", fmt.Errorf("contacts: who sets the relationship mark: %w", err)
	}
	return by, nil
}

// patchRelationshipMark stages the mark and its setter beside the other columns.
// A clear (the field named in in.Clear) is staged by ApplyClears; this adds the
// setter column that must go to NULL in the same statement.
func patchRelationshipMark(p *storekit.Patch, current crmcontracts.Contact, in UpdateContactInput, setter string) error {
	if slices.Contains(in.Clear, fieldRelationshipMark) {
		p.SetStamp(columnRelationshipMarkSetBy, nil)
		return nil
	}
	if in.RelationshipMark != nil {
		mark := *in.RelationshipMark
		if !relationshipmark.Valid(mark) {
			return httperr.Validation(fieldRelationshipMark, "invalid", "unknown relationship mark")
		}
		p.Set(fieldRelationshipMark, current.RelationshipMark, string(mark))
		p.SetStamp(columnRelationshipMarkSetBy, setter)
	}
	return nil
}

// markSetBy is the setter recorded beside a mark written at birth: the principal
// that made the contact, or nothing when the contact is not marked.
func markSetBy(mark relationshipmark.Mark, capturedBy string) string {
	if mark == "" {
		return ""
	}
	return capturedBy
}

// relationshipMarkAny is the list filter value that returns marked and unmarked
// contacts alike.
const relationshipMarkAny = "any"

// relationshipMarkClause narrows a contact list by the mark, or returns "" when
// it must not.
//
// With no filter the list leaves marked contacts out. A text search is the
// exception, because someone typing a name wants that contact whatever it is.
// An unknown value matches nothing, so a caller's typo reads as an empty page.
func relationshipMarkClause(in ListContactsInput, arg func(any) int) string {
	switch {
	case in.RelationshipMark == nil:
		if in.Query != nil && strings.TrimSpace(*in.Query) != "" {
			return ""
		}
		return relationshipmark.NotMarkedSQL(contactEntity)
	case *in.RelationshipMark == relationshipMarkAny:
		return ""
	default:
		return storekit.SQLf("contact.relationship_mark = $%d", arg(*in.RelationshipMark))
	}
}
