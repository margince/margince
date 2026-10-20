// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	mergeContactAddressSource = "a.contact_id = @source AND a.archived_at IS NULL"
	mergeCompanyDomainSource  = "a.company_id = @source AND a.archived_at IS NULL"
	mergeEmailPrimary         = `a.is_primary AND NOT EXISTS (SELECT 1 FROM contact_email b WHERE b.contact_id = @target AND b.email_type = a.email_type AND b.is_primary AND b.archived_at IS NULL)`
	mergePhonePrimary         = `a.is_primary AND NOT EXISTS (SELECT 1 FROM contact_phone b WHERE b.contact_id = @target AND b.phone_type = a.phone_type AND b.is_primary AND b.archived_at IS NULL)`
	mergeDomainPrimary        = `a.is_primary AND NOT EXISTS (SELECT 1 FROM company_domain b WHERE b.company_id = @target AND b.is_primary AND b.archived_at IS NULL)`
)

type MergeAddress struct {
	ID            ids.UUID
	Kind          string
	Value         string
	Usage         string
	WasPrimary    bool
	WillBePrimary bool
}

func previewMergeAddresses(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeAddress, error) {
	fields := []string{"email", "emails", "phone", "phones"}
	if kind == "company" {
		fields = []string{"domains"}
	}
	if err := previewFieldsVisible(ctx, kind, fields...); err != nil {
		return nil, err
	}
	queries := []struct{ kind, sql string }{
		{"email", `SELECT a.id, a.email, a.email_type, a.is_primary, ` + mergeEmailPrimary + ` FROM contact_email a WHERE ` + mergeContactAddressSource + ` ORDER BY a.id LIMIT 501`},
		{"phone", `SELECT a.id, a.phone, a.phone_type, a.is_primary, ` + mergePhonePrimary + ` FROM contact_phone a WHERE ` + mergeContactAddressSource + ` ORDER BY a.id LIMIT 501`},
	}
	if kind == "company" {
		queries = []struct{ kind, sql string }{{"domain", `SELECT a.id, a.domain, '', a.is_primary, ` + mergeDomainPrimary + ` FROM company_domain a WHERE ` + mergeCompanyDomainSource + ` ORDER BY a.id LIMIT 501`}}
	}
	var out []MergeAddress
	for _, query := range queries {
		rows, err := tx.Query(ctx, query.sql, pgx.StrictNamedArgs{"source": source, "target": target})
		if err != nil {
			return nil, err
		}
		addresses, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MergeAddress, error) {
			address := MergeAddress{Kind: query.kind}
			err := row.Scan(&address.ID, &address.Value, &address.Usage, &address.WasPrimary, &address.WillBePrimary)
			return address, err
		})
		if err != nil {
			return nil, err
		}
		if len(addresses) > 500 {
			return nil, apperrors.ErrInvalidArgument
		}
		out = append(out, addresses...)
	}
	return out, nil
}
