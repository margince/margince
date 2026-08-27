// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The partner extension's audit images. They live apart from the upsert that
// uses them because reading what the row WAS is a separate concern from
// writing what it becomes: the upsert coalesces every absent field onto its
// current value, so only a read of the standing row can say which fields the
// request actually moved.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// partnerAuditImage is one partner row's field values, keyed per FIELD because
// the human-edit-precedence probe derives ownership from these keys: a nested
// blob would make every partner field unownable.
func partnerAuditImage(r partnerRow) map[string]any {
	return map[string]any{
		"partner_role": r.PartnerRole, "cert_status": r.CertStatus,
		"margin_tier": r.MarginTier, "certified_staff": r.CertifiedStaff,
		"retention_rate": r.RetentionRate, "relationship_stage": r.RelationshipStage,
		"next_step": r.NextStep, "next_step_due_at": r.NextStepDueAt,
		"served_segments":   r.ServedSegments,
		"partner_fit_score": r.FitScore, "partner_fit_override_reason": r.FitOverrideReason,
	}
}

// emptyPartnerAuditImage is the before-image of an organization that is not a
// partner yet: every field explicitly null, so a promotion diffs against the
// absence it replaced rather than against an absent key. "It held nothing" and
// "nobody looked" are different answers and field history renders them apart.
// The field set is taken FROM partnerAuditImage rather than listed again: a
// second list would let the two disagree about which fields a partner has, and
// the promotion's image would then be silently short.
func emptyPartnerAuditImage() map[string]any {
	image := partnerAuditImage(partnerRow{})
	for field := range image {
		image[field] = nil
	}
	return image
}

// readPartnerImage reads the partner row as it stands before an upsert. The
// caller holds the organization row lock, which is what makes this image and
// the upsert that follows one transaction's work: read unlocked, a concurrent
// edit landing between them would be attributed to this request.
//
// An archived partner is read rather than skipped — the upsert revives it, so
// its stored values are genuinely what the revival replaced.
func readPartnerImage(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID) (map[string]any, error) {
	current, err := scanPartner(tx.QueryRow(ctx,
		`SELECT `+partnerColumns+` FROM partner WHERE organization_id = $1`, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return emptyPartnerAuditImage(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the partner field images: %w", err)
	}
	return partnerAuditImage(current), nil
}
