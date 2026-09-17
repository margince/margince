// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:approval-notify consumer: a staged proposal becomes a line in the
// queue of every seat that could actually answer it.
//
// THERE IS NO APPROVER COLUMN, and there is no query that could grow one.
// Whether a seat may decide a staged row is a per-(reader, row) predicate — the
// grants the released effect would spend, the narrowing to the one seat some
// kinds are staged for, and a live probe that the reader may act on the target
// record — so the only honest fan-out is to ask that predicate once per seat.
// approvals answers it (PendingDecidableBy); this file never re-derives it,
// because a second reading of "who may decide" drifts from the inbox and starts
// telling colleagues about cards their own inbox then hides.
//
// It lives here because the question crosses three modules: approvals owns the
// predicate, identity owns the roster and each seat's authority, notices owns
// the line — and no module imports a sibling.
//
// THE ENVELOPE CAN BE STALE. Supersession and withdrawal write `expired` with
// no event of their own, so a row that was pending when the outbox recorded
// this envelope may have stopped being pending long before the lane reads it.
// The re-read inside PendingDecidableBy is what keeps this from announcing a
// card nobody can answer.
//
// Idempotency is the notice table's own unique index on (recipient, dedupe key),
// not this handler and not the Redis dedupe cache the subscriber wraps it in:
// that cache marks AFTER the effect, so a crash in the window replays, and
// replay has to be free. Which is also why a failing seat may fail the whole
// delivery — the retry writes nothing twice.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalNotifyActor names this consumer in the audit rows it causes. A line
// placed in somebody's queue has to say what placed it there, and the answer is
// never the reader themselves.
const approvalNotifyActor = "system:approval-notify"

// unsummarisedProposal is what a seat is shown for a staging that composed no
// summary. The store refuses a notice with no subject, and the honest sentence
// is that something is waiting rather than a guess at what.
const unsummarisedProposal = "A proposal is waiting for your decision"

// ApprovalNotify announces a staged approval to the seats that could decide it.
type ApprovalNotify struct {
	db        *database.DB
	approvals *approvals.Service
	notices   *notices.Store
	identity  *identity.Service
}

// NewApprovalNotify builds the consumer over the installation's handle.
func NewApprovalNotify(pool *pgxpool.Pool, db *database.DB) *ApprovalNotify {
	return &ApprovalNotify{
		db:        db,
		approvals: approvals.NewService(db),
		notices:   notices.NewStore(db),
		identity:  identity.NewService(pool),
	}
}

// HandleEvent announces one staged approval. Anything else answers nil so the
// group keeps flowing rather than wedging on traffic this consumer ignores.
func (a *ApprovalNotify) HandleEvent(ctx context.Context, env events.Envelope) error {
	// The generated constant, not a hand-typed event name: a spelling that
	// drifts from the contract makes this consumer silently never fire, and
	// nothing fails when a consumer does nothing.
	if env.Type != string(crmcontracts.ApprovalRequested) || env.Entity.ID == ids.Nil {
		return nil
	}
	var staged crmcontracts.PublicEventApprovalRequested
	if err := json.Unmarshal(env.Payload, &staged); err != nil {
		return fmt.Errorf("approval notify: approval.requested payload: %w", err)
	}
	ws, err := a.db.Workspace(ctx)
	if err != nil {
		return err
	}
	// A subscriber carries no workspace, no actor and no trace. Without the
	// workspace every read is refused; without the actor the audited write below
	// has nobody to name; without the causing event the audit row cannot be tied
	// back to the staging that produced it.
	sysCtx := principal.WithWorkspaceID(ctx, ws.UUID)
	sysCtx = principal.WithCausationEvent(sysCtx, env.EventID)
	sysCtx = principal.WithActor(sysCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: approvalNotifyActor,
	})
	seats, err := a.candidateSeats(sysCtx)
	if err != nil {
		return err
	}
	return a.announce(sysCtx, ws.UUID, ids.From[ids.ApprovalKind](env.Entity.ID), staged, seats)
}

// candidateSeats is the roster the predicate is then asked about: the live,
// human, full seats of this installation.
//
// Deactivated and archived members are excluded because they can decide
// nothing; agents because a decision is a human's, which the decide path
// refuses anyway; read seats because deciding a proposal releases a WRITE, so a
// read seat is told about work it could never do.
//
// Narrowing here rather than leaning on the predicate is what keeps this from
// being a full-table probe per staging, and the predicate stays the authority
// on every seat this does admit.
func (a *ApprovalNotify) candidateSeats(ctx context.Context) ([]ids.UUID, error) {
	var seats []ids.UUID
	err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT u.id
			  FROM app_user u
			 WHERE `+identity.LiveMemberSQL("u")+`
			   AND u.is_agent = false
			   AND u.seat_type = 'full'
			 ORDER BY u.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var seat ids.UUID
			if err := rows.Scan(&seat); err != nil {
				return err
			}
			seats = append(seats, seat)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("approval notify: listing the seats that might decide: %w", err)
	}
	return seats, nil
}

// announce walks the roster, telling each seat that could decide.
//
// ONE SEAT'S FAILURE DOES NOT COST THE OTHERS THEIRS: the loop records what
// went wrong and carries on, then fails the delivery with every cause joined. A
// pass that stopped at the first failure would leave a whole workspace
// unnotified because one seat had a broken authority row, and the redelivery
// would keep hitting that seat first. Failing at the end is still right — the
// dedupe key makes the retry write nothing twice, so the only cost of a retry
// is the seats it already served being asked again.
func (a *ApprovalNotify) announce(
	ctx context.Context, wsID ids.UUID, approvalID ids.ApprovalID,
	staged crmcontracts.PublicEventApprovalRequested, seats []ids.UUID,
) error {
	var failures []error
	for _, seat := range seats {
		if err := a.announceToSeat(ctx, wsID, seat, approvalID, staged); err != nil {
			failures = append(failures, fmt.Errorf("seat %s: %w", seat, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("approval notify %s: %d of %d seat(s) unresolved: %w",
			approvalID, len(failures), len(seats), errors.Join(failures...))
	}
	return nil
}

// announceToSeat asks one seat's own authority whether this card is theirs to
// answer, and records the line if it is.
//
// The PROBE runs under the seat, and the WRITE does not. Deciding is bounded by
// what this colleague holds, so the question has to be asked as them; the notice
// is the product's own line about that card, and captured_by comes from the
// acting principal — writing it as the seat would record the reader as the
// author of the sentence addressed to them.
func (a *ApprovalNotify) announceToSeat(
	ctx context.Context, wsID, seat ids.UUID, approvalID ids.ApprovalID,
	staged crmcontracts.PublicEventApprovalRequested,
) error {
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	seatCtx, live, err := a.seatContext(ctx, wsID, seat)
	if err != nil || !live {
		return err
	}
	decidable, err := a.approvals.PendingDecidableBy(seatCtx, approvalID)
	if err != nil || !decidable {
		return err
	}
	// CreateTx rather than Create, so the seat's own preference decides inside
	// the same transaction the row would land in: a class this colleague
	// switched off writes nothing at all — no row, no audit entry, no
	// announcement — and answers the zero id this caller already ignores.
	return a.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := a.notices.CreateTx(ctx, tx, approvalNoticeFor(seat, approvalID, staged))
		return err
	})
}

// seatContext binds one seat's own authority, and a fresh correlation id so
// each colleague's line is one recoverable trace rather than a fan-out nobody
// can take apart.
//
// EffectiveAuthority reads the grants AND the seat as one snapshot, including
// the seat type: composed from separate reads they can describe an authority
// this colleague never held — permissions from before a role change with a seat
// from after — and the decision gate would then be asked about a principal that
// does not exist.
//
// live is false for a seat that stopped being one between the roster read and
// this resolve. That is a race rather than a fault: the colleague simply has no
// decision to be told about, and failing the whole delivery over it would strand
// everyone else's line behind a seat that is already gone.
func (a *ApprovalNotify) seatContext(
	ctx context.Context, wsID, seat ids.UUID,
) (context.Context, bool, error) {
	rbac, seatType, err := a.identity.EffectiveAuthority(ctx, wsID, seat)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolving the seat's authority: %w", err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          principal.HumanIDPrefix + seat.String(),
		UserID:      seat,
		SeatType:    seatType,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	}), true, nil
}

// approvalNoticeFor is the line one decidable seat is shown.
//
// The subject is the STAGED summary, which approvals already sanitized and
// bounded at staging and the store bounds again to its own subject length — the
// one sentence of an approval that is prose, and the one a colleague reads
// before deciding. The body is deliberately empty: everything else on the card
// is structured, the target below is what opens it, and a paragraph invented
// here would be this consumer's words rather than the proposal's.
//
// The dedupe key names the APPROVAL, so every redelivery of this envelope — and
// any later announcement about the same staging — collapses onto the one line
// each seat already has.
func approvalNoticeFor(
	seat ids.UUID, approvalID ids.ApprovalID, staged crmcontracts.PublicEventApprovalRequested,
) notices.NewNotice {
	subject := staged.Summary
	if subject == "" {
		subject = unsummarisedProposal
	}
	return notices.NewNotice{
		Recipient: ids.From[ids.UserKind](seat),
		Kind:      notices.KindApprovalPending,
		Subject:   subject,
		DedupeKey: notices.KindApprovalPending + ":" + approvalID.String(),
		Target:    approvalNoticeTarget(staged),
	}
}

// approvalNoticeTarget is the record the card is about, when it is about one. A
// staging that names no row — a step-up, a cold-start proposal — leaves the zero
// value, which the store writes as two NULLs.
func approvalNoticeTarget(staged crmcontracts.PublicEventApprovalRequested) notices.Target {
	if staged.TargetEntityId == nil {
		return notices.Target{}
	}
	return notices.Target{Type: staged.TargetEntityType, ID: ids.UUID(*staged.TargetEntityId)}
}
