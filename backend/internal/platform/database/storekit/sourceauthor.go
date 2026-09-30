// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Who wrote a record in the system it was imported from. Six record tables
// across four modules carry the pair, and a module never imports a sibling, so
// the part every create shares lives here.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// SourceAuthorInput is who wrote a record where it came from, in whichever
// spellings the source could give. The zero value is "no author".
type SourceAuthorInput struct {
	// AuthorID is the member who wrote it, when the author holds a seat here.
	AuthorID *ids.UUID
	// AuthorName is the source system's own spelling of the author.
	AuthorName *string
}

// AdmitSourceAuthor is the create mappers' one call: the wire's author pair,
// refused or admitted by provenance.AdmitSourceAuthor, as a store input.
// Generic over the id so the contract's UUID type never reaches this package.
func AdmitSourceAuthor[U ~[16]byte](id *U, name, sourceSystem *string, importer bool) (SourceAuthorInput, error) {
	trimmed, err := provenance.AdmitSourceAuthor(provenance.AuthorClaim{
		HasAuthorID: id != nil, AuthorName: name, SourceSystem: sourceSystem,
	}, importer)
	if err != nil {
		return SourceAuthorInput{}, err
	}
	var in SourceAuthorInput
	if trimmed != "" {
		in.AuthorName = &trimmed
	}
	if id != nil {
		seat := ids.UUID(*id)
		in.AuthorID = &seat
	}
	return in, nil
}

// AuthorInsertFragments appends the author pair to an INSERT's fixed args and
// returns the column and placeholder fragments, numbered from the args so no
// placeholder is counted by hand. Chain it before InsertFragments.
func AuthorInsertFragments(in SourceAuthorInput, base []any) (cols, placeholders string, args []any) {
	n := len(base)
	args = make([]any, 0, n+2)
	args = append(append(args, base...), in.AuthorID, in.AuthorName)
	return ", source_author_id, source_author_name", fmt.Sprintf(", $%d, $%d", n+1, n+2), args
}

// RefuseUnknownSeat answers a named seat that does not exist as the caller's
// mistake (422) rather than a foreign-key failure (500).
//
// No liveness filter: a departed colleague is who an import most often names,
// and a deactivated seat is still a seat.
func RefuseUnknownSeat(ctx context.Context, tx pgx.Tx, in SourceAuthorInput) error {
	if in.AuthorID == nil {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1)`, *in.AuthorID).Scan(&exists); err != nil {
		return fmt.Errorf("storekit: resolving the named author: %w", err)
	}
	if !exists {
		return &provenance.AuthorError{
			Field: "source_author_id", Code: "unknown_seat",
			Message: "source_author_id names no member of this installation",
		}
	}
	return nil
}
