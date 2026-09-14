// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BackfillEmploymentImport uses a keyset cursor, so an administrator can pause
// between bounded batches. Preview calls only reads and never provider APIs.
func (s *Store) BackfillEmploymentImport(ctx context.Context, after *ids.UUID, limit int, apply bool) (crmcontracts.EmploymentBackfillReport, error) {
	if err := auth.RequireAdmin(ctx); err != nil {
		return crmcontracts.EmploymentBackfillReport{}, err
	}
	contacts, more, err := s.employmentImportContacts(ctx, after, limit, false)
	if err != nil {
		return crmcontracts.EmploymentBackfillReport{}, err
	}
	out := crmcontracts.EmploymentBackfillReport{Applied: apply, HasMore: more, Reports: []crmcontracts.EmploymentImportReport{}}
	for _, contact := range contacts {
		var report crmcontracts.EmploymentImportReport
		if apply {
			report, err = s.ApplyEmploymentImport(ctx, contact, crmcontracts.EmploymentImportRequest{})
		} else {
			report, err = s.PreviewEmploymentImport(ctx, contact)
		}
		if err != nil {
			return crmcontracts.EmploymentBackfillReport{}, err
		}
		out.Reports = append(out.Reports, report)
		next := openapi_types.UUID(contact.UUID)
		out.NextCursor = &next
	}
	return out, nil
}

// SweepEmploymentImports reconciles retained purchases even after a provider is
// disconnected. It does not need vendor credentials and cannot buy a lookup.
func (s *Store) SweepEmploymentImports(ctx context.Context) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	var after *ids.UUID
	var failed []error
	for batch := 0; batch < 10; batch++ {
		contacts, more, err := s.employmentImportContacts(ctx, after, 10, true)
		if err != nil {
			return err
		}
		for _, contact := range contacts {
			_, err := s.ApplyEmploymentImport(ctx, contact, crmcontracts.EmploymentImportRequest{})
			if err != nil {
				terminal, retryErr := s.deferEmploymentImport(ctx, contact, err)
				if retryErr != nil {
					failed = append(failed, retryErr)
				} else if !terminal {
					failed = append(failed, err)
				}
			}
			last := contact.UUID
			after = &last
		}
		if !more {
			break
		}
	}
	return errors.Join(failed...)
}

func (s *Store) employmentImportContacts(ctx context.Context, after *ids.UUID, limit int, pendingOnly bool) ([]ids.ContactID, bool, error) {
	if limit < 1 || limit > 25 {
		limit = 10
	}
	out := []ids.ContactID{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		where := `c.archived_at IS NULL AND p.claim_key IN ('current_employment','job_history')`
		if after != nil {
			where += storekit.SQLf(" AND c.id > $%d", arg(*after))
		}
		if pendingOnly {
			where += ` AND p.employment_processed_at IS NULL AND (p.employment_retry_at IS NULL OR p.employment_retry_at <= now())`
		}
		query := storekit.SQLf(`SELECT DISTINCT c.id FROM contact c JOIN contact_provider_claim p ON p.contact_id=c.id WHERE %s ORDER BY c.id LIMIT $%d`, where, arg(limit+1))
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var contact ids.ContactID
			if err := rows.Scan(&contact); err != nil {
				return err
			}
			out = append(out, contact)
		}
		return rows.Err()
	})
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, err
}

// Retry scheduling is queue metadata; the failed domain transaction wrote nothing.
func (s *Store) deferEmploymentImport(ctx context.Context, contact ids.ContactID, failure error) (bool, error) {
	var inputError *EmploymentImportError
	terminal := errors.As(failure, &inputError)
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{contact, terminal}
		query := storekit.SQLf(`WITH deferred AS (
 UPDATE contact_provider_claim SET employment_retry_count=employment_retry_count+1,
 employment_retry_at=now()+make_interval(secs => 60 * power(2,least(employment_retry_count,5))::integer),
 employment_processing_error='Employment import needs administrator review. Correct the stored evidence or configuration, then retry this contact.',
 employment_processed_at=CASE WHEN $%d OR employment_retry_count>=5 THEN now() ELSE NULL END
 WHERE contact_id=$%d AND employment_processed_at IS NULL AND claim_key IN ('current_employment','job_history')
 RETURNING employment_processed_at IS NOT NULL AS stopped)
 SELECT coalesce(bool_and(stopped),true) FROM deferred`, len(args), len(args)-1)
		return tx.QueryRow(ctx, query, args...).Scan(&terminal)
	})
	return terminal, err
}
