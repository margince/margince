// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Membership is who belongs to an initiative, as the reader may see it. It
// holds the rule matches, the picked records and the deals promoted from
// member leads, less the exclusions. Launch is when the baseline was frozen;
// zero for a draft.
type Membership struct {
	Leads, Deals []ids.UUID
	// Reasons names why each included record belongs: rule, picked or carried.
	Reasons map[ids.UUID]string
	// Excluded are the records a hand took out, over a rule match or a pick,
	// keyed by record type.
	Excluded  map[string]map[ids.UUID]bool
	Truncated bool
	Launch    time.Time
}

// MembershipOf answers the membership for a reporting read under the caller's
// row scope. It is the one evaluation the report engine and the members list
// share, so the two never disagree about who belongs.
func (s *Store) MembershipOf(ctx context.Context, tx pgx.Tx, id ids.UUID) (Membership, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return Membership{}, err
	}
	initiative, err := readInitiative(ctx, tx, id)
	if err != nil {
		return Membership{}, err
	}
	return s.membersTx(ctx, tx, initiative)
}

func (s *Store) membersTx(ctx context.Context, tx pgx.Tx, initiative crmcontracts.SalesInitiative) (Membership, error) {
	out := Membership{Reasons: map[ids.UUID]string{}, Excluded: map[string]map[ids.UUID]bool{recordLead: {}, recordDeal: {}}}
	if initiative.LaunchAt != nil {
		out.Launch = *initiative.LaunchAt
	}
	id := ids.UUID(initiative.Id)
	rows, err := tx.Query(ctx, `SELECT record_type, record_id, reason FROM sales_initiative_member WHERE initiative_id = $1`, id)
	if err != nil {
		return out, fmt.Errorf("read initiative members: %w", err)
	}
	picked := map[string][]ids.UUID{}
	for rows.Next() {
		var recordType, reason string
		var recordID ids.UUID
		if err := rows.Scan(&recordType, &recordID, &reason); err != nil {
			rows.Close()
			return out, err
		}
		if reason == "excluded" {
			out.Excluded[recordType][recordID] = true
		} else {
			picked[recordType] = append(picked[recordType], recordID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	leads, err := s.ruleMatches(ctx, tx, recordLead, initiative.LeadRule, &out)
	if err != nil {
		return out, err
	}
	deals, err := s.ruleMatches(ctx, tx, recordDeal, initiative.DealRule, &out)
	if err != nil {
		return out, err
	}
	if initiative.LaunchAt != nil && !initiative.IncludeFuture {
		if leads, err = createdBefore(ctx, tx, recordLead, leads, out.Launch); err != nil {
			return out, err
		}
		if deals, err = createdBefore(ctx, tx, recordDeal, deals, out.Launch); err != nil {
			return out, err
		}
	}
	if out.Leads, err = s.include(ctx, tx, &out, leads, picked[recordLead], recordLead); err != nil {
		return out, err
	}
	// A deal promoted from a member lead carries the lead's membership, the
	// one downstream link a promotion records (lead.qualified_deal_id). The
	// reader sees it only when their row scope reaches the deal.
	carried, err := carriedDeals(ctx, tx, out.Leads)
	if err != nil {
		return out, err
	}
	if out.Deals, err = s.include(ctx, tx, &out, deals, picked[recordDeal], recordDeal); err != nil {
		return out, err
	}
	for _, deal := range carried {
		if out.Excluded[recordDeal][deal] || slices.Contains(out.Deals, deal) {
			continue
		}
		out.Deals = append(out.Deals, deal)
		out.Reasons[deal] = "carried"
	}
	return out, nil
}

// ruleMatches evaluates one rule, or answers nothing for an initiative with
// none: no condition means no automatic matches.
func (s *Store) ruleMatches(ctx context.Context, tx pgx.Tx, recordType string, rule *crmcontracts.SalesInitiativeRule, out *Membership) ([]ids.UUID, error) {
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
	matched, count, err := s.rules.Match(ctx, tx, recordType, predicate, ruleReadLimit)
	if err != nil {
		return nil, err
	}
	if count > len(matched) {
		out.Truncated = true
	}
	return matched, nil
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
		out.Reasons[id] = "rule"
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
		out.Reasons[id] = "picked"
	}
	return included, nil
}

// visibleRecords narrows picked ids to the rows the reader's row scope reaches.
func visibleRecords(ctx context.Context, tx pgx.Tx, recordType string, picked []ids.UUID) ([]ids.UUID, error) {
	if len(picked) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, recordType, principal.ActionRead); err != nil {
		return nil, err
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
		scope = "TRUE"
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

func carriedDeals(ctx context.Context, tx pgx.Tx, leads []ids.UUID) ([]ids.UUID, error) {
	if len(leads) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{leads}
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	dealScope, err := auth.ScopeClauseFor(ctx, recordDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if dealScope == "" {
		dealScope = "TRUE"
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT l.qualified_deal_id FROM lead l JOIN deal d ON d.id = l.qualified_deal_id WHERE l.id = ANY($1) AND l.qualified_deal_id IS NOT NULL AND d.archived_at IS NULL AND `+dealScope, args...)
	if err != nil {
		return nil, fmt.Errorf("read carried deals: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// Members lists the membership with labels, for the editor and the review.
func (s *Store) Members(ctx context.Context, id ids.UUID, recordType *string) (crmcontracts.SalesInitiativeMembers, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return crmcontracts.SalesInitiativeMembers{}, err
	}
	out := crmcontracts.SalesInitiativeMembers{Data: []crmcontracts.SalesInitiativeMember{}}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		membership, err := s.MembershipOf(ctx, tx, id)
		if err != nil {
			return err
		}
		out.Leads, out.Deals, out.Truncated = len(membership.Leads), len(membership.Deals), membership.Truncated
		companies := map[ids.UUID]bool{}
		if recordType == nil || *recordType == recordLead {
			rows, err := leadRows(ctx, tx, membership, companies)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, rows...)
		}
		if recordType == nil || *recordType == recordDeal {
			rows, err := dealRows(ctx, tx, membership, companies)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, rows...)
		}
		out.Companies = len(companies)
		return nil
	})
	return out, err
}

// memberRead is the clause every member row read carries: the wanted ids,
// live rows only, and the reader's row scope on the record type. A company
// name is read only through the company grant and its own row scope.
func memberRead(ctx context.Context, recordType, alias string, wanted []ids.UUID) (where, companyLabel string, args []any, err error) {
	args = []any{wanted}
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	scope, err := auth.ScopeClauseFor(ctx, recordType, alias, arg)
	if err != nil {
		return "", "", nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	companyLabel = "NULL::text"
	if auth.ReadGranted(ctx, "company") {
		if companyLabel, err = storekit.LinkedCompanyColumn(ctx, alias+".company_id", "display_name", arg); err != nil {
			return "", "", nil, err
		}
	}
	return alias + ".id = ANY($1) AND " + alias + ".archived_at IS NULL AND " + scope, companyLabel, args, nil
}

func leadRows(ctx context.Context, tx pgx.Tx, membership Membership, companies map[ids.UUID]bool) ([]crmcontracts.SalesInitiativeMember, error) {
	wanted := append(slices.Clone(membership.Leads), excludedOf(membership, recordLead)...)
	if len(wanted) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return nil, err
	}
	where, companyLabel, args, err := memberRead(ctx, recordLead, "l", wanted)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT l.id, COALESCE(NULLIF(l.full_name,''), NULLIF(l.company_name,''), ''), l.company_id, `+companyLabel+`, l.status, l.created_at
		FROM lead l WHERE `+where+` ORDER BY l.created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("read member leads: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.SalesInitiativeMember, error) {
		var id ids.UUID
		var company *ids.UUID
		var member crmcontracts.SalesInitiativeMember
		var status string
		if err := row.Scan(&id, &member.Label, &company, &member.CompanyLabel, &status, &member.CreatedAt); err != nil {
			return member, err
		}
		member.RecordType, member.RecordId, member.Status = recordLead, openapi_types.UUID(id), &status
		memberReason(&member, recordLead, id, membership)
		if member.Included && company != nil {
			companies[*company] = true
		}
		return member, nil
	})
}

func dealRows(ctx context.Context, tx pgx.Tx, membership Membership, companies map[ids.UUID]bool) ([]crmcontracts.SalesInitiativeMember, error) {
	wanted := append(slices.Clone(membership.Deals), excludedOf(membership, recordDeal)...)
	if len(wanted) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	where, companyLabel, args, err := memberRead(ctx, recordDeal, "d", wanted)
	if err != nil {
		return nil, err
	}
	amount, err := auth.MaskedExpressionSQL(ctx, "deal", "amount_minor", "d", "d.amount_minor_base", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id, d.name, d.company_id, `+companyLabel+`, d.status, `+amount+`, d.created_at
		FROM deal d WHERE `+where+` ORDER BY d.created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("read member deals: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.SalesInitiativeMember, error) {
		var id ids.UUID
		var company *ids.UUID
		var member crmcontracts.SalesInitiativeMember
		var status string
		if err := row.Scan(&id, &member.Label, &company, &member.CompanyLabel, &status, &member.AmountMinorBase, &member.CreatedAt); err != nil {
			return member, err
		}
		member.RecordType, member.RecordId, member.Status = recordDeal, openapi_types.UUID(id), &status
		memberReason(&member, recordDeal, id, membership)
		if member.Included && company != nil {
			companies[*company] = true
		}
		return member, nil
	})
}

func memberReason(member *crmcontracts.SalesInitiativeMember, recordType string, id ids.UUID, membership Membership) {
	if membership.Excluded[recordType][id] {
		member.Reason, member.Included = crmcontracts.SalesInitiativeMemberReasonExcluded, false
		return
	}
	member.Reason, member.Included = crmcontracts.SalesInitiativeMemberReason(membership.Reasons[id]), true
}

func excludedOf(membership Membership, recordType string) []ids.UUID {
	out := make([]ids.UUID, 0, len(membership.Excluded[recordType]))
	for id := range membership.Excluded[recordType] {
		out = append(out, id)
	}
	return out
}

// ReplaceMembers sets the picked and the excluded records. The rule rows are
// never written: a match is evaluated live, and an exclusion stands over it.
func (s *Store) ReplaceMembers(ctx context.Context, id ids.UUID, picked, excluded []crmcontracts.SalesInitiativeMemberRef) (crmcontracts.SalesInitiativeMembers, error) {
	if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
		return crmcontracts.SalesInitiativeMembers{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.SalesInitiativeMembers{}, err
	}
	for _, ref := range picked {
		for _, other := range excluded {
			if ref.RecordType == other.RecordType && ref.RecordId == other.RecordId {
				return crmcontracts.SalesInitiativeMembers{}, httperr.Validation("excluded", "conflict", "a record is picked and excluded at once")
			}
		}
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := lockInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := ensureVisibleRefs(ctx, tx, picked, excluded); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sales_initiative_member WHERE initiative_id = $1`, id); err != nil {
			return fmt.Errorf("clear initiative members: %w", err)
		}
		for reason, refs := range map[string][]crmcontracts.SalesInitiativeMemberRef{"picked": picked, "excluded": excluded} {
			for _, ref := range refs {
				if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative_member (initiative_id, record_type, record_id, reason, captured_by) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
					id, string(ref.RecordType), ids.UUID(ref.RecordId), reason, by); err != nil {
					return fmt.Errorf("write initiative member: %w", err)
				}
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE sales_initiative SET version = version + 1, updated_at = now() WHERE id = $1 AND archived_at IS NULL AND version = $2`, id, versionOf(before))
		if err != nil {
			return fmt.Errorf("bump initiative: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.ErrVersionSkew
		}
		after, err := readInitiative(ctx, tx, id)
		if err != nil {
			return err
		}
		return recordChange(ctx, tx, after, &before, []string{"members"})
	})
	if err != nil {
		return crmcontracts.SalesInitiativeMembers{}, err
	}
	return s.Members(ctx, id, nil)
}

// ensureVisibleRefs refuses, as not found, a pick or exclusion of a record
// the caller cannot read. So a guessed id neither joins nor leaves an
// initiative.
func ensureVisibleRefs(ctx context.Context, tx pgx.Tx, picked, excluded []crmcontracts.SalesInitiativeMemberRef) error {
	named := map[string][]ids.UUID{}
	for _, ref := range append(slices.Clone(picked), excluded...) {
		named[string(ref.RecordType)] = append(named[string(ref.RecordType)], ids.UUID(ref.RecordId))
	}
	for recordType, wanted := range named {
		visible, err := visibleRecords(ctx, tx, recordType, wanted)
		if err != nil {
			return err
		}
		for _, id := range wanted {
			if !slices.Contains(visible, id) {
				return fmt.Errorf("%s %s is not a record you can read: %w", recordType, id, apperrors.ErrNotFound)
			}
		}
	}
	return nil
}
