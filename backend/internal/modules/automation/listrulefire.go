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
// pauses the rule instead, as of the version its instance was loaded at.
func (e *WorkflowEngine) fireListRule(ctx context.Context, rule listRule, ev workflow.Event, version int64) error {
	firings, pause, err := rule.expand(ctx, e.resolver, ev)
	if err != nil {
		return err
	}
	if pause != nil {
		return e.pauses.pauseRule(ctx, ruleVersion{ID: ev.AutomationID, Version: version}, *pause, ev.Entity.ID)
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
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		// The owner can no longer find or read the list: what it holds is no
		// longer theirs to be told about.
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
	notifier PauseNotifier
	lists    Lists
}

// PauseNotifier writes a paused rule's notice on the pause's own
// transaction, so the two commit together or not at all.
type PauseNotifier interface {
	NotifyTx(ctx context.Context, tx pgx.Tx, recipient ids.UUID, subject, body string, target datasource.EntityRef, dedupe string) error
}

// NewRulePauser builds the pauser over the automation table, the notice
// transport its owners hear through, and the lists it re-reads before acting
// on a list's news.
func NewRulePauser(db *database.DB, notifier PauseNotifier, lists Lists) *RulePauser {
	return &RulePauser{db: db, notifier: notifier, lists: lists}
}

// listRuleKeys are the catalog keys whose params name lists.
var listRuleKeys = []string{listTaskName, listNotifyName, listShortlistName}

// PauseRulesOnArchivedList pauses the rules on a list its archive event
// names, unless the list is no longer archived: the event can arrive after
// the list was restored and its rules resumed, and that later decision stands.
func (p *RulePauser) PauseRulesOnArchivedList(ctx context.Context, listID ids.UUID) error {
	if p.lists == nil {
		return errors.New("automation: an archived list's rules cannot be re-checked with no lists seam wired")
	}
	list, err := p.lists.Find(ctx, listID)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
	case err != nil:
		return err
	case !list.Archived:
		return nil
	}
	return p.PauseRulesOnList(ctx, listID, PausedListArchived)
}

// PauseRulesOnList pauses every active list rule that watches the list or
// adds to it. Each rule pauses and is told in its own transaction, so one
// failure leaves the others paused and told.
func (p *RulePauser) PauseRulesOnList(ctx context.Context, listID ids.UUID, reason string) error {
	var candidates []ruleVersion
	err := p.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, version FROM automation
			WHERE enabled AND archived_at IS NULL AND key = ANY(@keys)
			  AND (params ->> 'list_id' = @list_id OR params ->> 'shortlist_id' = @list_id)`,
			pgx.StrictNamedArgs{"keys": listRuleKeys, "list_id": listID.String()})
		if err != nil {
			return err
		}
		candidates, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ruleVersion])
		return err
	})
	if err != nil {
		return err
	}
	var errs error
	for _, rule := range candidates {
		errs = errors.Join(errs, p.pauseRule(ctx, rule, rulePause{reason: reason}, listID))
	}
	return errs
}

// ruleVersion is a rule as a pause decision saw it.
type ruleVersion struct {
	ID      ids.UUID
	Version int64
}

// auditEnabled is the audit image key a rule's on/off state is recorded under.
const auditEnabled = "enabled"

// pausedRule is one rule a pause stopped.
type pausedRule struct {
	ID      ids.UUID
	Name    string
	Owner   *ids.UUID
	Version int64
}

// pauseRule stops one rule, audits it and writes its owner's notice, all in
// one transaction. It acts only on the version the decision was taken
// against: a rule edited, resumed or paused since then is left as it is.
func (p *RulePauser) pauseRule(ctx context.Context, seen ruleVersion, pause rulePause, listID ids.UUID) error {
	if p.notifier == nil {
		// A pause nobody hears about looks like a rule that stopped working.
		return ErrNoNotificationTransport
	}
	return p.db.Tx(ctx, func(tx pgx.Tx) error {
		var rule pausedRule
		err := tx.QueryRow(ctx, `UPDATE automation SET enabled = false, paused_reason = @reason,
				version = version + 1, updated_at = now()
			WHERE id = @id AND version = @version AND enabled AND archived_at IS NULL
			RETURNING id, name, owner_id, version`,
			pgx.StrictNamedArgs{"id": seen.ID, "version": seen.Version, "reason": pause.reason}).
			Scan(&rule.ID, &rule.Name, &rule.Owner, &rule.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "automation", rule.ID,
			map[string]any{auditEnabled: true}, map[string]any{auditEnabled: false, "paused_reason": pause.reason}); err != nil {
			return err
		}
		return p.tellOwner(ctx, tx, rule, pause, listID)
	})
}

// tellOwner writes a paused rule's owner the reason. The key carries the
// version the pause wrote, so a later pause of the same rule is news again.
func (p *RulePauser) tellOwner(ctx context.Context, tx pgx.Tx, rule pausedRule, pause rulePause, listID ids.UUID) error {
	if rule.Owner == nil {
		return nil
	}
	var target datasource.EntityRef
	if !listID.IsZero() {
		target = datasource.EntityRef{Type: rbacObjList, ID: listID}
	}
	return p.notifier.NotifyTx(ctx, tx, *rule.Owner, "Paused: "+rule.Name, pauseSentence(pause), target,
		fmt.Sprintf("automation_paused:%s:%d", rule.ID, rule.Version))
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
