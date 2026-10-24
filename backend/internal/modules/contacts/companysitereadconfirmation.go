// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// recordSiteReadConfirmation writes the confirmation's audit row, event and
// dossier mark, and returns the changed-columns image the audit row carries.
func recordSiteReadConfirmation(ctx context.Context, tx pgx.Tx, read SiteRead, confirmation siteReadConfirmation) (map[string]any, error) {
	action := actionUpdate
	if confirmation.target.created {
		action = actionCreate
	}
	before, after, err := anchorSaveImages(ctx, tx, confirmation.target)
	if err != nil {
		return nil, err
	}
	// Confirmation provenance belongs in evidence; field history treats every
	// key in the before/after image as a changed company field.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, action, "company", confirmation.target.id.UUID, before, after, map[string]any{
		auditKeySource: companySourceSiteRead, auditKeySourceURL: read.SeedURL,
		auditKeyFields: confirmation.appliedSite, "human_fields": confirmation.appliedHuman,
		auditKeyFacts: confirmation.appliedFacts, "site_read_id": read.ID, "draft_version": read.DraftVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("audit company site-read confirmation: %w", err)
	}
	payload := siteReadConfirmationPayload(read, confirmation)
	if err := storekit.EmitEvent(ctx, tx, auditID, confirmation.target.id.UUID, payload); err != nil {
		return nil, fmt.Errorf("emit %s: %w", payload.EventType(), err)
	}
	// An empty proposal list is known, not SQL NULL.
	if _, err := tx.Exec(ctx, `UPDATE site_read
		SET company_id = $2, proposal_ids = coalesce($3, '{}'::uuid[]),
		    confirmed_at = now(), updated_at = now()
		WHERE id = $1`, read.ID, confirmation.target.id, confirmation.proposalIDs); err != nil {
		return nil, fmt.Errorf("mark website read confirmed: %w", err)
	}
	return after, nil
}

// siteReadConfirmationPayload preserves the distinct public create/update
// contracts while carrying the same confirmation evidence.
//
//nolint:ireturn // dispatches to PublicEventCompanyCreated vs Updated by confirmation.created; tested directly via the interface in contact_company_payload_test.go
func siteReadConfirmationPayload(read SiteRead, confirmation siteReadConfirmation) events.Payload {
	delta := map[string]any{
		auditKeyFields: confirmation.appliedSite,
		"human_fields": confirmation.appliedHuman,
		auditKeyFacts:  confirmation.appliedFacts,
	}
	if confirmation.target.created {
		source := companySourceSiteRead
		sourceURL := read.SeedURL
		siteReadID := openapi_types.UUID(read.ID)
		capturedBy := companySiteReadCapturedBy
		return crmcontracts.PublicEventCompanyCreated{
			Delta:      &delta,
			Source:     &source,
			SourceUrl:  &sourceURL,
			SiteReadId: &siteReadID,
			CapturedBy: &capturedBy,
		}
	}
	return crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta:  delta,
			auditKeySource: companySourceSiteRead, auditKeySourceURL: read.SeedURL,
			"site_read_id": read.ID, eventKeyCapturedBy: companySiteReadCapturedBy,
		},
	}
}
