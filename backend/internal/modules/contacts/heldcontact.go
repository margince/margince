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
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ContactHeldByColleagueError refuses an address a contact the caller cannot
// read already holds.
type ContactHeldByColleagueError struct {
	Email string
	// ShareRequested reports that the holder has an open request to share the
	// contact with the caller. False after the holder declined one, and when
	// nobody can be asked.
	ShareRequested bool
	// The held contact and its live owner, for the request alone. They never
	// reach the caller.
	contactID ids.ContactID
	holderID  ids.UUID
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
	held := &ContactHeldByColleagueError{Email: email, contactID: existing}
	// Only a live owner can decide a request. A contact with none still
	// refuses, and nobody is asked.
	var owner *ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT p.owner_id FROM contact p
		  JOIN app_user u ON u.id = p.owner_id AND `+workforce.LiveMemberSQL("u")+`
		 WHERE p.id = $1`, existing).Scan(&owner); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("contacts: reading who holds a claimed address: %w", err)
	}
	if owner != nil {
		held.holderID = *owner
	}
	return held
}

// ShareRequest asks the holder of a contact to share it with the requester.
type ShareRequest struct {
	ContactID   ids.ContactID
	HolderID    ids.UUID
	RequesterID ids.UUID
}

// ShareRequestFiler files req on the caller's transaction and reports whether
// a request is open. It answers false when the holder declined one before, and
// files nothing then. Compose binds it to the approvals queue.
type ShareRequestFiler func(ctx context.Context, tx pgx.Tx, req ShareRequest) (bool, error)

// WithShareRequests wires the filer. Unwired, a held address still refuses
// and nobody is asked.
func (s *Store) WithShareRequests(file ShareRequestFiler) *Store {
	s.shareRequests = file
	return s
}

// WithShareRequests mirrors the store option for the HTTP surface.
func (h Handlers) WithShareRequests(file ShareRequestFiler) Handlers {
	h.store = h.store.WithShareRequests(file)
	return h
}

// WithShareRequests mirrors the store option for the agent surface.
func (p *Provider) WithShareRequests(file ShareRequestFiler) *Provider {
	p.store = p.store.WithShareRequests(file)
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
	if !ok || s.shareRequests == nil || held.holderID.IsZero() {
		return err
	}
	requester, ok := shareRequesterOf(ctx)
	if !ok || requester == held.holderID {
		return err
	}
	req := ShareRequest{ContactID: held.contactID, HolderID: held.holderID, RequesterID: requester}
	if fileErr := s.db.Tx(ctx, func(tx pgx.Tx) error {
		open, fileErr := s.shareRequests(ctx, tx, req)
		held.ShareRequested = open
		return fileErr
	}); fileErr != nil {
		return fmt.Errorf("contacts: asking a colleague to share a contact: %w", fileErr)
	}
	return err
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
