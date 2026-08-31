// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	// The request field a category refusal names. `evidenceFieldKey` already
	// carries "field" for the evidence images; a fact's category had no
	// constant, so it gets one here beside the code that refuses on it.
	factCategoryKey = "category"
	// The sidecar table these two verbs write. Named because three writers in
	// this package now reach it and a table name typed four times is a typo
	// waiting to become a runtime error.
	tableOrganizationFact = "organization_fact"
)

// FactCreateInput is a fact a person states about a company.
//
// The dedupe key is absent on purpose: it is derived from the value here, never
// taken from the caller. A supplied key could name a row the value does not
// belong to, or slip past the uniqueness the store's own reads depend on — the
// same reason the deep-read validator recomputes and checks it rather than
// trusting what it was handed.
type FactCreateInput struct {
	Category string
	Field    string
	Value    string
}

// factInVocabulary reports whether this category holds this field.
//
// Split out of validDeepReadFact so a HAND-stated fact can be checked against
// the same vocabulary without also being required to carry the evidence a crawl
// produces: a person stating what a company runs has a snippet of nothing, and
// demanding one would refuse every fact a human states.
func factInVocabulary(category, field string) error {
	fields, ok := OrganizationFactFields[category]
	if !ok {
		return &values.ParseError{
			Field: factCategoryKey, Code: "fact_category_unknown",
			Message: "a fact category is one of company, offering, market or signal",
		}
	}
	for _, name := range fields {
		if name == field {
			return nil
		}
	}
	return &values.ParseError{
		Field: evidenceFieldKey, Code: "fact_field_unknown",
		Message: fmt.Sprintf("%q is not a %s fact field", field, category),
	}
}

// factValueKeyFor derives a fact's dedupe identity from its value: the
// normalized name for a repeatable field, and the empty string for a
// single-value one, which is what the org_fact_value_key_cardinality check
// requires of each.
func factValueKeyFor(field, value string) string {
	if OrganizationFactMultiValue[field] {
		return NormalizeFactValueKey(value)
	}
	return ""
}

// CreateOrganizationFact records a fact a person states about a company.
//
// The row is human-owned from birth, which is also what protects it: both
// enrichment upserts decline to overwrite a row whose captured_by is a human,
// so the next site read leaves it alone.
func (s *Store) CreateOrganizationFact(
	ctx context.Context, orgID ids.OrganizationID, in FactCreateInput,
) (crmcontracts.OrganizationFact, error) {
	value := strings.TrimSpace(in.Value)
	if value == "" {
		return crmcontracts.OrganizationFact{}, &values.ParseError{
			Field: auditKeyValue, Code: "fact_value_empty",
			Message: "a fact carries a value",
		}
	}
	if err := factInVocabulary(in.Category, in.Field); err != nil {
		return crmcontracts.OrganizationFact{}, err
	}
	valueKey := factValueKeyFor(in.Field, value)
	factKey := in.Field + ":" + valueKey

	return writeEvidence(ctx, s, orgID, evidenceWrite[crmcontracts.OrganizationFact]{
		table:      tableOrganizationFact,
		archived:   storekit.NoArchiveColumn,
		changedKey: factKey,
		value:      &value,
		readBefore: func(ctx context.Context, tx pgx.Tx) (evidenceRow, bool, error) {
			_, err := readFactRow(ctx, tx, orgID, factKey)
			if err == nil {
				// Already stated. Upserting here would let a hand write
				// overwrite a machine claim with none of the correction path's
				// before-image, so the honest verbs are named instead.
				return evidenceRow{}, false, fmt.Errorf(
					"this company already states %s; confirm or correct it instead: %w",
					factKey, apperrors.ErrConflict)
			}
			if !errors.Is(err, apperrors.ErrNotFound) {
				return evidenceRow{}, false, err
			}
			minted, err := insertHumanOrganizationFact(ctx, tx, orgID, in.Category, in.Field, value, valueKey)
			if err != nil {
				return evidenceRow{}, false, err
			}
			return minted, true, nil
		},
		readAfter: func(ctx context.Context, tx pgx.Tx) (crmcontracts.OrganizationFact, error) {
			return readFactWire(ctx, tx, orgID, factKey)
		},
	})
}

// DeleteOrganizationFact removes a fact this company does not state.
//
// Correction answers "this value is wrong"; this answers "this is not a fact
// about this company at all", which a correction cannot say. The row is deleted
// rather than flagged, so a later site read may state the fact again — removal
// means "not true today". The audit before-image keeps what was removed.
func (s *Store) DeleteOrganizationFact(
	ctx context.Context, orgID ids.OrganizationID, factKey string, in FactWriteInput,
) error {
	_, err := writeEvidence(ctx, s, orgID, evidenceWrite[crmcontracts.OrganizationFact]{
		table:      tableOrganizationFact,
		archived:   storekit.NoArchiveColumn,
		changedKey: factKey,
		ifVersion:  in.IfVersion,
		remove:     true,
		readBefore: func(ctx context.Context, tx pgx.Tx) (evidenceRow, bool, error) {
			row, err := readFactRow(ctx, tx, orgID, factKey)
			return row, false, err
		},
	})
	return err
}

// insertHumanOrganizationFact writes the row a person stated and reads back the
// before-image shape the shared writer works in.
//
// `evidence_snippet` and `source_url` are empty and `confidence` is 1: a person
// stating a fact IS the evidence, and there is no page to quote. That is the
// same shape the site-read resolution path already inserts a human-edited fact
// in, so the two hand-written facts in this store are one kind of row.
func insertHumanOrganizationFact(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	category, field, value, valueKey string,
) (evidenceRow, error) {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return evidenceRow{}, err
	}
	var r evidenceRow
	err = tx.QueryRow(ctx, `
		INSERT INTO organization_fact
		       (organization_id, category, field, value, value_key,
		        evidence_snippet, source_url, confidence, source, captured_by, site_read_id)
		VALUES ($1, $2, $3, $4, $5, '', '', 1, 'human', $6, NULL)
		RETURNING id, value, source, evidence_snippet, source_url, confidence,
		          verified_at, verified_by, captured_by`,
		orgID, category, field, value, valueKey, capturedBy,
	).Scan(&r.ID, &r.Value, &r.Source, &r.EvidenceSnippet, &r.SourceURL, &r.Confidence,
		&r.VerifiedAt, &r.VerifiedBy, &r.CapturedBy)
	// The probe above and this insert are two statements, so a second caller
	// can land the same fact between them. uq_org_fact is what actually
	// prevents the duplicate; without translating its violation the loser of
	// that race gets a 500 where the contract promises a 409, and the answer
	// would depend on timing rather than on what is true.
	if constraint, dup := storekit.UniqueViolation(err); dup && constraint == "uq_org_fact" {
		return evidenceRow{}, fmt.Errorf(
			"this company already states %s:%s; confirm or correct it instead: %w",
			field, valueKey, apperrors.ErrConflict)
	}
	if err != nil {
		return evidenceRow{}, fmt.Errorf("state organization fact %s.%s: %w", category, field, err)
	}
	return r, nil
}
