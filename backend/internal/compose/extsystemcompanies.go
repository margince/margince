// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of companies. A job tick or a bus delivery has no caller, so
// no grant narrows it. What only some colleagues may see is left out instead:
// the read answers a company whose visibility is the whole workspace, and gives
// the facts a unit needs to push or match it, no contact, note or custom field.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Companies() extension.Companies { return systemCompanies{rt: r} }

type systemCompanies struct{ rt *callRuntime }

const companySummaryColumns = `c.id, c.display_name, c.legal_name, c.lifecycle, c.owner_id, c.source_system, c.source_id, c.updated_at, c.archived_at`

func (c systemCompanies) Get(ctx context.Context, companyID string) (extension.CompanySummary, error) {
	ctx, err := c.rt.systemRead(ctx, extension.SystemReadCompanies)
	if err != nil {
		return extension.CompanySummary{}, err
	}
	id, err := unitID(companyID, "company")
	if err != nil {
		return extension.CompanySummary{}, err
	}
	if err := c.rt.traceSystemRead(ctx, extension.SystemReadCompanies, "company", companyID); err != nil {
		return extension.CompanySummary{}, err
	}
	var out extension.CompanySummary
	err = database.WithWorkspaceTx(ctx, c.rt.deps.pool, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+companySummaryColumns+` FROM company c WHERE c.id = $1 AND c.visibility = 'workspace'`, id)
		var err error
		out, err = scanCompanySummary(row)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return extension.CompanySummary{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.CompanySummary{}, c.rt.systemReadFault(ctx, err)
	}
	return out, nil
}

func (c systemCompanies) List(ctx context.Context, q extension.CompanyPageQuery) (extension.Page[extension.CompanySummary], error) {
	ctx, err := c.rt.systemRead(ctx, extension.SystemReadCompanies)
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	limit, err := mailPageLimit(q.Limit)
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	after := ids.UUID{}
	if q.Cursor != "" {
		if after, err = ids.Parse(q.Cursor); err != nil {
			return extension.Page[extension.CompanySummary]{}, fmt.Errorf("%w: the cursor is not one this read gave", extension.ErrInvalid)
		}
	}
	if err := c.rt.traceSystemRead(ctx, extension.SystemReadCompanies, "company", "list"); err != nil {
		return extension.Page[extension.CompanySummary]{}, err
	}
	archived := ""
	if !q.IncludeArchived {
		archived = " AND c.archived_at IS NULL"
	}
	var page extension.Page[extension.CompanySummary]
	err = database.WithWorkspaceTx(ctx, c.rt.deps.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+companySummaryColumns+` FROM company c
			 WHERE c.visibility = 'workspace' AND c.id > $1`+archived+` ORDER BY c.id LIMIT $2`, after, limit+1)
		if err != nil {
			return err
		}
		items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (extension.CompanySummary, error) { return scanCompanySummary(row) })
		if err != nil {
			return err
		}
		if len(items) > limit {
			items = items[:limit]
			page.NextCursor = items[limit-1].ID
		}
		page.Items = items
		return nil
	})
	if err != nil {
		return extension.Page[extension.CompanySummary]{}, c.rt.systemReadFault(ctx, err)
	}
	return page, nil
}

func scanCompanySummary(row pgx.Row) (extension.CompanySummary, error) {
	var (
		id                    ids.UUID
		name                  string
		legal, system, source *string
		lifecycle             string
		owner                 *ids.UUID
		updated               time.Time
		archivedAt            *time.Time
	)
	if err := row.Scan(&id, &name, &legal, &lifecycle, &owner, &system, &source, &updated, &archivedAt); err != nil {
		return extension.CompanySummary{}, err
	}
	out := extension.CompanySummary{ID: id.String(), Name: name, Lifecycle: lifecycle, UpdatedAt: updated, Archived: archivedAt != nil}
	if legal != nil {
		out.LegalName = *legal
	}
	if system != nil {
		out.SourceSystem = *system
	}
	if source != nil {
		out.SourceID = *source
	}
	if owner != nil {
		out.OwnerID = extension.UserID(owner.String())
	}
	return out, nil
}
