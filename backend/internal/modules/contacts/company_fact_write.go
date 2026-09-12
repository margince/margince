// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// FactWriteInput carries a fact correction. Value is nil for a confirmation.
type FactWriteInput struct {
	Value     *string
	IfVersion *int64
}

// UpdateCompanyFact corrects an extracted fact's value.
//
// Unlike a profile field, a fact has no canonical column anywhere — the whole
// claim lives in the sidecar — so the row IS the value and there is no second
// write to keep in step.
func (s *Store) UpdateCompanyFact(
	ctx context.Context, companyID ids.CompanyID, factKey string, in FactWriteInput,
) (crmcontracts.CompanyFact, error) {
	if in.Value == nil {
		return crmcontracts.CompanyFact{}, fmt.Errorf(
			"a correction carries a value; use the confirm operation to agree without changing one: %w",
			apperrors.ErrConflict)
	}
	return s.writeFact(ctx, companyID, factKey, in)
}

// ConfirmCompanyFact records that a human read the fact and agreed.
func (s *Store) ConfirmCompanyFact(
	ctx context.Context, companyID ids.CompanyID, factKey string, in FactWriteInput,
) (crmcontracts.CompanyFact, error) {
	in.Value = nil
	return s.writeFact(ctx, companyID, factKey, in)
}

func (s *Store) writeFact(
	ctx context.Context, companyID ids.CompanyID, factKey string, in FactWriteInput,
) (crmcontracts.CompanyFact, error) {
	// No canonical hook: the whole claim lives in the sidecar, so the row IS
	// the value and there is no second write to keep in step.
	return writeEvidence(ctx, s, companyID, evidenceWrite[crmcontracts.CompanyFact]{
		table:      tableCompanyFact,
		archived:   storekit.NoArchiveColumn,
		changedKey: factKey,
		value:      in.Value,
		ifVersion:  in.IfVersion,
		// Never creates: a fact is addressed by `<field>:<value_key>`, a key a
		// machine derives from what it read, so there is no vocabulary a contact
		// could originate one from. An absent fact stays not-found.
		readBefore: func(ctx context.Context, tx pgx.Tx) (evidenceRow, bool, error) {
			row, err := readFactRow(ctx, tx, companyID, factKey)
			return row, false, err
		},
		readAfter: func(ctx context.Context, tx pgx.Tx) (crmcontracts.CompanyFact, error) {
			return readFactWire(ctx, tx, companyID, factKey)
		},
	})
}

// splitFactKey reads the `<field>:<value_key>` identity the contract addresses
// a fact by (FactKey parameter) into the two columns that actually locate the
// row.
//
// NEITHER HALF IS ENOUGH ALONE. A multi-value field holds several rows, so
// `field` does not name one; and every company fact carries an empty value_key
// by the company_fact_value_key_cardinality check, so matching on value_key alone would
// make `phone`, `founded_year` and `contact_email` all answer to the same
// query and a correction would land on whichever row the scan reached first.
// The pair is exact: uq_company_fact is unique on (category, field, value_key), and
// company_fact_field_vocab gives each field exactly one category, so field
// determines category and (field, value_key) identifies the row.
//
// The split is on the FIRST colon, so a normalized value_key may contain one.
func splitFactKey(factKey string) (field, valueKey string, ok bool) {
	field, valueKey, ok = strings.Cut(factKey, ":")
	if !ok || field == "" {
		return "", "", false
	}
	return field, valueKey, true
}

// errMalformedFactKey refuses a key that names no row, rather than letting it
// fall through to a not-found that would read as "this fact once existed".
func errMalformedFactKey() error {
	return &values.ParseError{
		Field: "factKey", Code: "fact_key_malformed",
		Message: `a fact key is spelled <field>:<value_key>; a single-value fact ends in a bare colon, e.g. "phone:"`,
	}
}

func readFactRow(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, factKey string,
) (evidenceRow, error) {
	var r evidenceRow
	field, valueKey, ok := splitFactKey(factKey)
	if !ok {
		return r, errMalformedFactKey()
	}
	var category string
	err := tx.QueryRow(ctx, `
		SELECT id, category, value, source, evidence_snippet, source_url, confidence, verified_at, verified_by, captured_by
		  FROM company_fact
		 WHERE company_id = $1 AND field = $2 AND value_key = $3`,
		companyID, field, valueKey,
	).Scan(&r.ID, &category, &r.Value, &r.Source, &r.EvidenceSnippet, &r.SourceURL, &r.Confidence,
		&r.VerifiedAt, &r.VerifiedBy, &r.CapturedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperrors.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("read company fact: %w", err)
	}
	// What this row IS, for the audit before-image. A removal leaves an
	// entity_id pointing at a row that no longer exists, so without these four
	// the trail cannot answer which company lost which fact.
	r.Identity = map[string]any{
		siteReadCompanyKey: companyID.UUID,
		factCategoryKey:    category,
		evidenceFieldKey:   field,
		"value_key":        valueKey,
	}
	return r, nil
}

func readFactWire(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, factKey string,
) (crmcontracts.CompanyFact, error) {
	var (
		f           crmcontracts.CompanyFact
		id          ids.UUID
		category    string
		field, srcV string
	)
	keyField, valueKey, ok := splitFactKey(factKey)
	if !ok {
		return f, errMalformedFactKey()
	}
	err := tx.QueryRow(ctx, `
		SELECT id, category, field, value, value_key, source, captured_by,
		       evidence_snippet, source_url, confidence,
		       retrieved_at, verified_at, verified_by, updated_at, version
		  FROM company_fact
		 WHERE company_id = $1 AND field = $2 AND value_key = $3`,
		companyID, keyField, valueKey,
	).Scan(&id, &category, &field, &f.Value, &f.ValueKey, &srcV, &f.CapturedBy,
		&f.EvidenceSnippet, &f.SourceUrl, &f.Confidence,
		&f.RetrievedAt, &f.VerifiedAt, &f.VerifiedBy, &f.UpdatedAt, &f.Version)
	if err != nil {
		return f, fmt.Errorf("re-read company fact: %w", err)
	}
	wireID := openapi_types.UUID(id)
	f.Id = &wireID
	f.Category = crmcontracts.CompanyFactCategory(category)
	f.Field = crmcontracts.CompanyFactField(field)
	f.Source = crmcontracts.CompanyFactSource(srcV)
	return f, nil
}
