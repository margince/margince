// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a lead's linked company records, as the lead read shows it and the lead list
// narrows by it. Also the industry a lead holds itself.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

const (
	leadIndustryField = "industry"
	filterCountry     = "country"
)

// leadCompanyFacts pairs each fact with its company column. The read and the
// dials both name columns through it, so a dial never selects by a value the row would not show.
var leadCompanyFacts = []struct{ field, column string }{
	{filterIndustry, fieldIndustry},
	{filterCountry, "address_country"},
	{filterSizeBand, filterSizeBand},
}

// leadCompanyFactsSQL is the linked company's facts as one JSON object, NULL without
// company read. withholdUnreadableCompanies then applies the company row scope.
func leadCompanyFactsSQL(ctx context.Context) string {
	if !auth.ReadGranted(ctx, companyEntity) {
		return "NULL::jsonb"
	}
	pairs := make([]string, 0, len(leadCompanyFacts))
	for _, fact := range leadCompanyFacts {
		pairs = append(pairs, "'"+fact.field+"', c."+fact.column)
	}
	return `(SELECT jsonb_build_object(` + strings.Join(pairs, ", ") + `) FROM company c WHERE c.id = lead.company_id)`
}

// scanLeadCompanyFacts decodes leadCompanyFactsSQL's column onto the lead.
func scanLeadCompanyFacts(l *crmcontracts.Lead, raw []byte) error {
	if raw == nil {
		return nil
	}
	var facts crmcontracts.LeadCompanyFacts
	if err := json.Unmarshal(raw, &facts); err != nil {
		return fmt.Errorf("decode the company facts of lead %s: %w", l.Id, err)
	}
	l.CompanyFacts = &facts
	return nil
}

// leadIndustryWrite maps industry text to a code as the company writer does; blank stores
// none. Text naming no code is refused: a lead has no description to keep it in.
func leadIndustryWrite(raw *string) (*string, error) {
	write := resolveIndustryWrite(raw)
	if write.given && write.code == nil {
		return nil, httperr.Validation(leadIndustryField, "not_a_known_value",
			"industry must name one of the industry codes, such as logistics_transport, or a label of one")
	}
	return write.code, nil
}

// LeadCompanyFactFilters are the lead list's dials over what the linked company
// records. Each reads a company column, so each needs the company read grant.
type LeadCompanyFactFilters struct {
	Industry *string
	Country  *string
	SizeBand *string
}

// clauses narrows a lead list by the facts asked for. A company outside the row scope
// counts as no company, so no dial tells the caller what it records.
func (f LeadCompanyFactFilters) clauses(ctx context.Context, arg func(any) int) ([]string, error) {
	if f == (LeadCompanyFactFilters{}) {
		return nil, nil
	}
	operands, err := f.operands()
	if err != nil {
		return nil, err
	}
	var where []string
	for _, fact := range leadCompanyFacts {
		operand, asked := operands[fact.field]
		if !asked {
			continue
		}
		value, err := storekit.LinkedCompanyColumn(ctx, "lead.company_id", fact.column, arg)
		if err != nil {
			return nil, err
		}
		if fact.field == filterIndustry {
			value = leadIndustrySQL(value)
		}
		where = append(where, storekit.SQLf("%s = $%d", value, arg(operand)))
	}
	return where, nil
}

// leadIndustrySQL is the industry a lead is in: its company's, else its own.
// companyIndustry is the company's column as the caller may see it.
func leadIndustrySQL(companyIndustry string) string {
	return "coalesce(" + companyIndustry + ", lead." + leadIndustryField + ")"
}

// operands checks each dial asked for, by wire name. A value no company holds is refused:
// the empty page it would answer reads as no such leads.
func (f LeadCompanyFactFilters) operands() (map[string]string, error) {
	operands := map[string]string{}
	if f.Industry != nil {
		if !crmcontracts.ListLeadsParamsIndustry(*f.Industry).Valid() {
			return nil, httperr.Validation(filterIndustry, "not_a_known_value",
				"filter by one of the industries the contract defines, or leave the parameter off")
		}
		operands[filterIndustry] = *f.Industry
	}
	if f.Country != nil {
		country, err := storekit.CountryOperand(*f.Country, filterCountry)
		if err != nil {
			return nil, err
		}
		operands[filterCountry] = country
	}
	if f.SizeBand != nil {
		if !crmcontracts.ListLeadsParamsSizeBand(*f.SizeBand).Valid() {
			return nil, httperr.Validation(filterSizeBand, "not_a_known_value",
				"filter by one of the size bands the contract defines, or leave the parameter off")
		}
		operands[filterSizeBand] = *f.SizeBand
	}
	return operands, nil
}
