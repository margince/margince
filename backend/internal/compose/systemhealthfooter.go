// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

const (
	healthFooterID = "footer"
	// healthFooterObject is the grant that writes the company details, which
	// the source asks for with update.
	healthFooterObject   = "installation_settings"
	footerIncompleteCode = "footer_incomplete"
)

// footerDetailEntityName is the gap of a default entity whose company states
// neither a display name nor a legal name. Every contact without an entity of
// its own falls back to that entity.
const footerDetailEntityName = "entity_name"

// entityFooterGap is one entity's gap with the name its company states.
type entityFooterGap struct {
	consent.FooterGap
	name string
}

// footerSummary raises one finding per live legal entity whose mail owes a
// company detail nobody stated. It asks the disclosure store the send path is
// wired with, so the finding and the held send read one answer.
func (s Server) footerSummary(ctx context.Context) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source := healthSource(healthFooterID, healthFooterObject)
	pool := s.jobHealthHandlers.pool
	store := newDisclosureAdapter(InstallationDB(pool)).store
	gaps, err := healthTx(ctx, pool, func(ctx context.Context, tx pgx.Tx) ([]entityFooterGap, error) {
		return footerGapsTx(ctx, tx, store)
	})
	if err != nil {
		return source, nil, err
	}
	now := utcNow()
	source.ObservedAt = &now
	source.Condition, source.Code = healthClear, "footer_complete"
	findings := make([]crmcontracts.HealthFinding, 0, len(gaps))
	for _, gap := range gaps {
		findings = append(findings, footerFinding(source, gap))
	}
	if len(findings) > 0 {
		source.Condition, source.Code = healthWaitingHuman, footerIncompleteCode
	}
	return source, findings, nil
}

// footerGapsTx names each entity and keeps the ones with a gap. The default
// entity's name is its anchor company's, which contacts resolves.
func footerGapsTx(ctx context.Context, tx pgx.Tx, store *consent.Store) ([]entityFooterGap, error) {
	gaps, err := store.FooterGapsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := []entityFooterGap{}
	for _, gap := range gaps {
		identity, err := contacts.ResolveEntityIdentity(ctx, tx, ids.From[ids.LegalEntityKind](gap.Entity))
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(identity.DisplayName)
		legalName := ""
		if identity.LegalName != nil {
			legalName = strings.TrimSpace(*identity.LegalName)
		}
		if gap.IsDefault && name == "" && legalName == "" {
			gap.Missing = append(gap.Missing, footerDetailEntityName)
		}
		if name == "" {
			name = legalName
		}
		if len(gap.Missing) > 0 {
			out = append(out, entityFooterGap{FooterGap: gap, name: name})
		}
	}
	return out, nil
}

func footerFinding(source crmcontracts.HealthSource, gap entityFooterGap) crmcontracts.HealthFinding {
	held := gap.HeldByPerson + gap.HeldInBackground
	f := healthFinding(source, footerIncompleteCode, healthWaitingHuman, held, string(crmcontracts.HealthFindingUnitMessages))
	f.Id += ":" + gap.Entity.String()
	f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
	f.Urgency = 1
	f.Recovery = healthFooterID
	if held > 0 {
		f.Impact = crmcontracts.HealthFindingImpactMeasured
	}
	missing := make([]crmcontracts.HealthFooterGapMissing, 0, len(gap.Missing))
	for _, kind := range gap.Missing {
		missing = append(missing, crmcontracts.HealthFooterGapMissing(kind))
	}
	f.FooterGap = &crmcontracts.HealthFooterGap{
		LegalEntityId: openapi_types.UUID(gap.Entity), LegalEntityName: gap.name, IsDefault: gap.IsDefault,
		Missing: missing, HeldByPerson: gap.HeldByPerson, HeldInBackground: gap.HeldInBackground,
	}
	f.Action.Destination = footerDestination(gap)
	return f
}

// footerDestination is where the first missing detail is filled in. A further
// entity states its own name and address on its record, and the default
// entity's name is its company's. Every other detail of a footer is the
// installation's, on the disclosure page.
func footerDestination(gap entityFooterGap) crmcontracts.HealthActionDestination {
	switch {
	case gap.Missing[0] == footerDetailEntityName:
		return crmcontracts.HealthActionDestinationCompanyProfile
	case !gap.IsDefault && gap.Missing[0] == string(messagingrules.ControllerIdentity):
		return crmcontracts.HealthActionDestinationLegalEntities
	}
	return crmcontracts.HealthActionDestinationDisclosures
}
