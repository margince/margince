// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The automation_effect_claim writer behind the EffectClaims seam
// (seams.go): claim-first with ON CONFLICT DO NOTHING, the same discipline
// claimRun (engine_run.go) applies to the per-instance run row. The crash
// window is the same one claimRun already accepts: a claim committed with
// the create then lost means a redelivery skips a create that never landed
// — rare, visible on the run row, and preferred over the double-write the
// claim exists to stop.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// Claim inserts the (handler, trigger event, fingerprint) row; false means
// another firing already holds it and this create must fold.
func (s *EffectClaimStore) Claim(ctx context.Context, handler string, triggerEvent ids.UUID, fingerprint string) (bool, error) {
	claimed := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO automation_effect_claim (handler, event_id, effect_fingerprint)
			VALUES ($1, $2, $3)
			ON CONFLICT (handler, event_id, effect_fingerprint) DO NOTHING`,
			handler, triggerEvent, fingerprint)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() > 0
		return nil
	})
	return claimed, err
}

// applyCreate writes one record, claim-first: an engine-stamped effect
// (eff.Handler set — engine_run.go stamps it before Apply) takes the
// effect-level claim so the IDENTICAL create from a sibling instance's
// firing folds instead of writing a second copy. A lost claim skips the
// write and returns the action annotated deduplicated:true — the run row
// then says the create was folded rather than silently claiming a write.
// An effect applied outside the engine (Handler empty) has no sibling
// firings to collide with and applies unclaimed, as it always did.
func applyCreate(ctx context.Context, ex Executors, eff workflow.Effect, action workflow.Action) (workflow.Action, *workflow.StagedApprovalError, error) {
	if eff.Handler != "" {
		if ex.Claims == nil {
			return action, nil, ErrNoEffectClaims
		}
		fingerprint, err := effectFingerprint(action)
		if err != nil {
			return action, nil, err
		}
		claimed, err := ex.Claims.Claim(ctx, eff.Handler, eff.TriggerEventID, fingerprint)
		if err != nil {
			return action, nil, err
		}
		if !claimed {
			folded, err := markDeduplicated(action)
			return folded, nil, err
		}
	}
	entity := action.Target.Type
	if action.Kind == workflow.ActionCreateTask {
		entity = datasource.EntityActivity
	}
	_, err := ex.Provider.Create(ctx, datasource.CreateInput{
		EntityType: entity,
		Fields:     action.Args,
		Source:     systemSource,
	})
	return action, nil, err
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

// markDeduplicated annotates a create the claim folded, so the instance's
// run row records that its planned write was applied by a sibling firing.
func markDeduplicated(action workflow.Action) (workflow.Action, error) {
	raw := action.Args
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return action, fmt.Errorf("automation: annotating a deduplicated %s action: %w", action.Kind, err)
	}
	args["deduplicated"] = true
	annotated, err := json.Marshal(args)
	if err != nil {
		return action, fmt.Errorf("automation: encoding a deduplicated %s action: %w", action.Kind, err)
	}
	action.Args = annotated
	return action, nil
}
