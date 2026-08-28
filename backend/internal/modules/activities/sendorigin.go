// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A send starts either as a reply or as a new conversation, and ADR-0087
// makes that choice explicit rather than inferring it from a missing anchor.
// An accidentally absent anchor would otherwise become a new conversation
// silently, losing a reply's threading with no signal anyone could see.
//
// Both origins resolve to the SAME two facts — the threading chain the
// message carries and the record links its timeline row inherits — and
// everything after that resolution is one code path, which is what keeps
// the authorization order, the consent gate, deliverability derivation,
// identity minting and the single-transaction staging from forking.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SendOrigin is where one outbound message starts. Exactly one of the two
// constructors below produces a usable value; the zero value refuses, so a
// caller that forgets to name an origin cannot accidentally send anything.
type SendOrigin struct {
	// anchor is the activity being replied to. Zero on an account-started
	// send, which is the only way the two are told apart.
	anchor ids.ActivityID
	// links are the record links an account-started message is filed under,
	// supplied explicitly because there is no anchor to inherit them from.
	// Each is row-scope probed at insert by insertActivityLinks.
	links []ActivityLinkInput
	// also are records a REPLY is filed under beyond what its anchor carries.
	//
	// Added rather than substituted, and that is the whole shape of it: a reply
	// belongs to the same people and the same deal as the conversation it
	// continues, so a caller that could replace the inherited set could detach
	// a thread from the records it is about. What it is for is the link the
	// anchor cannot have — a deal whose project was attached after the
	// conversation started leaves every reply in that thread unfiled.
	//
	// Ignored on an account origin, which names its whole set in links.
	also []ActivityLinkInput
}

// FromActivity is the reply origin: the anchor is read, its threading chain
// is continued, and the new activity inherits its record links.
func FromActivity(anchor ids.ActivityID) SendOrigin {
	return SendOrigin{anchor: anchor}
}

// AlsoFiledUnder adds records to a reply beyond the ones its anchor carries.
//
// A method on the origin rather than a parameter of FromActivity, because it is
// the uncommon case and every existing caller means "the anchor's own links".
// It returns a new value: an origin is a description of one send, and a
// constructor that could be mutated after the fact is one a later reader has to
// trace to know what was actually filed.
func (o SendOrigin) AlsoFiledUnder(links []ActivityLinkInput) SendOrigin {
	o.also = append([]ActivityLinkInput(nil), links...)
	return o
}

// FromAccount is the account-started origin: no anchor, a fresh thread
// rooted at this message's own newly minted identity, and record links
// named by the caller.
func FromAccount(links []ActivityLinkInput) SendOrigin {
	return SendOrigin{links: links}
}

// isReply reports whether this origin continues an existing conversation.
func (o SendOrigin) isReply() bool { return o.anchor.UUID != ids.UUID{} }

// NoSendOriginError refuses a send whose origin was never named. It is a
// composition defect on any first-party transport — both handlers construct
// an origin — so it carries no FieldFault: there is no request field a
// caller could correct.
type NoSendOriginError struct{}

func (e *NoSendOriginError) Error() string {
	return "send has no origin: name the activity it replies to, or the records it starts from"
}

// resolve reads whatever the origin needs BEFORE the guard sequence runs, so
// a record the caller cannot see answers with the row-scope verdict and
// nothing else.
//
// BOTH origins probe here, and the account one has to. Deferring its links to
// the insert would let a caller name a company they cannot see and still reach
// the consent gate, which answers about the RECIPIENTS — so the refusal they
// got back would disclose whether strangers had consented, and it would be a
// 409 where the row-scope answer is 404. The probe at insert stays: it runs
// inside the staging transaction and still catches a target archived between
// the two reads.
func (o SendOrigin) resolve(ctx context.Context, s *Store) ([]ActivityLinkInput, error) {
	if !o.isReply() {
		if len(o.links) == 0 {
			return nil, &NoSendOriginError{}
		}
		if err := s.probeLinkTargets(ctx, o.links); err != nil {
			return nil, err
		}
		return o.links, nil
	}
	anchor, err := s.GetActivity(ctx, o.anchor, storekit.LiveOnly)
	if err != nil {
		return nil, err
	}
	inherited := inheritedLinks(anchor)
	if len(o.also) == 0 {
		return inherited, nil
	}
	// Probed HERE, before the consent gate, for the reason the account origin's
	// links are: deferring to the insert would let a caller name a record they
	// cannot see and still reach a gate that answers about the RECIPIENTS, so
	// the refusal they got back would disclose whether strangers had consented
	// — and it would be a 409 where the row-scope answer is 404.
	if err := s.probeLinkTargets(ctx, o.also); err != nil {
		return nil, err
	}
	return mergedLinks(inherited, o.also), nil
}

// mergedLinks adds the caller's records to the anchor's, once each.
//
// The inherited links come FIRST and keep their order, so the reply's timeline
// row reads as the conversation's own filing with an addition — not as a set
// the caller composed. A duplicate is collapsed rather than inserted twice:
// naming a record the anchor already carries is a caller being explicit about
// what they expect, which is not an error and not a second link.
func mergedLinks(inherited, also []ActivityLinkInput) []ActivityLinkInput {
	seen := make(map[ActivityLinkInput]bool, len(inherited)+len(also))
	out := make([]ActivityLinkInput, 0, len(inherited)+len(also))
	for _, link := range append(append([]ActivityLinkInput(nil), inherited...), also...) {
		if seen[link] {
			continue
		}
		seen[link] = true
		out = append(out, link)
	}
	return out
}

// lockAnchorLive re-reads the anchor under a row lock inside the writing
// transaction, and refuses if it is no longer live.
//
// resolve() already rejected an archived anchor, but it read through the pool
// and held nothing: between that read and this write the anchor can be
// archived, and threading() does not filter archived rows — so the reply would
// join a thread whose root no longer exists. The account origin has always been
// safe here because its links are re-probed inside the staging transaction
// ("the probe at insert stays", resolve above); the reply origin had no
// equivalent, and this is it.
//
// The window is a request's width for an immediate send and can be days for a
// released draft or a scheduled send, which is why the check belongs on the
// shared write path rather than on whichever caller happened to notice it.
//
// ErrNotFound, not a distinct sentinel: an archived anchor is indistinguishable
// from one that was never visible to this caller, and saying which would leak
// the existence of a record they may no longer read.
func (o SendOrigin) lockAnchorLive(ctx context.Context, tx pgx.Tx) error {
	if !o.isReply() {
		return nil
	}
	// FOR SHARE, not FOR UPDATE. All this needs is that the anchor cannot be
	// archived — an UPDATE — while the reply is being written, and a share lock
	// refuses exactly that. FOR UPDATE would additionally serialize two people
	// replying to the same thread and block ordinary edits to it (a relink, a
	// subject fix) for the length of the write, which is a cost this check has
	// no reason to impose.
	var id ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM activity WHERE id = $1 AND archived_at IS NULL FOR SHARE`,
		o.anchor.UUID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("the message this replies to is no longer available: %w", apperrors.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("lock the anchor of a reply: %w", err)
	}
	return nil
}

// probeLinkTargets refuses an account-started send whose named records the
// caller cannot read, before any later guard can answer about anyone else.
func (s *Store) probeLinkTargets(ctx context.Context, links []ActivityLinkInput) error {
	if len(links) > maxActivityLinks {
		return &TooManyLinksError{Count: len(links)}
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, link := range links {
			if linkColumn(link.EntityType) == "" {
				return &InvalidLinkTypeError{EntityType: link.EntityType}
			}
			if err := auth.EnsureLinkTarget(ctx, tx, link.EntityType, link.EntityID); err != nil {
				return err
			}
		}
		return nil
	})
}

// threading derives the conversation chain this send carries, inside the
// staging transaction. An account-started message answers nothing, so it
// roots the thread at its own identity — the same key capture derives when
// it later reads that root message back out of the mailbox.
func (o SendOrigin) threading(ctx context.Context, tx pgx.Tx, messageID string) (threading, error) {
	if !o.isReply() {
		return threading{threadKey: messageID}, nil
	}
	return anchorThreading(ctx, tx, o.anchor, messageID)
}

// inheritedLinks carries the anchor's own links onto the reply, so the sent
// message lands on the same records' timelines as the conversation it
// answers. The links were already visibility-checked as part of reading the
// anchor, and each one is re-checked at insert.
func inheritedLinks(anchor crmcontracts.Activity) []ActivityLinkInput {
	if anchor.Links == nil {
		return nil
	}
	links := make([]ActivityLinkInput, 0, len(*anchor.Links))
	for _, l := range *anchor.Links {
		links = append(links, ActivityLinkInput{EntityType: string(l.EntityType), EntityID: ids.UUID(l.EntityId)})
	}
	return links
}
