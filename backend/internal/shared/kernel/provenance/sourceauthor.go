// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"strings"
	"unicode/utf8"
)

// SourceAuthorNameMax bounds the source system's spelling of an author, in
// runes: a name, not a paragraph.
const SourceAuthorNameMax = 200

// AuthorError refuses the author pair on a create wire, naming the field it
// arrived on. It is caller-fixable, so every surface answers it as a 422.
type AuthorError struct{ Field, Code, Message string }

func (e *AuthorError) Error() string { return e.Message }

// FieldFault states the refusal for the HTTP mapper and the tool surface alike.
func (e *AuthorError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

// AuthorClaim is the author pair as a create wire carries it, beside the
// source_system it is a claim about.
type AuthorClaim struct {
	HasAuthorID  bool
	AuthorName   *string
	SourceSystem *string
}

// AdmitSourceAuthor guards who-wrote-this on a create wire and returns the name
// trimmed for storage, "" when none was sent.
//
// The pair says who wrote the record in the system it came from, so it is the
// importer's to state and nobody else's: the same door as the mirror:
// namespace (RefuseWireAdmitting), decided by auth.DeclaredImporter at the
// handler. Either spelling or both may arrive — the seat's current name wins on
// read, and the source's spelling is what survives a deleted seat — but a claim
// that names nobody is refused, and so is one on a record that names no source.
func AdmitSourceAuthor(claim AuthorClaim, importer bool) (string, error) {
	hasAuthorID, authorName, sourceSystem := claim.HasAuthorID, claim.AuthorName, claim.SourceSystem
	if !hasAuthorID && authorName == nil {
		return "", nil
	}
	field := "source_author_name"
	if hasAuthorID {
		field = "source_author_id"
	}
	if !importer {
		return "", &AuthorError{
			Field: field, Code: "reserved_source_author",
			Message: field + " is written only by a declared importer; omit it",
		}
	}
	var trimmed string
	if authorName != nil {
		trimmed = strings.TrimSpace(*authorName)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > SourceAuthorNameMax {
			return "", &AuthorError{
				Field: "source_author_name", Code: "source_author_name_invalid",
				Message: "source_author_name must be a non-blank name of at most 200 characters",
			}
		}
	}
	if sourceSystem == nil || strings.TrimSpace(*sourceSystem) == "" {
		return "", &AuthorError{
			Field: field, Code: "source_author_needs_a_source",
			Message: field + " needs source_system: an author is a claim about the system the record came from",
		}
	}
	return trimmed, nil
}
