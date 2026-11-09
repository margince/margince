// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AlreadyDecidedError answers 409 already_decided on every surface: it
// unwraps to ErrConflict for the status and carries the code through the
// shared taxonomy, so the REST and MCP doors cannot word it differently.
type AlreadyDecidedError struct{ Status string }

func (e *AlreadyDecidedError) Error() string { return "approval is already " + e.Status }

// MessageFault names the standing verdict so a caller can branch on the code.
func (e *AlreadyDecidedError) MessageFault() (code, message string) {
	return "already_decided", e.Error()
}

func (e *AlreadyDecidedError) Unwrap() error { return apperrors.ErrConflict }

// InvalidEditError maps to 422: an edited payload that is not a JSON
// object cannot be canonicalized, so it cannot become an authority.
type InvalidEditError struct{ Cause error }

func (e *InvalidEditError) Error() string { return "edited_payload: " + e.Cause.Error() }
func (e *InvalidEditError) Unwrap() error { return e.Cause }

// Decide approves or rejects one pending approval. Both verdicts demand
// the same authority the inbox demands for visibility: the RBAC the
// staged action itself requires plus row-scope visibility of the target —
// a user cannot green-light an effect they could not perform, and a
// rejection is a decision too, not a free action anyone holding a leaked
// UUID may take. An undecidable approval reads as absent, exactly like
// Get, so Decide never becomes the lookup oracle the inbox filter closed.
func (s *Service) Decide(ctx context.Context, id ids.ApprovalID, approve bool, reason *string) (row, error) {
	return s.recordDecision(ctx, id, approve, reason, nil, decidedByContact)
}

// DecideEdited is the ADR-0036 §4 modify-then-approve arm: the human's
// edited payload replaces the staged change under a freshly computed
// diff_hash, and the decision's audit row carries BOTH the original
// agent proposal and the human's version. An agent redemption only fits
// the new hash if it re-presents the edited call — which the gate
// re-tiers and re-admits like any other call. The old hash, and any
// token bound to it, no longer opens anything.
//
// What an edit may touch is bounded by TWO assertions, because a record is
// named two different ways depending on how the call was staged: the staged
// payload's entity references are pinned (assertSameEntityRefs) for a value
// carried as a field, and a REST staging's operation/path are pinned
// separately (assertSameCallIdentity) for a record named inside the request
// path instead, which entityRefs cannot see. Together they mean the edit
// corrects the action but cannot re-aim it at another record or another
// call. That bound is the admission control on this arm — a server-proposed
// effect resolves its target from the payload and may run under a system
// principal, so the stores' own RBAC and row-scope gates cannot be relied on
// to re-check what the human wrote.
func (s *Service) DecideEdited(ctx context.Context, id ids.ApprovalID, edited json.RawMessage) (row, error) {
	return s.DecideEditedFor(ctx, id, edited, nil)
}

// DecideEditedFor is DecideEdited with the approver's reason, which is recorded
// with the decision as a rejection's is.
func (s *Service) DecideEditedFor(ctx context.Context, id ids.ApprovalID, edited json.RawMessage, reason *string) (row, error) {
	if len(edited) == 0 {
		return row{}, &InvalidEditError{Cause: errors.New("empty payload")}
	}
	return s.recordDecision(ctx, id, true, reason, edited, decidedByContact)
}

// decider says whether a contact answered this or the product applied it under
// a rep's standing policy. It is a parameter rather than something read off the
// context because it is a claim the receipt makes to a reader — "nobody was
// asked" — and a claim that travels invisibly is one a future call site sets
// wrongly without noticing.
type decider bool

const (
	decidedByContact decider = false
	decidedBySystem  decider = true
)

func (s *Service) recordDecision(ctx context.Context, id ids.ApprovalID, approve bool, reason *string, edited json.RawMessage, by decider) (row, error) {
	if err := actingForAHuman(ctx); err != nil {
		return row{}, err
	}
	p, _ := principal.Actor(ctx)

	if err := s.runPrecheck(ctx, id, approve, edited); err != nil {
		return row{}, err
	}
	footprint, err := s.footprintToStore(ctx, id, edited)
	if err != nil {
		return row{}, err
	}
	var a row
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		a, err = s.decideInTx(ctx, tx, p, id, approve, reason, edited, footprint, by)
		if err != nil {
			return err
		}
		// A declined effect runs HERE, inside the decision's transaction, so a
		// rejection and the work it releases commit together. Run afterwards it
		// would leave the card rejected, the retry refused as already-decided,
		// and the subject it was about still waiting — the failure this whole
		// card exists to prevent.
		//
		// The approve side cannot join this transaction: its effects redeem the
		// approval and write through other modules' stores, several of which
		// open their own. It carries its own atomicity through RedeemAndApply.
		if decline, ok := s.declines[a.Kind]; ok && !approve && serverProposed(a) {
			if err := decline(ctx, tx, id, a.ProposedChange); err != nil {
				return fmt.Errorf("executing the %s decline: %w", a.Kind, err)
			}
		}
		return nil
	})
	if err != nil {
		if s.effectIsStillOwed(a, approve, err) {
			// The decision stands and its work never ran. Re-drive rather than
			// refuse: see effectIsStillOwed for why this is the only answer
			// that is not a dead end.
			return a, s.runDecisionEffect(ctx, id, a, approve)
		}
		return row{}, err
	}
	return a, s.runDecisionEffect(ctx, id, a, approve)
}

// Preflight refuses unsafe context before a verdict can strand an unapplied effect.
func (s *Service) runPrecheck(ctx context.Context, id ids.ApprovalID, approve bool, edited json.RawMessage) error {
	if !approve {
		return nil
	}
	a, err := s.Get(ctx, id)
	if err != nil {
		if _, automatic := policyActorFor(ctx, id); automatic && errors.Is(err, apperrors.ErrNotFound) {
			return ErrPolicyWithdrawn
		}
		return err
	}
	// An approved but unapplied effect retries through its execution guards.
	if st := a.effectiveStatus(s.now()); st != "pending" {
		if st == approvalStatusApproved && s.owesItsEffect(a) {
			return nil
		}
		return &AlreadyDecidedError{Status: st}
	}
	return s.precheckMember(ctx, a, edited)
}

// countIfAContactDecided records the track record, and records nothing for an
// automatic apply or for a release an agent carried, which the human never saw.
//
// The counters are one contact's experience of one kind, and the clean-approval
// column is the one a promotion offer is read from — so a pass running every
// minute under a policy the rep already set would manufacture unbounded
// evidence that they keep agreeing, about proposals they never saw. The ladder
// is climbed by decisions, not by the automation a previous rung enabled.
func countIfAContactDecided(
	ctx context.Context, tx pgx.Tx, p principal.Principal, kind string,
	approve bool, edited json.RawMessage, by decider,
) error {
	if by != decidedByContact || p.Type == principal.PrincipalAgent {
		return nil
	}
	return countDecisionTx(ctx, tx, p.UserID, kind, decisionOutcomeOf(approve, edited))
}

// landEditedPayload writes a modify-then-approve edit, and refuses the one kind
// that may not carry one.
//
// A step-up carries nothing a human should rewrite. Its payload IS the question
// they were shown — which counter, which window, how much was spent — so an
// edit releases something other than what was asked. The meter refuses the
// impossible ones (a hard-stop counter, a window that has not started), but a
// read step-up edited into a write release is neither impossible nor what
// anyone saw.
//
// Refused inside the transaction, before the edit lands and before the status
// is written: there is no correct edit here, so there is nothing to salvage and
// nothing should be recorded as decided.
func landEditedPayload(
	ctx context.Context, tx pgx.Tx, id ids.ApprovalID, edited json.RawMessage,
	a row, auditEvidence map[string]any, decidedPayload *crmcontracts.PublicEventApprovalDecided,
) error {
	if edited == nil {
		return nil
	}
	if a.Kind == KindVolumeRelease {
		return &InvalidEditError{Cause: errors.New("a step-up is answered yes or no, not edited")}
	}
	if err := assertEditableMembers(a.Kind, a.ProposedChange, edited); err != nil {
		return err
	}
	return applyEditedPayload(ctx, tx, id, edited, a, auditEvidence, decidedPayload)
}

// readToDecide hides rows the decider cannot open.
func readToDecide(ctx context.Context, tx pgx.Tx, p principal.Principal, id ids.ApprovalID, by decider) (row, error) {
	a, err := get(ctx, tx, id)
	if err != nil {
		return row{}, err
	}
	visible, err := decidable(ctx, tx, p, a)
	switch {
	case err != nil:
		return row{}, err
	case !visible && by == decidedBySystem:
		return row{}, ErrPolicyWithdrawn
	case !visible:
		return row{}, auth.HideRow(ctx, "approval")
	}
	return a, nil
}

// pendingToDecide reads the locked approval as one the caller may decide now.
// A refusal for a decided row carries that row; recordDecision reads it.
func (s *Service) pendingToDecide(ctx context.Context, tx pgx.Tx, p principal.Principal, id ids.ApprovalID, approve bool, edited json.RawMessage, by decider) (row, error) {
	a, err := readToDecide(ctx, tx, p, id, by)
	if err != nil {
		return row{}, err
	}
	if approve && agentCallNeedsRecheck(a) {
		if _, err := s.agentCallAuthority(ctx, tx, a); err != nil {
			return row{}, err
		}
	}

	// After visibility, so a caller who cannot see this row is told it is absent
	// rather than that their credential is short a cap — the existence-hiding
	// this module keeps everywhere. Before the status check, because what a
	// credential may release is a question about the credential and not about
	// how far this particular proposal has got.
	if err := agentMayDecide(p, a, approve, s.ownReleaseFor(ctx, tx, a)); err != nil {
		return row{}, err
	}
	// An agent never edits what it releases, whoever staged it: what a credential
	// may release is judged on the staged payload, so an edit would release
	// something that was never classified. The contact edits in the CRM.
	if p.Type == principal.PrincipalAgent && edited != nil {
		return row{}, fmt.Errorf("an agent never edits what it releases; the contact edits and "+
			"releases it in the CRM: %w", apperrors.ErrPermissionDenied)
	}
	if st := a.effectiveStatus(s.now()); st != "pending" {
		// The ROW travels with the refusal. recordDecision has to tell an
		// approved row whose effect never ran from one that is genuinely
		// finished, and re-reading it there would be a second look at a row
		// this transaction is holding — a different answer is possible, and it
		// is the answer that decides whether a human's yes is honoured or
		// refused.
		return a, &AlreadyDecidedError{Status: st}
	}
	// A retired kind is governed only so its decided cards stay readable;
	// nothing applies it, so a yes would be recorded and do nothing.
	if approve && retiredKinds[a.Kind] {
		return row{}, fmt.Errorf("crmapprovals: %s is retired and can no longer be applied: %w",
			a.Kind, apperrors.ErrConflict)
	}
	return a, nil
}

func (s *Service) decideInTx(ctx context.Context, tx pgx.Tx, p principal.Principal, id ids.ApprovalID, approve bool, reason *string, edited, footprint json.RawMessage, by decider) (row, error) {
	// An automatic apply binds the owner, reads the mode and holds the target
	// before the row lock (bindUnderPolicy has the lock order).
	if by == decidedBySystem {
		var err error
		if ctx, p, err = bindUnderPolicy(ctx, tx, id); err != nil {
			return row{}, err
		}
	}
	// The row lock makes the pending pre-read and the status write below
	// one race-free unit: two concurrent decisions cannot both pass the
	// pending guard. Taken raw: the approval table has no archived_at,
	// so storekit.LockRow's live filter does not apply here.
	var locked ids.ApprovalID
	if err := tx.QueryRow(ctx, `SELECT id FROM approval WHERE id = $1 FOR UPDATE`, id).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row{}, apperrors.ErrNotFound
		}
		return row{}, err
	}
	a, err := s.pendingToDecide(ctx, tx, p, id, approve, edited, by)
	if err != nil {
		return a, err
	}

	status, action, verdict := approvalStatusRejected, "reject", approvalStatusRejected
	if approve {
		status, action, verdict = approvalStatusApproved, "approve", approvalStatusApproved
	}
	auditEvidence := map[string]any{
		approvalKeyKind: a.Kind, "verdict": verdict, approvalKeyReason: reason,
	}
	// decided_by is a pointer because ONE verdict has no decider: the expiry
	// sweep's. A human decision always names theirs, so it is always set here.
	decidedBy := openapi_types.UUID(p.UserID)
	// decided_by_system travels on the event as well as into the column. A
	// consumer measuring whether CONTACTS agree with what the product proposes
	// has to exclude the product's own applies from its denominator, and
	// without this field it cannot tell one from a human's clean approval —
	// the autopilot would be counted as agreeing with itself.
	bySystem := bool(by)
	decidedPayload := crmcontracts.PublicEventApprovalDecided{
		Kind:            a.Kind,
		Verdict:         crmcontracts.PublicEventApprovalDecidedVerdict(verdict),
		DecidedBy:       &decidedBy,
		DecidedBySystem: &bySystem,
	}
	if err := landEditedPayload(ctx, tx, id, edited, a, auditEvidence, &decidedPayload); err != nil {
		return row{}, err
	}
	if approve {
		if err := s.pinDecidedContext(ctx, tx, id, edited != nil); err != nil {
			return row{}, err
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE approval SET status = $2, decided_by = $3, decided_at = now(), decision_reason = $4,
		        decided_by_system = $5, plan_footprint = $6
		 WHERE id = $1`,
		id, status, p.UserID, reason, bool(by), footprint); err != nil {
		return row{}, err
	}
	// The track record the autonomy ladder is earned on, counted in the same
	// transaction as the decision it counts. A counter that could outlive a
	// rolled-back approval would offer a rep autonomy on evidence of a decision
	// they never made.
	if err := countIfAContactDecided(ctx, tx, p, a.Kind, approve, edited, by); err != nil {
		return row{}, err
	}
	// An approval's whole content is a state transition, so the images are the
	// transition — not decoration. Without them the audit row says a decision
	// happened and not what it decided, which is the one thing a reader of an
	// approval's history is looking for.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, action, entityApproval, id.UUID,
		map[string]any{approvalKeyStatus: StatusPending},
		map[string]any{approvalKeyStatus: statusAfter(approve)},
		auditEvidence)
	if err != nil {
		return row{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, decidedPayload); err != nil {
		return row{}, err
	}
	if err := s.emitKindDecided(ctx, tx, p, auditID, id.UUID, a.Kind, approve); err != nil {
		return row{}, err
	}
	return get(ctx, tx, id)
}

// emitKindDecided fires the kind-specific echo of the verdict (e.g. a
// coldstart read-back's approved/rejected event) on the same audit row,
// when the staging's kind registers one.
func (s *Service) emitKindDecided(ctx context.Context, tx pgx.Tx, p principal.Principal, auditID, id ids.UUID, kind string, approve bool) error {
	echo, ok := kindDecidedEvents[kind]
	if !ok {
		return nil
	}
	build := echo.rejected
	if approve {
		build = echo.approved
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, build(openapi_types.UUID(id), openapi_types.UUID(p.UserID)))
}

// ApplyUnderPolicy approves a proposal because the rep it belongs to has put
// this kind on automatic, rather than because anybody was asked.
//
// It takes the decision path a contact takes, so the effect, audit row, event
// and track record are written as they are for a human. decided_by_system tells
// the receipt nobody was asked. actAs binds an agent with the owner's own
// grants, so the apply is bounded by what that rep could do by hand. The
// decision and the effect's redemption each bind it again in their own
// transaction and ask the owner's mode there. A withdrawn policy answers
// ErrPolicyWithdrawn, and that stage writes nothing.
func (s *Service) ApplyUnderPolicy(ctx context.Context, id ids.ApprovalID, actAs PolicyActor) (row, error) {
	return s.recordDecision(withPolicyActor(ctx, id, actAs), id, true, nil, nil, decidedBySystem)
}

// statusAfter is the state a decision leaves the approval in, which is what the
// audit row's after image records.
func statusAfter(approve bool) string {
	if approve {
		return StatusApproved
	}
	return StatusRejected
}
