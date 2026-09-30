// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reading ONE deal: the column list, the row read and the scan.
//
// Split from deal_read.go, which is the LIST path — its input, its filters, its
// sort fields and its page scan. The two were one file until the author columns
// pushed it past the 500-line cap, and they are two concepts: what a list of
// deals is, and what one deal is.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// A var rather than a const: the seat-name subselect is built by a function.
var dealColumns = `id, name, amount_minor, expected_arr_minor, arr_source_offer_id, currency, pipeline_id, stage_id,
	company_id, project_id, owner_id, partner_company_id, partner_attribution, status, lost_reason,
	won_without_contract_reason, won_without_contract_detail,
	description, commercial_motion, priority, acquisition_source,
	expected_close_date, close_date_provisional, closed_at, forecast_category, wait_until, last_activity_at,
	source, captured_by,
	source_system, source_author_id, source_author_name,
	` + sourceAuthorSeatNameSQL("deal") + `,
	version, created_at, updated_at, archived_at, legal_hold`

// readDeal resolves one deal row; active names the custom-field columns
// to carry alongside the core ones — nil for internal decision reads
// whose result never reaches the wire.
func readDeal(ctx context.Context, tx pgx.Tx, id ids.DealID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Deal, error) {
	q := `SELECT ` + dealColumns + storekit.SelectSuffix(active) + ` FROM deal WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += liveRowsClause
	}
	d, err := scanDeal(tx.QueryRow(ctx, q, id), active)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Deal{}, apperrors.ErrNotFound
	}
	return d, err
}

// scanDeal scans core + active custom columns; extra receives any
// trailing expressions the caller's SELECT appended (the sorted list's
// cursor key).
func scanDeal(row pgx.Row, active []fieldcatalog.Column, extra ...any) (crmcontracts.Deal, error) {
	var d crmcontracts.Deal
	var id, pipelineID, stageID ids.UUID
	var companyID, projectID, ownerID, partnerID, arrSource *ids.UUID
	var status string
	var forecastCat *string
	var expectedClose, waitUntil *time.Time
	var closeDateProvisional bool
	var version int64

	var wonReason, motion, priority *string
	var sourceSystem, authorName, authorSeatName *string
	var authorID *ids.UUID
	dests := []any{
		&id, &d.Name, &d.AmountMinor, &d.ExpectedArrMinor, &arrSource, &d.Currency, &pipelineID, &stageID,
		&companyID, &projectID, &ownerID, &partnerID, &d.PartnerAttribution, &status, &d.LostReason,
		&wonReason, &d.WonWithoutContractDetail,
		&d.Description, &motion, &priority, &d.AcquisitionSource,
		&expectedClose, &closeDateProvisional, &d.ClosedAt, &forecastCat, &waitUntil, &d.LastActivityAt,
		&d.Source, &d.CapturedBy,
		&sourceSystem, &authorID, &authorName, &authorSeatName,
		&version, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt, &d.LegalHold,
	}
	cf := storekit.ScanDests(active)
	if err := row.Scan(append(append(dests, cf...), extra...)...); err != nil {
		return d, err
	}
	if values := storekit.ExtractValues(active, cf); len(values) > 0 {
		d.AdditionalProperties = values
	}
	if forecastCat != nil {
		cat := crmcontracts.DealForecastCategory(*forecastCat)
		d.ForecastCategory = &cat
	}
	if wonReason != nil {
		reason := crmcontracts.DealWonWithoutContractReason(*wonReason)
		d.WonWithoutContractReason = &reason
	}
	if motion != nil {
		m := crmcontracts.DealCommercialMotion(*motion)
		d.CommercialMotion = &m
	}
	if priority != nil {
		pr := crmcontracts.DealPriority(*priority)
		d.Priority = &pr
	}

	d.Id = openapi_types.UUID(id)
	pid := openapi_types.UUID(pipelineID)
	d.PipelineId = &pid
	sid := openapi_types.UUID(stageID)
	d.StageId = &sid
	d.CompanyId = uuidPtr(companyID)
	d.ArrSourceOfferId = uuidPtr(arrSource)
	d.ProjectId = uuidPtr(projectID)
	d.OwnerId = uuidPtr(ownerID)
	d.PartnerCompanyId = uuidPtr(partnerID)
	d.Status = crmcontracts.DealStatus(status)
	if expectedClose != nil {
		d.ExpectedCloseDate = &openapi_types.Date{Time: *expectedClose}
	}
	d.CloseDateProvisional = &closeDateProvisional
	if waitUntil != nil {
		d.WaitUntil = &openapi_types.Date{Time: *waitUntil}
	}
	d.Version = &version
	d.Author = sourceAuthorOf(authorID, authorSeatName, authorName, sourceSystem)
	stalled := IsStalled(status, d.CreatedAt, d.LastActivityAt, waitUntil, time.Now().UTC())
	d.Stalled = &stalled
	return d, nil
}
