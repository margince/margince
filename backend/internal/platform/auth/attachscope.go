// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The ATTACH direction of the row question: hanging a child row (an activity,
// a note, a task, a file, a record link) onto a record somebody can read.
//
// The attached row is something every reader of that record will now see, so
// it is a write to the record and needs write authority over it: the caller
// owns it, shares a team with its owner, or holds a live `write` grant. Seeing
// the record is not enough, whether the caller sees it through workspace
// readability or through a `read` share. Naming a visible record as a value
// in a row of the caller's own (a company on a new deal) stays a REFERENCE and
// asks EnsureLinkTarget alone. The test a call site applies is: whose record
// changes?
//
// The system principal holds write authority over every row, so automatic
// filing that runs as the system is unaffected.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// shareLevel selects which record_grant rows the visibility predicate's share
// arm admits. Reading a shared record takes any live grant. Attaching takes a
// `write` grant, and this matters where write authority alone would not
// refuse: a row_scope=all caller holds write authority over every row, yet
// sees a capture-private contact only through its share, and a share the
// sharing screen called read-only must not let them add to it.
type shareLevel bool

const (
	anyShare   shareLevel = false
	writeShare shareLevel = true
)

// EnsureAttachTarget is EnsureLinkTarget for the attach direction: the record
// must exist, be live, be visible, and be the caller's to change.
//
// An invisible record answers not found. A visible one the caller may not
// change answers AttachAuthorityError, a 403, because the caller already
// knows the record exists.
func EnsureAttachTarget(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	// Visibility first, and it also takes the FOR SHARE lock every caller of
	// this probe needs before it writes.
	if err := EnsureLinkTarget(ctx, tx, table, id); err != nil {
		return err
	}
	// An extension kind has no share to narrow; its update right is the whole
	// of its write authority, which ensureWriteAuthority asks.
	if !recordkinds.IsExtensionName(table) {
		shared, err := throughWriteShare(ctx, tx, table, id)
		if err != nil {
			return err
		}
		if !shared {
			return attachRefusal(ctx, table)
		}
	}
	err := ensureWriteAuthority(ctx, tx, table, id)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return attachRefusal(ctx, table)
	}
	return err
}

// throughWriteShare answers whether the row stays visible once a read-only
// share no longer counts.
func throughWriteShare(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)
	clause, err := writeShareClauseFor(ctx, table, "", arg)
	if err != nil || clause == "" {
		return true, err
	}
	var visible bool
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE id = $%d AND %s)`, table, idPos, clause),
		args...).Scan(&visible)
	return visible, err
}

// attachRefusal notes the refusal as a row gate's, as refuseRowChange does.
func attachRefusal(ctx context.Context, table string) error {
	return RefuseRow(ctx, table, &AttachAuthorityError{Kind: table})
}

// AttachClauseFor is EnsureAttachTarget as a clause, for a list that marks
// which rows the caller may attach to: the visibility clause and the write
// authority clause together. Empty means no narrowing, read exactly as
// ScopeClauseFor's empty is.
func AttachClauseFor(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	visible, err := writeShareClauseFor(ctx, table, alias, arg)
	if err != nil {
		return "", err
	}
	writable, err := WriteAuthorityClauseFor(ctx, table, alias, arg)
	if err != nil {
		return "", err
	}
	switch {
	case visible == "":
		return writable, nil
	case writable == "":
		return visible, nil
	}
	return "(" + visible + " AND " + writable + ")", nil
}

// AttachableSubset is EnsureAttachTarget over many live rows, in one
// statement. Absent means refused, for whatever reason.
func AttachableSubset(ctx context.Context, tx pgx.Tx, table string, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	terms := []string{fmt.Sprintf("id = ANY($%d)", arg(rowIDs)), "archived_at IS NULL"}
	clause, err := AttachClauseFor(ctx, table, "", arg)
	if err != nil {
		return nil, err
	}
	if clause != "" {
		terms = append(terms, clause)
	}
	return idsWhere(ctx, tx, table, "id", terms, args)
}

// AttachAuthorityError refuses an attach to a record the caller can read but
// not change, and says who can fix it, on every transport.
type AttachAuthorityError struct{ Kind string }

func (e *AttachAuthorityError) Error() string {
	code, message := e.MessageFault()
	return code + ": " + message
}

// Unwrap keeps the refusal on the sentinel table's 403 row.
func (e *AttachAuthorityError) Unwrap() error { return apperrors.ErrPermissionDenied }

// MessageFault names the kind of record and the remedy.
func (e *AttachAuthorityError) MessageFault() (code, message string) {
	return "permission_denied", fmt.Sprintf(
		"You can see this %s but cannot add to it. Only its owner, their team, or someone it is shared with "+
			"for editing can. Ask the owner to share it with you with edit access.", e.Kind)
}
