// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A company's main phone number and general email address. They are the
// company's own, so the email never names a contact and never finds one.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The two columns, as the patch and the clear list name them.
const (
	columnCompanyPhone = "phone"
	columnCompanyEmail = "email"
)

// parseCompanyPhoneAndEmail parses a create's phone and email, as a contact's number and
// address are parsed. A blank one is no value.
func parseCompanyPhoneAndEmail(phone, email *string) (*string, *string, error) {
	parsedPhone, err := companyPhoneValue(phone)
	if err != nil {
		return nil, nil, err
	}
	parsedEmail, err := companyEmailValue(email)
	if err != nil {
		return nil, nil, err
	}
	return parsedPhone, parsedEmail, nil
}

// companyPhoneValue is the stored spelling of a supplied phone, nil when it is
// absent or blank. A patch writes the nil as a clear.
func companyPhoneValue(raw *string) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil //nolint:nilnil // an absent or blank number is a NULL write, not an error
	}
	parsed, err := values.ParsePhone(*raw)
	if err != nil {
		return nil, err
	}
	stored := parsed.String()
	return &stored, nil
}

// companyEmailValue is companyPhoneValue for the general address.
func companyEmailValue(raw *string) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil //nolint:nilnil // an absent or blank address is a NULL write, not an error
	}
	parsed, err := values.ParseEmail(*raw)
	if err != nil {
		return nil, err
	}
	stored := parsed.String()
	return &stored, nil
}

// fillCompanyColumnFromFact fills an empty phone or email from the site read's
// accepted fact, as fillSizeBandFromFacts fills the size band. A typed value
// stands, and a fact that does not parse leaves the column for a cleaner read.
func fillCompanyColumnFromFact(
	ctx context.Context, tx pgx.Tx, in DeepReadProposal, by string, appliedFacts []map[string]any, fact string,
) error {
	if !factLanded(appliedFacts, fact) {
		return nil
	}
	for _, f := range in.Facts {
		if f.Category != factCategoryCompany || f.Field != fact {
			continue
		}
		column, value, ok := factColumnValue(fact, f.Value)
		if !ok {
			continue
		}
		var tag pgconn.CommandTag
		var err error
		if column == columnCompanyPhone {
			tag, err = tx.Exec(ctx, `UPDATE company SET phone = $2
			  WHERE id = $1 AND archived_at IS NULL AND phone IS NULL`, in.CompanyID, value)
		} else {
			tag, err = tx.Exec(ctx, `UPDATE company SET email = $2
			  WHERE id = $1 AND archived_at IS NULL AND email IS NULL`, in.CompanyID, value)
		}
		if err != nil {
			return fmt.Errorf("fill company %s: %w", column, err)
		}
		if tag.RowsAffected() != 1 {
			return nil
		}
		return stampSiteReadColumn(ctx, tx, in, by, column, f)
	}
	return nil
}

// factLanded reports whether the accept stored this fact. A fact the human
// precedence guard refused must not fill the column either.
func factLanded(appliedFacts []map[string]any, fact string) bool {
	for _, applied := range appliedFacts {
		if applied["field"] == fact {
			return true
		}
	}
	return false
}

// factColumnValue is the column a fact fills and its stored spelling, or false
// when the fact does not parse.
func factColumnValue(fact, raw string) (string, string, bool) {
	column, parse := columnCompanyEmail, companyEmailValue
	if fact == FactPhone {
		column, parse = columnCompanyPhone, companyPhoneValue
	}
	value, err := parse(&raw)
	if err != nil || value == nil {
		return "", "", false
	}
	return column, *value, true
}

// stampSiteReadColumn records the site read as the source of a filled column.
func stampSiteReadColumn(ctx context.Context, tx pgx.Tx, in DeepReadProposal, by, column string, f DeepReadFact) error {
	confidence := f.Confidence
	stamp := storekit.FieldStamp{Field: column, Confidence: &confidence}
	if f.SourceURL != "" {
		stamp.EvidenceRef = &f.SourceURL
	}
	return storekit.StampFields(ctx, tx, "company", in.CompanyID.UUID, companySourceSiteRead, by, []storekit.FieldStamp{stamp})
}
