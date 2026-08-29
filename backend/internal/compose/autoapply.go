// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Applying a reversible proposal without asking, under the standing policy of
// the rep who owns the record it changes.
//
// The product's autonomy posture: a change the rep can put back applies now and
// lands in the day's "Done for you" lane with an Undo, rather than waiting in a
// queue for a click that adds nothing. What is NOT here is as deliberate — an
// outbound send and a merge keep asking, because nothing reverses a message a
// customer has read.
//
// UNDO IS NOT BUILT HERE. An automatic apply writes an ordinary audit row, and
// the existing record-history restore route reverses it like any other change
// (undoability.go computes whether it can, and says why when it cannot). A
// second reversal path would be a second answer to a question the audit spine
// already answers.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// autoApplyActorID names the machine in the audit trail. The row also carries
// on_behalf_of, so a reader sees both halves: the product acted, for this rep.
const autoApplyActorID = "agent:auto-apply"

// ownedTables maps the target types an automatic apply can serve to the table
// holding their owner.
//
// A closed map rather than a formatted table name: the type comes off an
// approval row, and a table name assembled from stored text is how a catalog
// name becomes an injection. A target type absent here is not applied — which
// is the honest answer for a record whose owner this cannot establish.
//
// Two entries, because two is what the eligible kinds can name: a close-date
// correction targets a deal, and both a rename and a lifecycle move target an
// organization. A kind joining AutoApplyKinds against another record type adds
// its row here, and until then a speculative entry would be a table this can
// reach that nothing asks it to.
//
//nolint:goconst // wire record-type names read as data; a shared constant would tie this map to whichever other concept spells the same word
var ownedTables = map[string]string{
	"deal":         "deal",
	"organization": "organization",
}

// autoApplier decides and applies proposals that their owner has put on
// automatic.
type autoApplier struct {
	pool  *pgxpool.Pool
	svc   *approvals.Service
	users *identity.Service
}

// Apply runs one pending proposal if its owner has this kind on automatic.
//
// It reports whether it applied. Not applying is the ordinary outcome and never
// an error: an unowned record, a departed owner, a rep who has not opted in and
// a kind that is not eligible all mean the same thing to the product — the
// proposal stays in the queue for a person to answer.
//
// The owner is resolved at APPLY time rather than carried on the staged row, so
// a handover moves who an automatic apply acts for. That also means a proposal
// staged weeks ago cannot apply under the authority of somebody who has since
// left: EffectiveAuthority answers ErrNotFound for an archived or suspended
// member, and this treats that as "do not apply".
func (a autoApplier) Apply(ctx context.Context, approvalID ids.ApprovalID) (bool, error) {
	target, err := a.proposalToApply(ctx, approvalID)
	if err != nil {
		return false, err
	}
	if target.kind == "" || !approvals.AutoApplyKinds[target.kind] {
		return false, nil
	}
	owner, err := a.ownerOf(ctx, target.entityType, target.entityID)
	if err != nil || owner.IsZero() {
		return false, err
	}
	ownerCtx, err := a.asOwnersAgent(ctx, owner)
	if err != nil {
		// A refusal to establish the owner's authority is a refusal to apply,
		// not a failure of the caller that offered the proposal. The row keeps
		// waiting for a person, which is the safe direction.
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	// Read the policy AS the owner, not before becoming them: the policy read
	// takes its subject from the acting principal, so asking first would ask
	// about whoever the sweep happened to be running as.
	mode, err := a.svc.AutoApplyMode(ownerCtx, target.kind)
	if err != nil {
		return false, err
	}
	if mode != approvals.ModeAuto {
		return false, nil
	}
	if _, err := a.svc.ApplyUnderPolicy(ownerCtx, approvalID); err != nil {
		return false, err
	}
	return true, nil
}

// proposalTarget is the proposal's kind and what it points at.
type proposalTarget struct {
	kind       string
	entityType string
	entityID   ids.UUID
}

// proposalToApply reads the pending row's kind and target.
//
// Only a PENDING row: a decided proposal has already been answered, and
// applying one again would run its effect twice. The status test lives in the
// query rather than in Go so the read and the test are one statement.
func (a autoApplier) proposalToApply(ctx context.Context, approvalID ids.ApprovalID) (proposalTarget, error) {
	var out proposalTarget
	var entityType *string
	var entityID *ids.UUID
	err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT kind, target_entity_type, target_entity_id
			  FROM approval
			 WHERE id = $1 AND status = 'pending' AND passport_id IS NULL`,
			approvalID).Scan(&out.kind, &entityType, &entityID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return proposalTarget{}, nil
	}
	if err != nil {
		return proposalTarget{}, fmt.Errorf("compose: reading the proposal to apply: %w", err)
	}
	if entityType == nil || entityID == nil {
		return proposalTarget{}, nil
	}
	out.entityType, out.entityID = *entityType, *entityID
	return out, nil
}

// ownerOf reads the current owner of the record a proposal changes.
//
// A record with no owner yields the zero id and no error: nobody's standing
// policy covers it, so it is not a proposal anybody has agreed to apply
// automatically. That is the reading consistent with never applying under an
// authority nobody currently holds.
func (a autoApplier) ownerOf(ctx context.Context, entityType string, entityID ids.UUID) (ids.UUID, error) {
	table, ok := ownedTables[entityType]
	if !ok {
		return ids.Nil, nil
	}
	var owner *ids.UUID
	err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		// The table name is a value from ownedTables, a compile-time literal
		// chosen by a key that had to match — never text off the approval row.
		return tx.QueryRow(ctx,
			`SELECT owner_id FROM `+table+` WHERE id = $1 AND archived_at IS NULL`,
			entityID).Scan(&owner)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: reading the owner of a %s: %w", entityType, err)
	}
	if owner == nil {
		return ids.Nil, nil
	}
	return *owner, nil
}

// asOwnersAgent binds the product acting FOR the owner.
//
// An agent principal carrying OnBehalfOf, never the system principal, because
// what this principal must pass is the DECISION: decidable() checks the
// staged action's own grants against these permissions and the target's
// visibility against this row scope, and a system principal passes both
// unconditionally. Resolving the owner's real grants at apply time is what
// makes an ownership or role change landing after staging count.
//
// What it does NOT bound is every write that follows. Two of the three
// eligible effects deliberately swap in a system principal before writing —
// an org rename and a lifecycle move stamp their own machine provenance — so
// the honest statement is that the DECISION is gated by the owner's authority
// and the effect then runs exactly as it does after a human's click. That is
// the same bound a person gets, which is the point: this path is not a wider
// authority than the button, only an unattended one.
//
// actingForAHuman admits this shape already: an agent naming the person it acts
// for is somebody's agent, and a credential nobody lent is what it refuses.
//
// dealOwnerAuthority.asOwner (dealownerseam.go) binds an owner the same way and
// is deliberately not shared with this: it builds a PrincipalHuman for a READ,
// where an OnBehalfOf and a scope ceiling would mean nothing. This one must
// carry both, because what it binds goes on to release a decision.
func (a autoApplier) asOwnersAgent(ctx context.Context, owner ids.UUID) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: applying under policy outside a bound workspace")
	}
	rbac, seat, err := a.users.EffectiveAuthority(ctx, wsID, owner)
	if err != nil {
		return nil, fmt.Errorf("compose: authority of the owner an apply would act for: %w", err)
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:       principal.PrincipalAgent,
		ID:         autoApplyActorID,
		UserID:     owner,
		OnBehalfOf: owner,
		SeatType:   seat,
		TeamIDs:    rbac.TeamIDs,
		// Write, and ONLY write. An agent credential's scopes are the ceiling
		// its lender set, and deciding a staged action spends the write cap —
		// so a credential carrying none is refused, which is what the empty
		// zero value would have produced. Granting `send` here instead of
		// leaving it out is what would matter: the eligible kinds change fields
		// on one record and none of them puts a message on the wire, and a
		// credential that could would be one an automatic pass could use to
		// mail a customer.
		Scopes:      principal.ScopeSet{principal.ScopeWrite: struct{}{}},
		Permissions: rbac.Permissions,
	}), nil
}

// duePending lists pending proposals of the eligible kinds, oldest first.
//
// Bounded: a sweep that read every pending row would hold a growing result set
// on a table that grows with the installation, and the rows it did not reach
// this minute are still pending the next. Oldest first so a backlog drains in
// the order it accumulated rather than starving the earliest proposals.
//
// Server-proposed only (passport_id IS NULL). An agent-staged row is redeemed
// by the agent re-issuing its own call, so approving one here would mark a
// decision whose work nobody performs.
func (a autoApplier) duePending(ctx context.Context, limit int) ([]ids.ApprovalID, error) {
	var due []ids.ApprovalID
	err := database.WithWorkspaceTx(ctx, a.pool, func(tx pgx.Tx) error {
		// The kinds come from the module that owns them, in its order: the scan
		// must not come to look for a set the applier would then refuse, and the
		// kinds travel as one parameter, so a map-ordered list would send the
		// same set differently on every tick and make two runs incomparable.
		rows, err := tx.Query(ctx, `
			SELECT id FROM approval
			 WHERE status = 'pending' AND passport_id IS NULL
			   AND expires_at > now()
			   AND kind = ANY($1)
			 ORDER BY created_at
			 LIMIT $2`, approvals.SortedAutoApplyKinds(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.ApprovalID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			due = append(due, id)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("compose: listing proposals that may apply automatically: %w", err)
	}
	return due, nil
}

// autoApplySweepBatch bounds one pass. Large enough that an ordinary
// installation drains in one tick, small enough that a flood cannot hold a
// transaction open across thousands of applies.
const autoApplySweepBatch = 200

// Sweep applies every due proposal whose owner has said to, and reports how
// many it applied.
//
// One proposal's refusal never stops the pass, and the batch is ORDERED, which
// is what makes that rule load-bearing rather than tidy. A row that can never
// apply sits at the head of every tick until it expires, so ending the sweep on
// it would park every other rep's automation behind one stale proposal —
// reachable by anyone who can edit the record a pinned proposal names.
//
// A genuine fault still ends the pass, because continuing would report a clean
// sweep over rows it never read.
func (a autoApplier) Sweep(ctx context.Context) (int, error) {
	due, err := a.duePending(ctx, autoApplySweepBatch)
	if err != nil {
		return 0, err
	}
	var applied int
	for _, id := range due {
		ok, err := a.Apply(ctx, id)
		switch {
		case err == nil:
			if ok {
				applied++
			}
		case refusesThisRow(err):
			// The row itself cannot apply and says why on its own record: the
			// decision path marks a failed effect on the approval, so the
			// proposal is visible as needing a person rather than silently
			// skipped here.
			continue
		default:
			return applied, err
		}
	}
	return applied, nil
}

// refusesThisRow reports whether an error is about ONE proposal rather than
// about the pass.
//
// Version skew is the case that forces this to exist and the reason it is not
// exotic: a close-date proposal pins the deal version it was staged against, so
// anybody editing that deal afterwards makes its apply fail forever. That is an
// ordinary edit, not a race, and it must not stop the rows behind it.
//
// The others are the same shape. A decision already taken, a target the owner
// may no longer reach, a redemption whose token no longer fits — each is a fact
// about that proposal, and each leaves it pending for a person.
func refusesThisRow(err error) bool {
	var decided *approvals.AlreadyDecidedError
	return errors.As(err, &decided) ||
		errors.Is(err, apperrors.ErrVersionSkew) ||
		errors.Is(err, apperrors.ErrApprovalTokenInvalid) ||
		errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrPermissionDenied)
}

// SweepAutoApply runs one auto-apply pass over the bound workspace.
//
// The exported seam the job's Work calls, and the one an integration test drives
// to prove the pass end to end. It exists so the applier's own fields stay
// unexported: what a caller needs is "run the pass here", not the three pieces
// it is assembled from.
func SweepAutoApply(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	return autoApplier{
		pool:  pool,
		svc:   approvalsServiceWithEffects(pool),
		users: identity.NewService(pool),
	}.Sweep(ctx)
}
