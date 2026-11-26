// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// A rule an agent set up stops when the agent no longer stands behind it, because
// the user no longer lends it authority. workforce.LiveSetterSQL decides, at set-up
// and at every firing. A user enabling or editing the rule makes it theirs. A seeded
// rule belongs to the product, so no setter stands on one. The exception is a draft
// starter an agent enabled, which spends a model call for as long as it runs.

const (
	keySetterPassport = "setter_passport_id"
	keyName           = "name"
	keyParams         = "params"
	keyStatus         = "status"
	keyCatalogKey     = "key"
)

// SQL argument names that several statements share.
const (
	argOwner   = "owner"
	argHandler = "handler"
	argKeys    = "keys"
	argRunKey  = "run_key"
)

// livePassportOf holds the agent's passport for the rest of the transaction and
// answers its id, or the zero id for an agent with no passport. A revocation waits
// for this write, so the next firing reads the setter this leaves. An admin disabling
// the client does not wait, and the next firing reads that too before anything runs.
func livePassportOf(ctx context.Context, tx pgx.Tx, actor principal.Principal) (ids.UUID, error) {
	if actor.PassportID.IsZero() {
		return ids.Nil, nil
	}
	var live bool
	err := tx.QueryRow(ctx, `SELECT `+workforce.LiveSetterSQL("now()")+` FROM passport p`+workforce.LiveSetterJoins+`
		WHERE p.id = $1 FOR SHARE OF p`, actor.PassportID).Scan(&live)
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

// setterAfterUpdate is who stands behind a rule after an update. An agent that turns
// a paused rule on becomes its setter, and a user clears it. An agent's other edits
// leave it, so naming a running rule's status claims nothing.
func setterAfterUpdate(ctx context.Context, tx pgx.Tx, before Automation, in UpdateAutomationInput) (*ids.UUID, error) {
	setter := before.SetterPassportID
	actor, isAgent := callingAgent(ctx)
	switch {
	case isAgent && in.Enabled != nil && *in.Enabled && !before.Enabled:
		entry, known := CatalogEntryByKey(before.Key)
		if !known || (seededRow(entry, before) && entry.Action != string(ActionTypeDraftEmail)) {
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

// How a setter stopped standing behind its rule, for the audit row of the pause.
const (
	setterRevoked = "revoked"
	setterExpired = "expired"
)

// lapsedSetter reads a rule's version and how its setter lapsed, when it has. The
// clock is the database's unless a test fixes the engine's.
func (e *WorkflowEngine) lapsedSetter(ctx context.Context, id ids.UUID) (version int64, how string, lapsed bool, err error) {
	var at *time.Time
	if e.clock != nil {
		fixed := e.clock()
		at = &fixed
	}
	err = e.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT a.version, CASE WHEN `+workforce.UnrevokedSetterSQL+` THEN $3 ELSE $4 END
			  FROM automation a JOIN passport p ON p.id = a.setter_passport_id`+workforce.LiveSetterJoins+`
			 WHERE a.id = $1 AND NOT `+workforce.LiveSetterSQL("coalesce($2::timestamptz, now())"),
			id, at, setterExpired, setterRevoked).Scan(&version, &how)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("automation: reading the rule's setter: %w", err)
	}
	return version, how, true, nil
}

// stopIfSetterLapsed pauses a rule whose setter no longer stands behind it and says
// to stop this firing. The pause acts on the version it read. So a user who took
// the rule over meanwhile keeps it, and the second read lets their firing run. An
// error also stops the firing: a rule that could not be paused still does not run.
func (e *WorkflowEngine) stopIfSetterLapsed(ctx context.Context, ev workflow.Event) (bool, error) {
	if ev.AutomationID.IsZero() {
		return false, nil
	}
	version, how, lapsed, err := e.lapsedSetter(ctx, ev.AutomationID)
	if err != nil || !lapsed {
		return err != nil, err
	}
	if e.pauses == nil {
		return true, ErrNoNotificationTransport
	}
	pause := rulePause{reason: PausedPassportRevoked, setter: how}
	if err := e.pauses.pauseRule(ctx, ruleVersion{ID: ev.AutomationID, Version: version}, pause, ids.Nil); err != nil {
		return true, err
	}
	_, _, stillLapsed, err := e.lapsedSetter(ctx, ev.AutomationID)
	return err != nil || stillLapsed, err
}

// passedOverForLapsedSetter records the firing a lapsed setter stopped. A clock
// trigger records nothing. Its key names an anchor, and claiming it now would keep
// the firing from running once a user resumes the rule.
func (e *WorkflowEngine) passedOverForLapsedSetter(ctx context.Context, h workflow.Handler, ev workflow.Event) error {
	if isClockTrigger(h) {
		return nil
	}
	return e.recordSkipReason(ctx, h, ev, "the agent that set this rule up no longer has a live passport or connection")
}
