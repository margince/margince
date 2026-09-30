// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// How one check of a Live List fires the rules that watch it. The check's
// event names the list; each rule reads, as its owner, the changes that check
// recorded under the list's current filter, and fires once per record its
// owner can see. A rule whose list stops being usable, or whose check moved
// more records than it acts on at once, pauses itself and tells its owner.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// burstCap is the most records one check may fire a rule for. Past it the
// rule fires for none of them: acting on the first hundred of a thousand
// would be an arbitrary hundred.
const burstCap = 100

// Why a rule paused itself (automation_paused_reason_check).
const (
	PausedListArchived    = "list_archived"
	PausedListInvalid     = "list_invalid"
	PausedListUnavailable = "list_unavailable"
	PausedBurst           = "burst"
)

// rulePause is a rule's decision to stop, and how many records one check
// moved when that is why.
type rulePause struct {
	reason string
	count  int
}

// fireListRule fans one check out into a firing per changed record, or
// pauses the rule instead.
func (e *WorkflowEngine) fireListRule(ctx context.Context, rule listRule, ev workflow.Event) error {
	firings, pause, err := rule.expand(ctx, e.resolver, ev)
	if err != nil {
		return err
	}
	if pause != nil {
		return e.pauses.pauseRule(ctx, ids.From[ids.AutomationKind](ev.AutomationID), *pause, ev.Entity.ID)
	}
	var firstErr error
	for _, firing := range firings {
		if err := e.runOne(ctx, rule, firing); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// expand reads what the check behind ev saw, as the rule's owner. A check of
// another list, the first check after the filter changed, and a check under a
// filter the list no longer has all fire nothing.
func (r listRule) expand(ctx context.Context, resolver authz.Resolver, ev workflow.Event) ([]workflow.Event, *rulePause, error) {
	if r.ex.Lists == nil {
		return nil, nil, errors.New("automation: a list rule fired with no lists seam wired")
	}
	rule, err := decodeListRuleParams(ev.Params)
	if err != nil || rule.ListID != ev.Entity.ID {
		return nil, nil, err
	}
	var check crmcontracts.PublicEventListEvaluated
	if err := json.Unmarshal(ev.Payload, &check); err != nil {
		return nil, nil, fmt.Errorf("automation: reading a list check: %w", err)
	}
	if check.FilterChanged {
		return nil, nil, nil
	}
	list, pause, err := r.watchedList(ctx, rule.ListID)
	if pause != nil || err != nil || list.Version != check.DefinitionVersion || ev.OwnerID.IsZero() {
		return nil, pause, err
	}
	owner, err := ownerContext(ctx, resolver, ev.WorkspaceID, ev.OwnerID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	changes, total, err := r.ex.Lists.ObservedChanges(owner, list.ID, check.DefinitionVersion, check.EvaluatedAt,
		actionsFor(rule.Direction), burstCap)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return nil, &rulePause{reason: PausedListUnavailable}, nil
	case err != nil:
		return nil, nil, err
	case total > burstCap:
		return nil, &rulePause{reason: PausedBurst, count: total}, nil
	}
	return firingsFor(ev, list, check.EvaluatedAt, changes)
}

// watchedList reads the watched list as the engine, and says whether its
// state pauses the rule.
func (r listRule) watchedList(ctx context.Context, id ids.UUID) (ListRef, *rulePause, error) {
	list, err := r.ex.Lists.Find(ctx, id)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return ListRef{}, &rulePause{reason: PausedListUnavailable}, nil
	case err != nil:
		return ListRef{}, nil, err
	case list.Archived:
		return list, &rulePause{reason: PausedListArchived}, nil
	case list.Invalid:
		return list, &rulePause{reason: PausedListInvalid}, nil
	}
	return list, nil, nil
}

// firingsFor is one event per changed record, each about the record.
func firingsFor(ev workflow.Event, list ListRef, at time.Time, changes []ListChange) ([]workflow.Event, *rulePause, error) {
	out := make([]workflow.Event, 0, len(changes))
	for _, change := range changes {
		payload, err := json.Marshal(listFiring{
			ListID: list.ID, ListName: list.Name, Action: change.Action, MemberEventID: change.EventID,
		})
		if err != nil {
			return nil, nil, err
		}
		firing := ev
		firing.Entity, firing.Payload, firing.OccurredAt = change.Record, payload, at
		out = append(out, firing)
	}
	return out, nil, nil
}

// RulePauser pauses list rules and tells each owner why. A paused rule stays
// paused until its owner resumes it: restoring or fixing the list does not.
type RulePauser struct {
	db       *database.DB
	notifier Notifier
}

// NewRulePauser builds the pauser over the automation table and the notice
// transport its owners hear through.
func NewRulePauser(db *database.DB, notifier Notifier) *RulePauser {
	return &RulePauser{db: db, notifier: notifier}
}

// listRuleKeys are the catalog keys whose params name lists.
var listRuleKeys = []string{listTaskName, listNotifyName, listShortlistName}

// PauseRulesOnList pauses every active list rule that watches the list or
// adds to it.
func (p *RulePauser) PauseRulesOnList(ctx context.Context, listID ids.UUID, reason string) error {
	return p.pause(ctx, `key = ANY(@keys) AND (params ->> 'list_id' = @list OR params ->> 'shortlist_id' = @list)`,
		pgx.StrictNamedArgs{"keys": listRuleKeys, "list": listID.String()}, rulePause{reason: reason}, listID)
}

func (p *RulePauser) pauseRule(ctx context.Context, id ids.AutomationID, pause rulePause, listID ids.UUID) error {
	return p.pause(ctx, `id = @id`, pgx.StrictNamedArgs{"id": id}, pause, listID)
}

// pausedRule is one rule a pause stopped.
type pausedRule struct {
	ID      ids.UUID
	Name    string
	Owner   *ids.UUID
	Version int64
}

// pause stops the enabled rules matching where, audits each, and then tells
// each owner. A rule already paused is left alone and nobody is told twice.
func (p *RulePauser) pause(ctx context.Context, where string, args pgx.StrictNamedArgs, pause rulePause, listID ids.UUID) error {
	args["reason"] = pause.reason
	var stopped []pausedRule
	err := p.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE automation SET enabled = false, paused_reason = @reason,
				version = version + 1, updated_at = now()
			WHERE enabled AND archived_at IS NULL AND `+where+`
			RETURNING id, name, owner_id, version`, args)
		if err != nil {
			return err
		}
		stopped, err = pgx.CollectRows(rows, pgx.RowToStructByPos[pausedRule])
		if err != nil {
			return err
		}
		if len(stopped) > 0 && p.notifier == nil {
			// A pause nobody hears about looks like a rule that stopped working.
			return ErrNoNotificationTransport
		}
		for _, rule := range stopped {
			if _, err := storekit.Audit(ctx, tx, "update", "automation", rule.ID,
				map[string]any{"enabled": true}, map[string]any{"enabled": false, "paused_reason": pause.reason}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, rule := range stopped {
		if err := p.tellOwner(ctx, rule, pause, listID); err != nil {
			return err
		}
	}
	return nil
}

// tellOwner sends a paused rule's owner the reason. The key carries the
// version the pause wrote, so a later pause of the same rule is news again.
func (p *RulePauser) tellOwner(ctx context.Context, rule pausedRule, pause rulePause, listID ids.UUID) error {
	if rule.Owner == nil {
		return nil
	}
	var target datasource.EntityRef
	if !listID.IsZero() {
		target = datasource.EntityRef{Type: "list", ID: listID}
	}
	return p.notifier.Notify(ctx, *rule.Owner, "Paused: "+rule.Name, pauseSentence(pause), target,
		fmt.Sprintf("automation_paused:%s:%d", rule.ID, rule.Version), nil)
}

// pauseSentence says why a rule stopped and what brings it back.
func pauseSentence(pause rulePause) string {
	switch pause.reason {
	case PausedListArchived:
		return "The list this rule watches or adds to was archived. Restoring the list does not resume the rule; resume it in Settings."
	case PausedListInvalid:
		return "The filter of the Live List this rule watches no longer works. Fixing the filter does not resume the rule; resume it in Settings."
	case PausedBurst:
		return fmt.Sprintf("One check of the list moved %d records, more than the %d this rule acts on at once, so it acted on none of them. Resume it in Settings when you are ready.",
			pause.count, burstCap)
	default:
		return "You can no longer find the list this rule watches. Resume it in Settings once you can."
	}
}
