// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ruleRead is how far membersTx reads a rule: one bounded page for a reader,
// every match for the writer of the dated rows. A bounded read cannot tell a
// record past the bound from one that stopped matching.
type ruleRead bool

const (
	boundedRead  ruleRead = false
	completeRead ruleRead = true
	// rulePage is how many matches one page of a complete read holds.
	rulePage = 200
)

// ruleMatches evaluates one rule, or answers nothing for an initiative with
// none: no condition means no automatic matches.
func (s *Store) ruleMatches(ctx context.Context, tx pgx.Tx, recordType string, rule *crmcontracts.SalesInitiativeRule, read ruleRead, out *Membership) ([]ids.UUID, error) {
	if rule == nil || len(*rule) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	var predicate storekit.Predicate
	if err := json.Unmarshal(raw, &predicate); err != nil {
		return nil, httperr.Validation(recordType+"_rule", "invalid", "the rule is not a filter tree")
	}
	if read == completeRead {
		return s.allRuleMatches(ctx, tx, recordType, predicate)
	}
	matched, count, err := s.rules.Match(ctx, tx, recordType, predicate, ruleReadLimit)
	if err != nil {
		return nil, err
	}
	if count > len(matched) {
		out.Truncated = true
	}
	return matched, nil
}

// allRuleMatches walks every page of a rule.
func (s *Store) allRuleMatches(ctx context.Context, tx pgx.Tx, recordType string, rule storekit.Predicate) ([]ids.UUID, error) {
	var all []ids.UUID
	var after *ids.UUID
	for {
		page, more, err := s.rules.Page(ctx, tx, recordType, rule, after, rulePage)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if !more || len(page) == 0 {
			return all, nil
		}
		after = &page[len(page)-1]
	}
}

// include folds the rule matches and the picks of one record type into the
// membership, each with its reason, with the exclusions taken out. A picked
// record the reader may not see stays out of their reading.
func (s *Store) include(ctx context.Context, tx pgx.Tx, out *Membership, matched, picked []ids.UUID, recordType string) ([]ids.UUID, error) {
	included := make([]ids.UUID, 0, len(matched)+len(picked))
	for _, id := range matched {
		if out.Excluded[recordType][id] {
			continue
		}
		included = append(included, id)
		out.Reasons[recordType][id] = "rule"
	}
	visible, err := visibleRecords(ctx, tx, recordType, picked)
	if err != nil {
		return nil, err
	}
	for _, id := range visible {
		if out.Excluded[recordType][id] || slices.Contains(included, id) {
			continue
		}
		included = append(included, id)
		out.Reasons[recordType][id] = "picked"
	}
	return included, nil
}

// visibleRecords narrows picked ids to the rows the reader's row scope reaches.
func visibleRecords(ctx context.Context, tx pgx.Tx, recordType string, picked []ids.UUID) ([]ids.UUID, error) {
	// A type the reader's role cannot read holds no record they can see, so its
	// picks stay out of their reading instead of failing it.
	if len(picked) == 0 || !auth.ReadGranted(ctx, recordType) {
		return nil, nil
	}
	args := []any{picked}
	scope, err := auth.ScopeClauseFor(ctx, recordType, "t", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = unscoped
	}
	rows, err := tx.Query(ctx, `SELECT t.id FROM `+recordType+` t WHERE t.id = ANY($1) AND t.archived_at IS NULL AND `+scope, args...)
	if err != nil {
		return nil, fmt.Errorf("read picked %ss: %w", recordType, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// createdBefore keeps the rule matches that existed at the launch, for an
// initiative that closed its doors to later matches.
func createdBefore(ctx context.Context, tx pgx.Tx, recordType string, matched []ids.UUID, launch time.Time) ([]ids.UUID, error) {
	if len(matched) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT t.id FROM `+recordType+` t WHERE t.id = ANY($1) AND t.created_at < $2`, matched, launch)
	if err != nil {
		return nil, fmt.Errorf("read the %ss that existed at the launch: %w", recordType, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

func carriedDeals(ctx context.Context, tx pgx.Tx, id ids.UUID, launch time.Time, leads []ids.UUID) ([]ids.UUID, error) {
	if !auth.ReadGranted(ctx, recordLead) || !auth.ReadGranted(ctx, recordDeal) {
		return nil, nil
	}
	args := []any{id, launch, leads}
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	dealScope, err := auth.ScopeClauseFor(ctx, recordDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if dealScope == "" {
		dealScope = unscoped
	}
	rows, err := tx.Query(ctx, `SELECT d.id FROM deal d WHERE (d.id IN (`+promotedFromEnrolledSQL+`) OR d.id IN (SELECT l.qualified_deal_id FROM lead l WHERE l.id = ANY($3)))
		AND d.archived_at IS NULL AND `+dealScope, args...)
	if err != nil {
		return nil, fmt.Errorf("read carried deals: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
