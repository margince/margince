// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The write shape of an organization's logo. It lives apart from the resolve
// and adoption paths next door because it is what those two SHARE: both end by
// stamping the field's provenance, recording what the mark was and is, and
// publishing the record's change — and a second spelling of that would let a
// mark land on a company with nothing saying who chose it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordLogoWrite completes the logo write's shape: the field's provenance,
// the audit row, and the organization.updated event that links both into the
// trace. It runs inside the caller's transaction, so the mark and the record
// of who set it commit together or not at all.
func recordLogoWrite(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, previousOrigin *string, originURL, by string) error {
	if err := storekit.StampFields(ctx, tx, "organization", id.UUID, companySourceSiteRead, by,
		[]storekit.FieldStamp{{Field: logoFieldName, EvidenceRef: &originURL}}); err != nil {
		return err
	}
	delta := map[string]any{logoFieldName: originURL}
	// The mark is named on every surface by the page it was resolved from, so
	// that URL is the field's VALUE here — and the caller reads the outgoing one
	// out of its own write, under the row lock, because a record with no logo
	// and a record whose logo this call replaced are different histories.
	//
	// The source vocabulary is context ABOUT the write and rides evidence: in
	// the image it would project as a change to a field called `source`
	// (storekit.AuditWithEvidence).
	before := map[string]any{logoFieldName: nil}
	if previousOrigin != nil {
		before[logoFieldName] = *previousOrigin
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "organization", id.UUID,
		before, delta, map[string]any{
			auditKeySource: companySourceSiteRead, auditKeySourceURL: originURL,
		})
	if err != nil {
		return fmt.Errorf("audit organization logo: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventOrganizationUpdated{
		ChangedFields: map[string]any{
			eventKeyDelta:  map[string]any{auditKeyFields: delta},
			auditKeySource: companySourceSiteRead, auditKeySourceURL: originURL,
		},
	}); err != nil {
		return fmt.Errorf("emit organization.updated: %w", err)
	}
	return nil
}
