// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A company read shows the company's partner programme, and the company list
// narrows by it. Both hang off the partner grant: a seat that may not read
// partners sees companies without the programme, and a filter on it is a 403.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// partnerProgramFilter narrows by a partner programme: a kind it holds, the
// stage key it stands on, or the meaning of that stage.
type partnerProgramFilter struct {
	Kind, Stage, Meaning *string
}

func (f partnerProgramFilter) empty() bool {
	return f.Kind == nil && f.Stage == nil && f.Meaning == nil
}

// check refuses a kind or meaning outside its vocabulary, naming the caller's
// own parameter. A filter nothing can match is a client mistake.
func (f partnerProgramFilter) check(kindField, meaningField string) error {
	if f.Kind != nil && !crmcontracts.PartnerKind(*f.Kind).Valid() {
		return httperr.Validation(kindField, "not_a_known_value",
			"filter by hosting, consulting, strategic or referral, or leave the parameter off")
	}
	if f.Meaning != nil && !crmcontracts.PartnerStageMeaning(*f.Meaning).Valid() {
		return httperr.Validation(meaningField, "not_a_known_value",
			"filter by candidate, signed, ready, active, dormant or ended, or leave the parameter off")
	}
	return nil
}

// partnerProgramClauses renders the filter over the aliased partner row. The
// partner list and the company list share it, so "an active partner" means one
// thing on both.
func partnerProgramClauses(alias string, f partnerProgramFilter, arg func(any) int) []string {
	var where []string
	if f.Kind != nil {
		where = append(where, storekit.SQLf("%s.partner_kinds @> ARRAY[$%d]::text[]", alias, arg(*f.Kind)))
	}
	if f.Stage != nil {
		where = append(where, storekit.SQLf("%s.relationship_stage = $%d", alias, arg(*f.Stage)))
	}
	if f.Meaning != nil {
		where = append(where, storekit.SQLf(`EXISTS (SELECT 1 FROM partner_stage ps
			WHERE ps.key = %s.relationship_stage AND ps.meaning = $%d)`, alias, arg(*f.Meaning)))
	}
	return where
}

// companyPartnerClause narrows the company list to companies whose live
// programme matches, or "" when the caller named no partner filter. The filter
// reads partner data, so it takes the partner grant.
func companyPartnerClause(ctx context.Context, f partnerProgramFilter, arg func(any) int) (string, error) {
	if f.empty() {
		return "", nil
	}
	if err := auth.Require(ctx, partnerMaskObject, principal.ActionRead); err != nil {
		return "", err
	}
	if err := f.check(filterPartnerKind, filterPartnerStageMeaning); err != nil {
		return "", err
	}
	clauses := append([]string{livePartnerSQL("pp")}, partnerProgramClauses("pp", f, arg)...)
	return storekit.SQLf("EXISTS (SELECT 1 FROM partner pp WHERE pp.company_id = company.id AND %s)",
		strings.Join(clauses, " AND ")), nil
}

// attachCompanyPartners fills each company's live partner programme for a
// caller who may read partners. The companies are already scoped to the
// caller, so the partner rows need no scope of their own.
func attachCompanyPartners(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company) error {
	if len(companies) == 0 || !auth.Allows(ctx, partnerMaskObject, principal.ActionRead) {
		return nil
	}
	companyIDs := make([]ids.UUID, len(companies))
	for i := range companies {
		companyIDs[i] = ids.UUID(companies[i].Id)
	}
	rows, err := tx.Query(ctx, `SELECT `+partnerColumns+` FROM partner
		WHERE company_id = ANY($1) AND `+livePartnerSQL(""), companyIDs)
	if err != nil {
		return fmt.Errorf("read the companies' partner programmes: %w", err)
	}
	partnerRows, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (partnerRow, error) { return scanPartner(row) })
	if err != nil {
		return fmt.Errorf("collect the companies' partner programmes: %w", err)
	}
	partners, err := wirePartners(ctx, tx, partnerRows)
	if err != nil {
		return err
	}
	byCompany := make(map[ids.UUID]*crmcontracts.Partner, len(partners))
	for i := range partners {
		byCompany[ids.UUID(partners[i].CompanyId)] = &partners[i]
	}
	for i := range companies {
		companies[i].Partner = byCompany[ids.UUID(companies[i].Id)]
	}
	return nil
}
