// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The automation_effect_claim writer behind the EffectClaims seam
// (seams.go): claim-first, the same discipline claimRun (engine_run.go)
// applies to the per-instance run row.
//
// The claim is TWO-PHASE, which is what separates it from claimRun. A claim
// commits in its own transaction and the domain create runs after it, so a
// worker dying between the two used to leave a claim with no record behind
// it: the next firing folded as "deduplicated" against a write that never
// landed, and the task was lost with nothing to repair it. applied_at is how a
// sibling tells a claim whose create landed from one whose holder died.
//
// The age test runs in SQL against now(), so the lease is measured by the same
// clock that stamped created_at. A Go-side comparison would introduce a second
// clock, and a host trailing the database by more than the lease would hand two
// firings the same claim.
//
// WHY NOT approvals.RedeemAndApply's shape, which answers the same question —
// consume a token and write the thing it guards, atomically. That one owns its
// transaction and hands the tx to the caller's apply, and it can, because the
// redemption and the write are both inside approvals' reach. Here the write
// leaves the module: it goes through datasource.SystemOfRecordProvider.Create,
// a shared port that exposes no transaction, implemented by every provider.
// Threading one through to match approvals would change that port for all of
// them to fix a window in one caller.
//
// So the two disciplines stay separate on purpose. They differ in what they can
// reach, not in what they want, and either would be wrong in the other's place:
// RedeemAndApply across a port it does not own, or a lease where one
// transaction would do.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// EffectClaimStore is the module-owned implementation of EffectClaims.
type EffectClaimStore struct {
	db *database.DB
}

// NewEffectClaims builds the claim store compose wires into Executors.
func NewEffectClaims(db *database.DB) *EffectClaimStore {
	return &EffectClaimStore{db: db}
}

var _ EffectClaims = (*EffectClaimStore)(nil)

// claimLease is how long a claim may sit unapplied before a sibling firing may
// take it over.
//
// Long enough that a create still running is never stolen — the window it
// covers is one domain write, which is seconds — and short enough that a task
// stranded by a crashed worker comes back on the next redelivery rather than
// never. Fifteen minutes is far outside the first and well inside the second.
//
// Taking one over too early is the dangerous direction, because it is the
// double-write the claim exists to stop; waiting too long only delays a repair.
// So the lease is generous on purpose.
const claimLease = "15 minutes"

// Claim takes the (handler, occurrence, fingerprint) row for this firing.
//
// False means somebody else holds it and this create must fold. True means this
// firing owns the claim and MUST call Confirm once its create has landed — an
// unconfirmed claim is what a later sibling reclaims.
//
// One statement, three outcomes. The insert wins when nothing holds the claim.
// The conflicting update wins when a previous holder left it unapplied for
// longer than the lease, which is the crash this exists for. Neither fires when
// the claim is applied, or unapplied and still inside its lease — an applied
// claim guards a record that really is there, and a fresh unapplied one belongs
// to a firing still working.
func (s *EffectClaimStore) Claim(ctx context.Context, handler, occurrenceKey, fingerprint string) (bool, error) {
	claimed := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO automation_effect_claim (handler, occurrence_key, effect_fingerprint)
			VALUES ($1, $2, $3)
			ON CONFLICT (handler, occurrence_key, effect_fingerprint) DO UPDATE
				SET created_at = now()
				WHERE automation_effect_claim.applied_at IS NULL
				  AND automation_effect_claim.created_at < now() - $4::interval`,
			handler, occurrenceKey, fingerprint, claimLease)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() > 0
		return nil
	})
	return claimed, err
}

// Confirm records that the create this claim guards has landed, so no later
// firing reclaims it.
//
// Called only after a successful create. A claim left unconfirmed is not a
// failure to report — it is exactly the state a crash leaves, and the lease is
// what collects it — so the caller treats a Confirm error as it treats the
// create's own: the record exists either way, and the worst case is one
// duplicate after the lease rather than a task lost forever.
func (s *EffectClaimStore) Confirm(ctx context.Context, handler, occurrenceKey, fingerprint string) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE automation_effect_claim SET applied_at = now()
			WHERE handler = $1 AND occurrence_key = $2 AND effect_fingerprint = $3
			  AND applied_at IS NULL`,
			handler, occurrenceKey, fingerprint)
		return err
	})
}

// applyCreate writes one record, claim-first: an engine-stamped effect
// (eff.Handler set — engine_run.go stamps it before Apply) takes the
// effect-level claim so the IDENTICAL create from a sibling instance's
// firing folds instead of writing a second copy. A lost claim skips the
// write and returns the action with Deduplicated set — the run row then
// says the create was folded rather than silently claiming a write.
// An effect applied outside the engine (Handler empty) has no sibling
// firings to collide with and applies unclaimed, as it always did.
func applyCreate(ctx context.Context, ex Executors, eff workflow.Effect, action workflow.Action) (_ workflow.Action, _ *workflow.StagedApprovalError, createErr error) {
	if eff.Handler != "" {
		if ex.Claims == nil {
			return action, nil, ErrNoEffectClaims
		}
		fingerprint, err := effectFingerprint(action)
		if err != nil {
			return action, nil, err
		}
		claimed, err := ex.Claims.Claim(ctx, eff.Handler, eff.OccurrenceKey, fingerprint)
		if err != nil {
			return action, nil, err
		}
		if !claimed {
			return markDeduplicated(action), nil, nil
		}
		// Confirmed AFTER the create below, never before: the claim's whole
		// job is to say a record exists, and saying so first is what made a
		// crash here lose the task.
		defer func() {
			if createErr != nil {
				// The create failed, so there is no record for the claim to
				// guard. Leaving it unconfirmed is deliberate — the lease
				// collects it and a redelivery applies it — where confirming
				// would deduplicate every future firing against nothing.
				return
			}
			if confirmErr := ex.Claims.Confirm(ctx, eff.Handler, eff.OccurrenceKey, fingerprint); confirmErr != nil {
				// Not promoted to the caller's error: the record LANDED, and
				// failing the firing would re-run a create that already
				// succeeded. The unconfirmed claim costs at most one duplicate
				// after the lease, which is the smaller of the two wrongs.
				slog.WarnContext(ctx, "automation: an applied effect claim was not confirmed",
					"handler", eff.Handler, "error", confirmErr)
			}
		}()
	}
	entity := action.Target.Type
	if action.Kind == workflow.ActionCreateTask {
		entity = datasource.EntityActivity
	}
	_, createErr = ex.Provider.Create(ctx, datasource.CreateInput{
		EntityType: entity,
		Fields:     action.Args,
		Source:     systemSource,
	})
	return action, nil, createErr
}

// effectFingerprint is the claim's identity for one create action: kind,
// target and canonicalized args together. Two instances planning the same
// words onto the same record collapse; a different parameterization (a
// different due date) fingerprints apart and both apply, which is the
// correct reading of two genuinely different instances.
func effectFingerprint(action workflow.Action) (string, error) {
	raw := action.Args
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	_, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return "", fmt.Errorf("automation: fingerprinting a %s effect: %w", action.Kind, err)
	}
	return string(action.Kind) + "|" + string(action.Target.Type) + "|" + action.Target.ID.String() + "|" + hash, nil
}

// markDeduplicated flags a create the claim folded, so the instance's run
// row records that its planned write was performed by a sibling firing —
// a typed field rather than a value smuggled into Args, so every reader
// of the trace (runActionKinds included) can say so instead of reporting
// a write that never happened.
func markDeduplicated(action workflow.Action) workflow.Action {
	action.Deduplicated = true
	return action
}
