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

// logoWrite is one change to an organization's mark, as the record of it needs
// to read: where the mark came from, where the one before it did, who made the
// change and under which source vocabulary.
//
// `origin` is nil for the two writes with no page behind them — an upload,
// which came off a person's disk, and a removal, which came from nowhere at
// all. `named` is what the field's history shows for the change: the page a
// read resolved from, the file a person chose, or nothing for a removal.
type logoWrite struct {
	previousOrigin *string
	origin         *string
	named          *string
	source         string
	by             string
}

// resolvedLogoWrite is what a website read records. The mark is named on every
// surface by the page it was resolved from, so that URL is both the origin
// stored on the row and the value the field's history shows.
func resolvedLogoWrite(previousOrigin *string, originURL, by string) logoWrite {
	return logoWrite{
		previousOrigin: previousOrigin, origin: &originURL, named: &originURL,
		source: companySourceSiteRead, by: by,
	}
}

// recordLogoWrite completes the logo write's shape: the field's provenance,
// the audit row, and the organization.updated event that links both into the
// trace. It runs inside the caller's transaction, so the mark and the record
// of who set it commit together or not at all.
func recordLogoWrite(ctx context.Context, tx pgx.Tx, id ids.OrganizationID, write logoWrite) error {
	if err := storekit.StampFields(ctx, tx, "organization", id.UUID, write.source, write.by,
		[]storekit.FieldStamp{{Field: logoFieldName, EvidenceRef: write.named}}); err != nil {
		return err
	}
	// A removal's after-image says the field is empty, which is the one thing a
	// reader of the history must be able to tell from "unchanged".
	delta := map[string]any{logoFieldName: nil}
	if write.named != nil {
		delta[logoFieldName] = *write.named
	}
	// The mark is named on every surface by the page it was resolved from, so
	// that URL is the field's VALUE here — and the caller reads the outgoing one
	// out of its own write, under the row lock, because a record with no logo
	// and a record whose logo this call replaced are different histories.
	//
	// The source vocabulary is context ABOUT the write and rides evidence: in
	// the image it would project as a change to a field called `source`
	// (storekit.AuditWithEvidence).
	before := map[string]any{logoFieldName: nil}
	if write.previousOrigin != nil {
		before[logoFieldName] = *write.previousOrigin
	}
	evidence := map[string]any{auditKeySource: write.source}
	if write.origin != nil {
		evidence[auditKeySourceURL] = *write.origin
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "organization", id.UUID,
		before, delta, evidence)
	if err != nil {
		return fmt.Errorf("audit organization logo: %w", err)
	}
	changed := map[string]any{eventKeyDelta: map[string]any{auditKeyFields: delta}}
	for key, value := range evidence {
		changed[key] = value
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventOrganizationUpdated{
		ChangedFields: changed,
	}); err != nil {
		return fmt.Errorf("emit organization.updated: %w", err)
	}
	return nil
}
