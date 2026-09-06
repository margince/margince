// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The SDR-to-AE handoff: a prospect qualified by one seat and passed to another.
//
// NOT the delivery briefing the agent surface calls a handoff. prepare_handoff
// there tells a delivery team what was sold on a project that closed; this is a
// SALES handoff of a prospect that has not closed and may never. The two share a
// word and nothing else, which is why this one is spelled sdr_handoff wherever
// it appears.
//
// WHY IT LIVES IN people. The subject is a lead or a person, both of which this
// module owns; the deal an acceptance produces is an OUTCOME, and a module never
// imports a sibling. The acceptance therefore takes a deal id rather than
// creating a deal — compose wires the caller that does both, the way it wires
// every other edge between two modules.
//
// The rejection reason is ADMINISTERED, not free text: a reason nobody can count
// is a reason nobody acts on. sdr_handoff_reason is the same shape
// lead_disqualify_reason already has, so an operator configuring one has learned
// the other.
//
// Visibility follows the standing rule — everyone reads everything except
// correspondence — so an SDR sees a colleague's rejected handoff and a manager
// sees all of them. That is the decided behaviour, and it is what makes
// "why were my handoffs rejected" answerable across a team rather than one seat
// at a time.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The closed set of handoff states.
const (
	// HandoffSubmitted is offered and not yet decided.
	HandoffSubmitted = "submitted"
	// HandoffAccepted is taken on by an owner, with the deal it produced.
	HandoffAccepted = "accepted"
	// HandoffRejected is refused: this prospect is not work the receiver will
	// take, for a reason the catalog names.
	HandoffRejected = "rejected"
	// HandoffRecycled is sent back to the SDR rather than refused outright —
	// worth another try, not now.
	HandoffRecycled = "recycled"
)

// handoffStatusKey names the handoff's own status in an audit payload.
//
// Its own constant rather than leadStatusColumn beside it: that one means the
// LEAD's status, and the two vocabularies happen to share a spelling today
// while answering different questions. Borrowing it would make a rename of one
// silently rename the other.
const handoffStatusKey = "status"

// rbacHandoff is the object every entry point below gates on. The handoff is a
// sales record about a prospect, so it answers to the same object the lead and
// the person do rather than minting a permission of its own.
const rbacHandoff = "lead"

var (
	errHandoffNeedsOneSubject = errors.New(
		"a handoff names either a lead or a person, and needs exactly one of them")
	errHandoffNeedsReason = errors.New(
		"a refused or recycled handoff names the reason it was, from the workspace's own list")
	errHandoffAlreadyDecided = errors.New(
		"this handoff has already been decided; a second decision would overwrite the first")
)

// SDRHandoff is one handoff as a reader sees it.
type SDRHandoff struct {
	ID             ids.UUID
	LeadID         *ids.UUID
	PersonID       *ids.UUID
	OrganizationID *ids.UUID
	SubmittedBy    ids.UUID
	AssignedTo     *ids.UUID
	Status         string
	ReasonID       *ids.UUID
	Note           string
	DealID         *ids.UUID
	SubmittedAt    string
	DecidedAt      *string
	Version        int64
}

// NewSDRHandoff is one prospect being handed on.
//
// Exactly one of LeadID and PersonID: an SDR working an inbound list has leads,
// one working an account has people, and a row claiming both would be two
// handoffs wearing one id.
type NewSDRHandoff struct {
	LeadID         *ids.UUID
	PersonID       *ids.UUID
	OrganizationID *ids.UUID
	// AssignedTo is optional. A handoff may be offered to a named AE or left for
	// whoever picks it up, and the second is a real workflow rather than an
	// unfinished first: a team with a shared queue has nobody to name yet.
	AssignedTo *ids.UUID
	Note       string
}

// SubmitHandoff records a prospect being handed on.
func (s *Store) SubmitHandoff(ctx context.Context, in NewSDRHandoff) (ids.UUID, error) {
	if err := auth.Require(ctx, rbacHandoff, principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	if (in.LeadID == nil) == (in.PersonID == nil) {
		return ids.UUID{}, errHandoffNeedsOneSubject
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}, errNoActorForHandoff
	}
	var id ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// captured_by comes from the authenticated principal and never from the
		// request body — the write shape's rule, and the reason a handoff can be
		// attributed at all.
		if err := tx.QueryRow(ctx, `
			INSERT INTO sdr_handoff (lead_id, person_id, organization_id, submitted_by, assigned_to, note, captured_by)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''), $7)
			RETURNING id`,
			in.LeadID, in.PersonID, in.OrganizationID, actor.UserID, in.AssignedTo, in.Note, actor.ID,
		).Scan(&id); err != nil {
			return fmt.Errorf("people: submitting the handoff: %w", err)
		}
		if err := appendHandoffEvent(ctx, tx, id, HandoffSubmitted, nil, in.Note, actor.ID); err != nil {
			return err
		}
		if _, err := storekit.AuditEvent(ctx, tx, "create", "sdr_handoff", id, map[string]any{
			handoffStatusKey: HandoffSubmitted, "assigned_to": in.AssignedTo,
		}); err != nil {
			return err
		}
		return nil
	})
	return id, err
}

// HandoffDecision is an AE answering a handoff.
type HandoffDecision struct {
	Status string
	// ReasonID is required for a rejection or a recycle and refused otherwise —
	// a reason on an acceptance would describe a refusal that never happened.
	ReasonID *ids.UUID
	Note     string
	// DealID is what an acceptance links or created. It is the anchor the
	// held-meeting conversion reads, which is why it belongs to the acceptance
	// TRANSACTION rather than being inferred later from a deal that happens to
	// share a company: that inference credits an SDR for work they may not have
	// done, and the error grows with account size.
	DealID *ids.UUID
}

// DecideHandoff answers a handoff, once.
//
// The status predicate is the CAS: a handoff already decided reports
// errHandoffAlreadyDecided rather than taking a second answer, because two
// decisions on one handoff is two stories about what happened to one prospect.
func (s *Store) DecideHandoff(ctx context.Context, id ids.UUID, in HandoffDecision) error {
	if err := auth.Require(ctx, rbacHandoff, principal.ActionUpdate); err != nil {
		return err
	}
	if err := validateHandoffDecision(in); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return errNoActorForHandoff
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE sdr_handoff
			   SET status = $2, reason_id = $3, note = coalesce(nullif($4, ''), note),
			       deal_id = $5, decided_at = now(), updated_at = now(), version = version + 1
			 WHERE id = $1 AND status = $6`,
			id, in.Status, in.ReasonID, in.Note, in.DealID, HandoffSubmitted)
		if err != nil {
			return fmt.Errorf("people: deciding the handoff: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return handoffDecisionMiss(ctx, tx, id)
		}
		if err := appendHandoffEvent(ctx, tx, id, in.Status, in.ReasonID, in.Note, actor.ID); err != nil {
			return err
		}
		if _, err := storekit.AuditEvent(ctx, tx, "update", "sdr_handoff", id, map[string]any{
			handoffStatusKey: in.Status, "reason_id": in.ReasonID, "deal_id": in.DealID,
		}); err != nil {
			return err
		}
		return nil
	})
}

// handoffDecisionMiss says WHY the update matched nothing.
//
// The two cases read identically from RowsAffected and mean opposite things: a
// handoff nobody may see, and one already answered. Reporting "not found" for
// the second would send an AE looking for a record that is on their screen.
func handoffDecisionMiss(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM sdr_handoff WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("people: reading the handoff's standing: %w", err)
	}
	return errHandoffAlreadyDecided
}

// validateHandoffDecision holds the reason rule in Go as well as in the CHECK
// constraint, so a caller gets a sentence rather than a constraint violation.
// The database is still what makes it true.
func validateHandoffDecision(in HandoffDecision) error {
	switch in.Status {
	case HandoffAccepted:
		if in.ReasonID != nil {
			return errHandoffReasonOnAcceptance
		}
	case HandoffRejected, HandoffRecycled:
		if in.ReasonID == nil {
			return errHandoffNeedsReason
		}
		if in.DealID != nil {
			return errHandoffDealOnRefusal
		}
	default:
		return fmt.Errorf("%q is not a decision a handoff takes: accepted, rejected or recycled", in.Status)
	}
	return nil
}

// appendHandoffEvent records one transition. Both writers go through it, so the
// submission and every later decision cannot come to be recorded two ways.
func appendHandoffEvent(
	ctx context.Context, tx pgx.Tx, id ids.UUID, status string, reasonID *ids.UUID, note, actor string,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO sdr_handoff_event (handoff_id, to_status, reason_id, note, actor)
		VALUES ($1, $2, $3, nullif($4, ''), $5)`, id, status, reasonID, note, actor); err != nil {
		return fmt.Errorf("people: recording the handoff transition: %w", err)
	}
	return nil
}

var (
	errNoActorForHandoff = errors.New(
		"people: a handoff records who submitted and who decided it, so it needs an authenticated actor")
	errHandoffReasonOnAcceptance = errors.New(
		"an accepted handoff carries no rejection reason — the reason describes a refusal that did not happen")
	errHandoffDealOnRefusal = errors.New(
		"a refused handoff carries no deal: attaching one would credit the submitter for work their handoff was refused for")
)
