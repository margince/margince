// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// Which active rules depend on a list, for the list's own page: a steward
// about to archive a list or change its filter is shown the rules it moves.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// How a rule uses a list.
const (
	RuleWatchesList = "watches"
	RuleWritesList  = "writes"
)

// ListRuleUse is one active rule that watches or writes a list. ID and Name
// are empty for a reader who may not read automations: they learn a rule
// depends on the list, and nothing about the rule.
type ListRuleUse struct {
	ID        ids.UUID
	Name      string
	Role      string
	CreatedAt time.Time
}

// RulesOnList answers the active rules that watch or write a list the caller
// can find, oldest first.
func (s *AutomationStore) RulesOnList(ctx context.Context, listID ids.UUID) ([]ListRuleUse, error) {
	named := auth.Require(ctx, "automation", principal.ActionRead) == nil
	var out []ListRuleUse
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// A list the caller cannot find has no rules to tell them about.
		if err := auth.EnsureVisible(ctx, tx, rbacObjList, listID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, name, CASE WHEN params ->> 'list_id' = @list_id THEN @watches ELSE @writes END, created_at
			FROM automation
			WHERE enabled AND archived_at IS NULL AND key = ANY(@keys)
			  AND (params ->> 'list_id' = @list_id OR params ->> 'shortlist_id' = @list_id)
			ORDER BY created_at, id`,
			pgx.StrictNamedArgs{
				"list_id": listID.String(), "keys": listRuleKeys,
				"watches": RuleWatchesList, "writes": RuleWritesList,
			})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ListRuleUse])
		return err
	})
	if err != nil || named {
		return out, err
	}
	for i := range out {
		out[i] = ListRuleUse{Role: out[i].Role, CreatedAt: out[i].CreatedAt}
	}
	return out, nil
}
