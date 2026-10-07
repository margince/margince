// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package automation

// A list rule pausing itself: the pause, its audit row and its owner's notice
// commit together, one rule at a time, and only against the version of the
// rule the decision was taken on.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// pauseNotices records who a pause told, and refuses to tell one recipient.
type pauseNotices struct {
	told    []ids.UUID
	refuses ids.UUID
}

func (n *pauseNotices) NotifyTx(_ context.Context, _ pgx.Tx, recipient ids.UUID, _, _ string, _ datasource.EntityRef, _ string) error {
	if recipient == n.refuses {
		return errors.New("the notice could not be written")
	}
	n.told = append(n.told, recipient)
	return nil
}

func (fx *autoFixture) listRule(t *testing.T, owner, list ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	fx.exec(t, `
		INSERT INTO automation (id, key, name, trigger, action, params, owner_id, enabled, tier)
		VALUES ($1, $2, $2, '{"event_type":"list.evaluated"}', '{"kind":"notify"}', jsonb_build_object('list_id', $3::text), $4, true, 'auto_execute')`,
		id, listNotifyName, list.String(), owner)
	return id
}

func (fx *autoFixture) pauser(notices *pauseNotices) (*RulePauser, context.Context) {
	db := database.BindTo(fx.pool, ids.From[ids.WorkspaceKind](fx.ws))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), fx.ws), "system")
	return NewRulePauser(db, notices, nil), ctx
}

func TestAPauseDecidedOnAnOlderVersionLeavesTheRuleAlone(t *testing.T) {
	fx := setupAutomationDB(t)
	list := ids.NewV7()
	rule := fx.listRule(t, fx.rep1, list)
	notices := &pauseNotices{}
	pauser, ctx := fx.pauser(notices)
	fx.exec(t, `UPDATE automation SET name = 'Renamed', version = version + 1 WHERE id = $1`, rule)

	if err := pauser.pauseRule(ctx, ruleVersion{ID: rule, Version: 1}, rulePause{reason: PausedBurst, count: 101}, list); err != nil {
		t.Fatal(err)
	}
	if n := fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND enabled`, rule); n != 1 || len(notices.told) != 0 {
		t.Fatalf("a pause decided on version 1 stopped the rule at version 2 (enabled=%d, told %v)", n, notices.told)
	}
	if err := pauser.pauseRule(ctx, ruleVersion{ID: rule, Version: 2}, rulePause{reason: PausedBurst, count: 101}, list); err != nil {
		t.Fatal(err)
	}
	if n := fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND NOT enabled AND paused_reason = 'burst'`, rule); n != 1 || len(notices.told) != 1 {
		t.Fatalf("a pause on the current version did not stop the rule and tell its owner (paused=%d, told %v)", n, notices.told)
	}
}

func TestEachRuleIsPausedAndToldInItsOwnTransaction(t *testing.T) {
	fx := setupAutomationDB(t)
	list := ids.NewV7()
	untold := fx.listRule(t, fx.rep1, list)
	told := fx.listRule(t, fx.rep2, list)
	notices := &pauseNotices{refuses: fx.rep1}
	pauser, ctx := fx.pauser(notices)

	if err := pauser.PauseRulesOnList(ctx, list, PausedListInvalid); err == nil {
		t.Fatal("a notice that could not be written was not reported")
	}
	if n := fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND enabled AND paused_reason IS NULL`, untold); n != 1 {
		t.Fatal("a rule whose owner could not be told was paused anyway")
	}
	if n := fx.count(t, `SELECT count(*) FROM automation WHERE id = $1 AND NOT enabled`, told); n != 1 || len(notices.told) != 1 || notices.told[0] != fx.rep2 {
		t.Fatalf("one owner's failed notice stopped the other rule's pause (paused=%d, told %v)", n, notices.told)
	}
}
