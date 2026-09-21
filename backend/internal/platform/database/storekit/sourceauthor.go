// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The parts of "record who wrote this where it came from" that are the same
// whichever table it is written to.
//
// WHY HERE. The repair reaches five record tables across three modules, and a
// module never imports a sibling (gates/arch_test.go TestNoSiblingModuleImports)
// — so `contacts`, `deals` and `projects` cannot share a helper by one of them
// owning it. What they CAN share is this package, which all three already
// import for LockRow, Patch and Audit.
//
// What lives here is only the part that is genuinely table-independent: the
// input, the three outcomes, the words a skip answers in, and the two questions
// asked of the row's own columns. The LOCK, the PATCH and the AUDIT stay in
// each module, because each module owns what its own table's write means —
// and an activity's version of this needs a savepoint and an audience check
// that no record table has.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SourceAuthorInput is one record's answer: who wrote it, in whichever of the
// two spellings the source could give.
type SourceAuthorInput struct {
	// AuthorID is the member who wrote it, when the author holds a seat here.
	AuthorID *ids.UUID
	// AuthorName is the source system's own spelling, for an author who never
	// held one.
	AuthorName *string
}

// Named reports whether the caller gave an answer at all.
//
// Neither spelling is a refusal rather than "clear it": a caller that sends
// neither has lost its answer somewhere upstream, and silently emptying an
// attribution it previously wrote is the one outcome nobody could have meant.
func (in SourceAuthorInput) Named() bool {
	return in.AuthorID != nil || (in.AuthorName != nil && *in.AuthorName != "")
}

// SourceAuthorOutcome is what happened to one record, in the words the wire
// reports them.
type SourceAuthorOutcome string

// The three answers an attribution write gives.
//
// `unchanged` is decided under the row's own lock rather than by the caller
// from a digest beside it. Deciding it outside the lock was a defect twice over
// in the activity path: a concurrent write lands between the reading and the
// skipping and the two records disagree forever, and a caller outside the row's
// audience learns whether its author matches a guess without ever being allowed
// to read it.
const (
	SourceAuthorApplied   SourceAuthorOutcome = "applied"
	SourceAuthorSkipped   SourceAuthorOutcome = "skipped"
	SourceAuthorUnchanged SourceAuthorOutcome = "unchanged"
)

// SourceAuthorUnnamedReason is the skip for a row carrying no author at all.
const SourceAuthorUnnamedReason = "no author given: send source_author_id, source_author_name, or both"

// SourceAuthorMissingReason is the skip for a row this installation does not
// hold, or has archived.
//
// ONE sentence for both, deliberately. An erasure archives what it destroys, so
// a record that is merely gone and a record that was erased must read the same
// — otherwise a caller offering a guessed author could tell them apart.
func SourceAuthorMissingReason(object string) string {
	return fmt.Sprintf("no such %s here, or it has been archived", object)
}

// SourceAuthorBefore is the answer a row already carries, read under its lock.
type SourceAuthorBefore struct {
	ID   *ids.UUID
	Name *string
}

// Same reports whether the row already carries exactly this answer.
//
// Both halves must match. An offer naming only a name, against a row carrying a
// name AND a seat id, is a different answer — it drops the seat — so it is a
// write rather than a no-op.
func (b SourceAuthorBefore) Same(in SourceAuthorInput) bool {
	switch {
	case (b.ID == nil) != (in.AuthorID == nil):
		return false
	case b.ID != nil && *b.ID != *in.AuthorID:
		return false
	case (b.Name == nil) != (in.AuthorName == nil):
		return false
	case b.Name != nil && *b.Name != *in.AuthorName:
		return false
	}
	return true
}

// AttributableNow reads the row's current answer and decides whether this one
// may replace it: the before-image for the audit, and a skip reason when it may
// not.
//
// `object` names the table, and it is a package-internal constant at every call
// site rather than anything a request carries — the caller's object_type is
// mapped to one of six known stores before this is reached.
func AttributableNow(
	ctx context.Context, tx pgx.Tx, object string, id ids.UUID, in SourceAuthorInput,
) (before SourceAuthorBefore, reason string, err error) {
	var sourceSystem *string
	// `object` reaches here only from a package constant at each call site —
	// the caller's object_type is mapped to one of six known stores first — so
	// it is never request data reaching SQL.
	q := fmt.Sprintf(
		`SELECT source_system, source_author_id, source_author_name FROM %s WHERE id = $1`, object)
	if err := tx.QueryRow(ctx, q, id).Scan(&sourceSystem, &before.ID, &before.Name); err != nil {
		return before, "", fmt.Errorf("storekit: reading the %s before attributing it: %w", object, err)
	}
	if sourceSystem == nil || *sourceSystem == "" {
		return before, fmt.Sprintf(
			"the %s names no source system, so it was created here rather than imported", object), nil
	}
	if in.AuthorID == nil {
		return before, "", nil
	}
	// No liveness filter: a departed colleague is exactly who this repair most
	// often names, and a deactivated seat is still a seat. Only a row that does
	// not exist at all is a refusal.
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM app_user WHERE id = $1)`, *in.AuthorID).Scan(&exists); err != nil {
		return before, "", fmt.Errorf("storekit: resolving the named author: %w", err)
	}
	if !exists {
		return before, "the named author holds no seat in this installation", nil
	}
	return before, "", nil
}
