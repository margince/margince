// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// A rule an agent set up stops when that agent's passport is revoked, because the
// user no longer lends it authority. An expired passport does not stop it. A user
// enabling or editing the rule makes it theirs. A seeded rule belongs to the
// product, so no setter ever stands on one.

const (
	keySetterPassport = "setter_passport_id"
	keyName           = "name"
	keyParams         = "params"
	keyStatus         = "status"
)

// livePassportOf holds the agent's passport for the rest of the transaction and
// answers its id, or the zero id for an agent with no passport. A revocation waits
// for this write, so the next firing reads the setter this leaves.
func livePassportOf(ctx context.Context, tx pgx.Tx, actor principal.Principal) (ids.UUID, error) {
	if actor.PassportID.IsZero() {
		return ids.Nil, nil
	}
	var live bool
	err := tx.QueryRow(ctx, `SELECT revoked_at IS NULL FROM passport WHERE id = $1 FOR SHARE`, actor.PassportID).Scan(&live)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !live) {
		return ids.Nil, fmt.Errorf("this agent's passport is no longer live: %w", apperrors.ErrPermissionDenied)
	}
	if err != nil {
		return ids.Nil, fmt.Errorf("automation: reading the agent's passport: %w", err)
	}
	return actor.PassportID, nil
}

// agentSetterOf is the setter a new rule names: the calling agent's passport, or
// the zero id for a user. A rule created from a seeded key is the agent's too.
func agentSetterOf(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	actor, isAgent := callingAgent(ctx)
	if !isAgent {
		return ids.Nil, nil
	}
	return livePassportOf(ctx, tx, actor)
}

// takesOverAgentsRule reports a user's update to a rule an agent set up, which
// clears the setter even when it changes nothing else.
func takesOverAgentsRule(ctx context.Context, before Automation) bool {
	actor, ok := principal.Actor(ctx)
	return ok && actor.Type == principal.PrincipalHuman && before.SetterPassportID != nil
}

// setterAfterUpdate is who stands behind a rule after an update. An agent enabling
// one becomes its setter, a user clears it, and an agent's other edits leave it.
func setterAfterUpdate(ctx context.Context, tx pgx.Tx, before Automation, in UpdateAutomationInput) (*ids.UUID, error) {
	setter := before.SetterPassportID
	actor, isAgent := callingAgent(ctx)
	switch {
	case isAgent && in.Enabled != nil && *in.Enabled:
		entry, known := CatalogEntryByKey(before.Key)
		if !known || seededRow(entry, before) {
			break
		}
		live, err := livePassportOf(ctx, tx, actor)
		if err != nil {
			return setter, err
		}
		setter = storekit.UUIDOrNil(live)
	case takesOverAgentsRule(ctx, before):
		setter = nil
	}
	return setter, nil
}

// revokedSetterVersion reads a rule's version when its setter's passport is revoked.
func (e *WorkflowEngine) revokedSetterVersion(ctx context.Context, id ids.UUID) (int64, bool, error) {
	var version int64
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT a.version FROM automation a JOIN passport p ON p.id = a.setter_passport_id
			 WHERE a.id = $1 AND p.revoked_at IS NOT NULL`, id).Scan(&version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("automation: reading the rule's setter: %w", err)
	}
	return version, true, nil
}

// stopIfSetterRevoked pauses a rule whose setter's passport was revoked and says
// to stop this firing. The pause acts on the version it read. So a user who took
// the rule over meanwhile keeps it, and the second read lets their firing run. An
// error also stops the firing: a rule that could not be paused still does not run.
func (e *WorkflowEngine) stopIfSetterRevoked(ctx context.Context, ev workflow.Event) (bool, error) {
	if ev.AutomationID.IsZero() {
		return false, nil
	}
	version, revoked, err := e.revokedSetterVersion(ctx, ev.AutomationID)
	if err != nil || !revoked {
		return err != nil, err
	}
	if e.pauses == nil {
		return true, ErrNoNotificationTransport
	}
	pause := rulePause{reason: PausedPassportRevoked}
	if err := e.pauses.pauseRule(ctx, ruleVersion{ID: ev.AutomationID, Version: version}, pause, ids.Nil); err != nil {
		return true, err
	}
	_, stillRevoked, err := e.revokedSetterVersion(ctx, ev.AutomationID)
	return err != nil || stillRevoked, err
}

// passedOverForRevokedSetter records the firing a revoked setter stopped. A clock
// trigger records nothing. Its key names an anchor, and claiming it now would keep
// the firing from running once a user resumes the rule.
func (e *WorkflowEngine) passedOverForRevokedSetter(ctx context.Context, h workflow.Handler, ev workflow.Event) error {
	if isClockTrigger(h) {
		return nil
	}
	return e.recordSkipReason(ctx, h, ev, "the agent that set this rule up had its passport revoked")
}
