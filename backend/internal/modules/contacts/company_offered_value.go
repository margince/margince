// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The value a source last offered for a company field, kept beside the value on
// the record. A feed that must leave a field a user edited still has its offer
// remembered here, so a screen can read both.

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordOfferedValues keeps offered (field key to value) as what source last
// offered for the company. A field the source offers again replaces its own
// earlier value; another source's value for the same field is not touched.
func (s *Store) RecordOfferedValues(ctx context.Context, id ids.CompanyID, source string, offered map[string]string) error {
	if len(offered) == 0 {
		return nil
	}
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return err
	}
	fields := make([]string, 0, len(offered))
	for field := range offered {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	values := make([]string, len(fields))
	for i, field := range fields {
		values[i] = offered[field]
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritableLive(ctx, tx, "company", id.UUID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO company_offered_value (company_id, source_system, field_key, offered_value)
			SELECT $1, $2, field_key, offered_value FROM unnest($3::text[], $4::text[]) AS o(field_key, offered_value)
			ON CONFLICT (company_id, source_system, field_key)
			DO UPDATE SET offered_value = EXCLUDED.offered_value, offered_at = now()`,
			id.UUID, source, fields, values)
		if err != nil {
			return err
		}
		// Names only: the values may hold personal data the trail must not copy.
		_, err = storekit.AuditEventWithEvidence(ctx, tx, "update", "company", id.UUID, nil,
			map[string]any{"source_system": source, auditKeyFields: fields})
		return err
	})
}

// ListOfferedValues answers what each source last offered for the company, one
// row per source and field, under the caller's own read gates.
func (s *Store) ListOfferedValues(ctx context.Context, id ids.CompanyID) ([]crmcontracts.CompanyOfferedValue, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.CompanyOfferedValue{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureReadable(ctx, tx, "company", id.UUID); err != nil {
			return err
		}
		// An archived company answers 404, like its own read, so offers kept
		// for a restore are not served while the record is retired.
		if _, err := readCompanyRow(ctx, tx, id, storekit.LiveOnly, nil); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT source_system, field_key, offered_value, offered_at FROM company_offered_value
			WHERE company_id = $1 ORDER BY source_system, field_key`, id.UUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v crmcontracts.CompanyOfferedValue
			var at time.Time
			if err := rows.Scan(&v.SourceSystem, &v.Field, &v.Value, &at); err != nil {
				return err
			}
			v.OfferedAt = at
			out = append(out, v)
		}
		return rows.Err()
	})
	return out, err
}
