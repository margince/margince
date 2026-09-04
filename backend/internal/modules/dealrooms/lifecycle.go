// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The states the room moves through. Spelled once here so a transition rule and
// the SQL it writes cannot disagree about how a state is named.
const (
	stateDraft    = "draft"
	stateLive     = "live"
	statePaused   = "paused"
	stateClosed   = "closed"
	stateArchived = "archived"
)

// PauseRoom refuses buyer reads while every credential stays valid.
func (s *Store) PauseRoom(ctx context.Context, id ids.DealRoomID) (crmcontracts.DealRoom, error) {
	return s.moveRoom(ctx, id, roomMove{
		to:     statePaused,
		admits: func(current string) bool { return current == stateLive },
		refuse: notPausable,
		action: "pause",
		anchor: anchorRetracts,
		payload: func(dealID openapi_types.UUID) events.Payload {
			return crmcontracts.PublicEventDealRoomPaused{DealId: dealID}
		},
	})
}

// ResumeRoom returns a paused room to live on its existing release. No
// republish: the buyer sees exactly what they saw before the pause.
func (s *Store) ResumeRoom(ctx context.Context, id ids.DealRoomID) (crmcontracts.DealRoom, error) {
	return s.moveRoom(ctx, id, roomMove{
		to:     stateLive,
		admits: func(current string) bool { return current == statePaused },
		refuse: notPaused,
		action: "resume",
		anchor: anchorGrants,
		payload: func(dealID openapi_types.UUID) events.Payload {
			return crmcontracts.PublicEventDealRoomResumed{DealId: dealID}
		},
	})
}

// CloseRoom stops the room taking new work while leaving buyer ACCESS intact:
// the buyer keeps reading it, and neither side adds a document or a comment.
//
// It is not a FREEZE, and saying so matters. What the buyer reads is the live
// room, so a file later archived on the deal, unlinked from it, or hidden from
// its Files area stops being served here too — a closed room is a room nobody
// works in any more, not a preserved copy of one. Anyone needing the second
// thing needs the audit trail, which records every document this room carried
// and when it stopped.
func (s *Store) CloseRoom(ctx context.Context, id ids.DealRoomID) (crmcontracts.DealRoom, error) {
	return s.moveRoom(ctx, id, roomMove{
		to:        stateClosed,
		admits:    func(current string) bool { return current == stateLive || current == statePaused },
		refuse:    notClosable,
		action:    "close",
		anchor:    anchorRetracts,
		stampsCol: "closed_at",
		payload: func(dealID openapi_types.UUID) events.Payload {
			return crmcontracts.PublicEventDealRoomClosed{DealId: dealID}
		},
	})
}

// anchorSide names which half of the archived-anchor rule an operation takes:
// a move that hands access out is frozen by an archived deal, and one that takes
// access away never is. auth.EnsureRetractable states the rule; this is how one
// declarative move descriptor answers it.
type anchorSide uint8

const (
	// The zero value is deliberately nameless: it says nothing, no move may keep
	// it, and moveRoom refuses a descriptor that declares neither side.
	anchorGrants anchorSide = iota + 1
	anchorRetracts
)

// roomMove is one lifecycle transition: which states admit it, what it refuses
// with, and what it announces.
type roomMove struct {
	to     string
	admits func(current string) bool
	refuse func(current string) error
	action string
	// anchor is the side of the archived-anchor rule this move takes, and every
	// move declares one: pause and close TAKE the room's working surface away,
	// so an archived deal must not freeze them, while resume hands buyer reads
	// back and must not run on one. The zero value names neither, and moveRoom
	// refuses it — so a fourth move picks a side in the same literal it is
	// declared in rather than inheriting whichever half came first.
	anchor anchorSide
	// stampsCol names a timestamp column the move sets to now(), or "" when the
	// move records only its new state.
	stampsCol string
	// payload is the event this move announces, built from the room's deal id.
	// events.Payload is the generated interface every public payload satisfies,
	// so storekit.EmitEvent reads the event type and entity type off the value
	// rather than from a string a caller passes beside it.
	payload func(dealID openapi_types.UUID) events.Payload
}

// moveRoom is the shared spelling of the three ACCESS moves — pause, resume and
// close: lock, check the state, write, audit, emit.
//
// Publish and archive deliberately do not come through here. Each writes more
// than a state (a release row; a session revocation) and carries its own audit
// and event for what it actually did, which this function could not describe.
func (s *Store) moveRoom(ctx context.Context, id ids.DealRoomID, move roomMove) (crmcontracts.DealRoom, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionUpdate); err != nil {
		return crmcontracts.DealRoom{}, err
	}
	// Human-only at the store as well as in the contract. The REST gate already
	// refuses an agent on these three routes, but a caller reaching moveRoom
	// from inside the process — a compose orchestration, the buyer edge — would
	// pass that gate by never meeting it. Suspending or ending a buyer's access
	// is a person's act, and this is where that survives a new caller.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.DealRoom{}, err
	}
	var out crmcontracts.DealRoom
	err := s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readRoom(ctx, tx, id)
		if err != nil {
			return err
		}
		switch move.anchor {
		case anchorRetracts:
			err = ensureDealRetractable(ctx, tx, current)
		case anchorGrants:
			err = ensureDealWritable(ctx, tx, current)
		default:
			err = fmt.Errorf("dealrooms: the %q move declares no side of the archived-anchor rule", move.action)
		}
		if err != nil {
			return err
		}
		// Lock before deciding: without it two concurrent pauses both read
		// `live`, both pass the check, and the second writes over a state the
		// first already changed.
		lock, err := storekit.LockRow(ctx, tx, roomObject, id.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		current, err = readRoom(ctx, tx, id)
		if err != nil {
			return err
		}
		from := string(current.State)
		if !move.admits(from) {
			return move.refuse(from)
		}

		p := storekit.NewPatch()
		p.Set("state", from, move.to)
		if move.stampsCol != "" {
			p.Set(move.stampsCol, nil, time.Now().UTC())
		}
		// No If-Match: none of the three access moves takes one on the wire, and
		// the row lock above already serializes them against each other. Written
		// through the lock's own witness rather than as a nil version, so the
		// serialization this relies on is the one the compiler can see — a nil
		// there re-derives the same lock and says nothing about holding it.
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("apply deal room %s: %w", move.action, err)
		}
		auditID, err := storekit.Audit(ctx, tx, move.action, roomObject, id.UUID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit deal room %s: %w", move.action, err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, move.payload(current.DealId)); err != nil {
			return fmt.Errorf("emit deal room %s: %w", move.action, err)
		}
		// Leaving live ends the seller's own preview tabs; a buyer's session
		// is untouched, because a paused or closed room still owes them the
		// paused page or the record.
		if move.to != stateLive {
			if err := endPreviewSessions(ctx, tx, id); err != nil {
				return err
			}
		}
		out, err = readRoom(ctx, tx, id)
		return err
	})
	return out, err
}
