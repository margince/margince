// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A create whose address belongs to a contact the caller cannot read.
//
// The refusal stays: two records of one contact are the defect the address key
// exists to prevent. The private contact is capture's holding state, not a
// confidentiality boundary, because inside one workspace everyone reads every
// record except mail. So the refusal says that a colleague holds the contact,
// and the holder is asked to share the contact with the caller.
//
// That is why this answers 409 and not the 404 a row-scope miss gives. The 409
// tells the caller that the address exists in the workspace, which the
// duplicate_email answer already told them. It names no record, no owner and
// no field of the contact.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContactHeldByColleagueError refuses an address a contact the caller cannot
// read already holds.
type ContactHeldByColleagueError struct {
	Email string
	// ShareRequested reports that the holder has an open request to share the
	// contact with the caller. False after the holder declined one, and when
	// nobody can be asked.
	ShareRequested bool
	// The held contact, for the request alone. It never reaches the caller.
	contactID ids.ContactID
}

func (e *ContactHeldByColleagueError) Error() string {
	if e.ShareRequested {
		return "a colleague already has this contact; they were asked to share it"
	}
	return "a colleague already has this contact"
}

// Is keeps the refusal a conflict for every caller that asks only that.
func (e *ContactHeldByColleagueError) Is(target error) bool { return target == apperrors.ErrConflict }

// IsClaimedEmail reports whether err refuses an address another contact holds,
// whether or not the caller can read that contact.
func IsClaimedEmail(err error) bool {
	if _, ok := errors.AsType[*DuplicateEmailError](err); ok {
		return true
	}
	_, ok := errors.AsType[*ContactHeldByColleagueError](err)
	return ok
}

// claimedEmailRefusal answers an address that contact `existing` holds. A
// caller who can read it gets its id; one who cannot gets the held refusal.
func claimedEmailRefusal(ctx context.Context, tx pgx.Tx, email string, existing ids.ContactID) error {
	visible, err := auth.VisibleTo(ctx, tx, entityContact, existing.UUID)
	if err != nil {
		return err
	}
	if visible {
		return &DuplicateEmailError{Email: email, ExistingID: existing}
	}
	return &ContactHeldByColleagueError{Email: email, contactID: existing}
}

// ShareRequest asks the holder of a contact to share it with the requester.
type ShareRequest struct {
	ContactID   ids.ContactID
	HolderID    ids.UUID
	RequesterID ids.UUID
}

// ShareRequests is the approvals queue's side of a share request. Compose
// binds it.
type ShareRequests interface {
	// FileTx files req on the caller's transaction and reports whether a
	// request is open. It answers false when the holder declined one before,
	// and files nothing then.
	FileTx(ctx context.Context, tx pgx.Tx, req ShareRequest) (bool, error)
	// FollowOwnerTx withdraws the pending requests on a contact whose owner
	// is no longer the one they name. to is nil for a contact nobody owns.
	FollowOwnerTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, to *ids.UUID) error
}

// WithShareRequests wires the queue. Unwired, a held address still refuses
// and nobody is asked.
func (s *Store) WithShareRequests(requests ShareRequests) *Store {
	s.shareRequests = requests
	return s
}

// followShareOwnerTx keeps the contact's pending share requests with its
// current owner. Every writer that changes a contact's owner calls it on its
// own transaction.
func (s *Store) followShareOwnerTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, from, to *ids.UUID) error {
	if s.shareRequests == nil || sameOwner(from, to) {
		return nil
	}
	return s.shareRequests.FollowOwnerTx(ctx, tx, id, to)
}

func sameOwner(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// WithShareRequests mirrors the store option for the HTTP surface.
func (h Handlers) WithShareRequests(requests ShareRequests) Handlers {
	h.store = h.store.WithShareRequests(requests)
	return h
}

// WithShareRequests mirrors the store option for the agent surface.
func (p *Provider) WithShareRequests(requests ShareRequests) *Provider {
	p.store = p.store.WithShareRequests(requests)
	return p
}

// AskHolderToShare is the one follow-up to a held refusal. It files the share
// request in a transaction of its own and returns err with the outcome on it.
// Any other error passes through.
//
// It runs after the refused transaction rolled back. The refusal aborts the
// write it answers, and on the race path a unique violation has already
// poisoned that transaction. Store-owned writes reach it through tx. A door
// that owns its transaction calls it after that transaction ends.
func (s *Store) AskHolderToShare(ctx context.Context, err error) error {
	held, ok := errors.AsType[*ContactHeldByColleagueError](err)
	if !ok || s.shareRequests == nil {
		return err
	}
	requester, ok := shareRequesterOf(ctx)
	if !ok {
		return err
	}
	if fileErr := s.db.Tx(ctx, func(tx pgx.Tx) error {
		holder, owned, err := lockHolder(ctx, tx, held.contactID)
		if err != nil || !owned || holder == requester {
			return err
		}
		open, err := s.shareRequests.FileTx(ctx, tx, ShareRequest{
			ContactID: held.contactID, HolderID: holder, RequesterID: requester,
		})
		held.ShareRequested = open
		return err
	}); fileErr != nil {
		return fmt.Errorf("contacts: asking a colleague to share a contact: %w", fileErr)
	}
	return err
}

// lockHolder reads the contact's owner on the request's own transaction and
// holds the contact row. An owner change either lands first and is the owner
// read here, or waits and then withdraws the request it finds
// (ShareRequests.FollowOwnerTx). A contact with no owner refuses and nobody is
// asked; an owner who left is refused by the approvals queue, which asks only
// a member who may still decide.
func lockHolder(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (ids.UUID, bool, error) {
	var owner *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT owner_id FROM contact WHERE id = $1 AND archived_at IS NULL FOR UPDATE`,
		contactID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner == nil) {
		return ids.Nil, false, nil
	}
	if err != nil {
		return ids.Nil, false, fmt.Errorf("contacts: reading who holds a claimed address: %w", err)
	}
	return *owner, true, nil
}

// shareRequesterOf is the human a request is filed for: the seat itself, or
// the colleague an agent works for. Automatic work with nobody behind it asks
// nobody.
func shareRequesterOf(ctx context.Context) (ids.UUID, bool) {
	actor, ok := principal.Actor(ctx)
	switch {
	case !ok:
		return ids.Nil, false
	case actor.Type == principal.PrincipalHuman && !actor.UserID.IsZero():
		return actor.UserID, true
	case actor.ActsAsItsUser():
		return actor.OnBehalfOf, true
	}
	return ids.Nil, false
}
