// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which offering a company context describes. The main offering is the anchor
// company's profile; a further legal entity carries its own.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OfferingSelector names the offering to read. The zero value is the main
// offering, the anchor company's.
type OfferingSelector struct{ EntityID ids.LegalEntityID }

// ParseOfferingSelector reads the selector a model request carries. Empty is
// the main offering; anything that is not an id is refused rather than
// widened to the main offering.
func ParseOfferingSelector(raw string) (OfferingSelector, error) {
	if raw == "" {
		return OfferingSelector{}, nil
	}
	id, err := ids.ParseAs[ids.LegalEntityKind](raw)
	if err != nil {
		return OfferingSelector{}, fmt.Errorf("offering %q is not a legal entity id: %w", raw, err)
	}
	return OfferingSelector{EntityID: id}, nil
}

// String is the spelling ParseOfferingSelector reads back.
func (o OfferingSelector) String() string {
	if o.EntityID.IsZero() {
		return ""
	}
	return o.EntityID.String()
}

// offeringMinimumComplete is the one definition of a described offering: a
// name, what it sells and who it sells to. The anchor read and the entity read
// both ask it, so onboarding and the growth fit cannot disagree.
func offeringMinimumComplete(displayName, offerSummary, icp string) bool {
	return strings.TrimSpace(displayName) != "" &&
		strings.TrimSpace(offerSummary) != "" &&
		strings.TrimSpace(icp) != ""
}

// entityOfferingCapturedBy stamps an entity's offering items. The entity row
// keeps no author per field, and the stamp is part of the context fingerprint.
const entityOfferingCapturedBy = "legal_entity"

type readOfferingResult struct {
	company Company
	id      ids.LegalEntityID
	name    string
}

func readOffering(ctx context.Context, tx pgx.Tx, offering OfferingSelector) (readOfferingResult, error) {
	if !offering.EntityID.IsZero() {
		return readEntityOffering(ctx, tx, offering.EntityID)
	}
	companyID, err := anchorCompany(ctx, tx, false)
	if err != nil {
		return readOfferingResult{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
		return readOfferingResult{}, err
	}
	company, err := readAnchorCompany(ctx, tx, companyID)
	if err != nil {
		return readOfferingResult{}, err
	}
	var defaultID ids.LegalEntityID
	err = tx.QueryRow(ctx, `SELECT id FROM legal_entity WHERE is_default`).Scan(&defaultID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return readOfferingResult{}, fmt.Errorf("read the default legal entity: %w", err)
	}
	return readOfferingResult{company: company, id: defaultID, name: company.DisplayName}, nil
}

// readEntityOffering builds the Company-shaped input assembleCompanyContext
// reads from a live further entity. The default entity and an archived or
// unknown one answer ErrNotFound: the default's offering is the anchor's.
func readEntityOffering(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID) (readOfferingResult, error) {
	entity, err := readLegalEntity(ctx, tx, id)
	if err != nil {
		return readOfferingResult{}, err
	}
	if entity.IsDefault || entity.ArchivedAt != nil {
		return readOfferingResult{}, apperrors.ErrNotFound
	}
	company := Company{
		DisplayName: entity.DisplayName, CompanySource: CompanySourceHuman,
		CompanyCapturedBy: entityOfferingCapturedBy,
	}
	offering := entity.Offering
	for _, field := range []struct {
		name  string
		value *string
	}{
		{fieldOfferSummary, offering.OfferSummary}, {fieldICP, offering.ICP},
		{fieldValueProposition, offering.ValueProposition}, {fieldUSP, offering.USP},
	} {
		if field.value == nil {
			continue
		}
		company.ProfileFields = append(company.ProfileFields, CompanyProfileField{
			Field: field.name, Value: *field.value, Source: CompanySourceHuman,
			CapturedBy: entityOfferingCapturedBy,
		})
	}
	company.MinimumComplete = offeringMinimumComplete(entity.DisplayName,
		deref(offering.OfferSummary), deref(offering.ICP))
	return readOfferingResult{company: company, id: id, name: entity.DisplayName}, nil
}
