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
	ID          ids.UUID
	LeadID      *ids.UUID
	PersonID    *ids.UUID
	CompanyID   *ids.UUID
	SubmittedBy ids.UUID
	AssignedTo  *ids.UUID
	Status      string
	ReasonID    *ids.UUID
	Note        string
	DealID      *ids.UUID
	SubmittedAt string
	DecidedAt   *string
	Version     int64
}

// NewSDRHandoff is one prospect being handed on.
//
// Exactly one of LeadID and PersonID: an SDR working an inbound list has leads,
// one working an account has people, and a row claiming both would be two
// handoffs wearing one id.
type NewSDRHandoff struct {
	LeadID    *ids.UUID
	PersonID  *ids.UUID
	CompanyID *ids.UUID
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
		// The three prospect references are CALLER-SUPPLIED, so each is a read of
		// the record it names and takes the target probe before it lands.
		//
		// auth.Require above answers "may this role hand prospects on at all",
		// which is a different question from "may this seat hand on THIS one".
		// Without the probe a seat could name a lead outside its scope and learn
		// from the outcome that the id exists — and the handoff it wrote would
		// then point at a record its own author cannot open. ErrNotFound for an
		// id out of scope, the same answer an id that does not exist gets.
		if err := ensureHandoffTargetsVisible(ctx, tx, in); err != nil {
			return err
		}
		// captured_by comes from the authenticated principal and never from the
		// request body — the write shape's rule, and the reason a handoff can be
		// attributed at all.
		if err := tx.QueryRow(ctx, `
			INSERT INTO sdr_handoff (lead_id, person_id, company_id, submitted_by, assigned_to, note, captured_by)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''), $7)
			RETURNING id`,
			in.LeadID, in.PersonID, in.CompanyID, actor.UserID, in.AssignedTo, in.Note, actor.ID,
		).Scan(&id); err != nil {
			return fmt.Errorf("people: submitting the handoff: %w", err)
		}
		if err := appendHandoffEvent(ctx, tx, id, HandoffSubmitted, nil, nil, in.Note, actor.ID); err != nil {
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
		// The SUBJECT's write scope, not the handoff's. A handoff is a record
		// about a lead or a person, and a seat that may not write that record has
		// no business deciding what happens to it — auth.Require above answers
		// "may this role decide handoffs at all", which is a different question
		// from "may this seat touch THIS prospect". Without it any seat holding
		// lead:update could accept a colleague's handoff on an account they
		// cannot see.
		if err := ensureHandoffSubjectWritable(ctx, tx, id); err != nil {
			return err
		}
		// An INACTIVE reason is not a choice a decider may make today. The row
		// stays for the history that points at it — deactivating is how an
		// operator retires a reason without orphaning last quarter's rejections —
		// but a new decision citing one would record a reason nobody is offered.
		if err := refuseRetiredReason(ctx, tx, in.ReasonID); err != nil {
			return err
		}
		// The deal an acceptance links is caller-supplied like the prospect
		// references on the submit, and is gated on the same terms: without this
		// an AE could anchor a handoff to a deal they cannot open, and the
		// held-meeting conversion would then read that deal through a link its
		// own author had no scope for.
		if in.DealID != nil {
			if err := auth.EnsureLinkTarget(ctx, tx, "deal", *in.DealID); err != nil {
				return err
			}
		}
		kind := reasonKindFor(in)
		tag, err := tx.Exec(ctx, `
			UPDATE sdr_handoff
			   SET status = $2, reason_id = $3, reason_applies_to = $4,
			       note = coalesce(nullif($5, ''), note),
			       deal_id = $6,
			       decided_at = CASE WHEN $2 IN ('accepted', 'rejected') THEN now() END,
			       -- An acceptance CLAIMS the handoff for whoever accepted it. A
			       -- queue handoff is offered to nobody in particular, and leaving
			       -- assigned_to null after somebody took it loses the one fact an
			       -- "who owns this now" read needs.
			       assigned_to = CASE WHEN $2 = 'accepted' THEN $7::uuid ELSE assigned_to END,
			       updated_at = now(), version = version + 1
			 WHERE id = $1 AND status = $8`,
			id, in.Status, in.ReasonID, kind, in.Note, in.DealID, actor.UserID, HandoffSubmitted)
		if err != nil {
			return fmt.Errorf("people: deciding the handoff: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return handoffDecisionMiss(ctx, tx, id)
		}
		if err := appendHandoffEvent(ctx, tx, id, in.Status, in.ReasonID, kind, in.Note, actor.ID); err != nil {
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
	ctx context.Context, tx pgx.Tx, id ids.UUID, status string, reasonID *ids.UUID, reasonKind *string, note, actor string,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO sdr_handoff_event (handoff_id, to_status, reason_id, reason_applies_to, note, actor)
		VALUES ($1, $2, $3, $4, nullif($5, ''), $6)`, id, status, reasonID, reasonKind, note, actor); err != nil {
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

// ensureHandoffTargetsVisible puts every record a submitted handoff NAMES
// through the target probe, in the transaction that is about to reference it.
//
// All three arrive from the request body. The subject — a lead or a person,
// exactly one — is what the handoff is about, and the company is the
// company it is filed under; a reference to any of them is a read of it.
func ensureHandoffTargetsVisible(ctx context.Context, tx pgx.Tx, in NewSDRHandoff) error {
	targets := []struct {
		table string
		id    *ids.UUID
	}{
		{"lead", in.LeadID},
		{"person", in.PersonID},
		{entityCompany, in.CompanyID},
	}
	for _, t := range targets {
		if t.id == nil {
			continue
		}
		if err := auth.EnsureLinkTarget(ctx, tx, t.table, *t.id); err != nil {
			return err
		}
	}
	return nil
}

// ensureHandoffSubjectWritable applies the SUBJECT's row scope to a handoff
// decision.
//
// The handoff has no owner column of its own to scope by, and inventing one
// would be a second answer to a question the lead and the person already
// answer. So the gate is the subject's: a seat that may write this prospect may
// decide what happens to it, and one that may not gets the 404 every other
// out-of-scope write gets, so the refusal does not disclose that the handoff is
// there.
func ensureHandoffSubjectWritable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var leadID, personID *ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT lead_id, person_id FROM sdr_handoff WHERE id = $1`, id).Scan(&leadID, &personID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("people: reading the handoff's subject: %w", err)
	}
	// Exactly one is set — the sdr_handoff_one_subject constraint holds it — so
	// this reaches the table that actually carries the row scope.
	if leadID != nil {
		return auth.EnsureWritable(ctx, tx, "lead", *leadID)
	}
	if personID != nil {
		return auth.EnsureWritable(ctx, tx, "person", *personID)
	}
	return errHandoffNeedsOneSubject
}

// reasonKindFor is the reason's own category, carried beside its id so the
// database can check the pair.
//
// It is DERIVED from the decision rather than taken from the caller: the two
// must agree, and a caller that could supply both could supply a mismatched
// pair for the composite key to reject — a 500 where the product means to
// refuse a rejection citing a recycle reason with a sentence.
func reasonKindFor(in HandoffDecision) *string {
	if in.ReasonID == nil {
		return nil
	}
	kind := in.Status
	return &kind
}

// ResubmitHandoff puts a recycled handoff back in front of a receiver.
//
// Recycling is the one decision that does not end a handoff: it says "not now,
// worth another try", and a prospect that could never be handed on again would
// make that an elaborate rejection. The CAS is `status = recycled`, so this
// reopens exactly the handoffs that were sent back and nothing else.
//
// The reason and the decision moment go with it. A reopened handoff is waiting
// again, and a reason left standing would answer "why was this refused" about a
// round that is no longer the current one — the history is where that answer
// lives.
func (s *Store) ResubmitHandoff(ctx context.Context, id ids.UUID, assignTo *ids.UUID) error {
	if err := auth.Require(ctx, rbacHandoff, principal.ActionUpdate); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return errNoActorForHandoff
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureHandoffSubjectWritable(ctx, tx, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE sdr_handoff
			   SET status = $2, reason_id = NULL, reason_applies_to = NULL, decided_at = NULL,
			       assigned_to = coalesce($3::uuid, assigned_to),
			       submitted_at = now(), updated_at = now(), version = version + 1
			 WHERE id = $1 AND status = $4`,
			id, HandoffSubmitted, assignTo, HandoffRecycled)
		if err != nil {
			return fmt.Errorf("people: resubmitting the handoff: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errHandoffNotRecycled
		}
		if err := appendHandoffEvent(ctx, tx, id, HandoffSubmitted, nil, nil, "", actor.ID); err != nil {
			return err
		}
		if _, err := storekit.AuditEvent(ctx, tx, "update", "sdr_handoff", id, map[string]any{
			handoffStatusKey: HandoffSubmitted, "resubmitted": true,
		}); err != nil {
			return err
		}
		return nil
	})
}

// refuseRetiredReason keeps a deactivated reason out of a NEW decision.
func refuseRetiredReason(ctx context.Context, tx pgx.Tx, reasonID *ids.UUID) error {
	if reasonID == nil {
		return nil
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT active FROM sdr_handoff_reason WHERE id = $1`, *reasonID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("people: reading the reason's standing: %w", err)
	}
	if !active {
		return errHandoffReasonRetired
	}
	return nil
}

var errHandoffReasonRetired = errors.New(
	"that reason has been retired: it stays on the handoffs already decided for it, and is not one to choose now")

var errHandoffNotRecycled = errors.New(
	"only a recycled handoff is resubmitted: an open one is already waiting, and a decided one is closed")
