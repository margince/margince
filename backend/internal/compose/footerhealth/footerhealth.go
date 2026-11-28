// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package footerhealth raises System health's warning for a legal entity whose
// mail must show a company detail nobody stated.
package footerhealth

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

const (
	// SourceID names the System health source.
	SourceID = "footer"
	// Object is the grant that writes the company details. The source asks
	// for it with update, so only a seat that may fill them in is told.
	Object = "installation_settings"
	// IncompleteCode is the finding and source code of a gap.
	IncompleteCode = "footer_incomplete"
	// EntityName is the gap of a default entity whose company states neither
	// a display name nor a legal name. Every contact without an entity of its
	// own falls back to that entity.
	EntityName = "entity_name"
)

// Gap is one entity's gap with the name its company states.
type Gap struct {
	consent.FooterGap
	Name string
}

// Summary reads the source and its findings. A seat that may not change the
// details sees the source withheld. base builds a finding counting held messages.
func Summary(
	ctx context.Context, pool *pgxpool.Pool, store *consent.Store, source crmcontracts.HealthSource, now time.Time,
	base func(held int) crmcontracts.HealthFinding,
) (crmcontracts.HealthSource, []crmcontracts.HealthFinding, error) {
	source.Permission = Object + ":" + string(principal.ActionUpdate)
	if !auth.Allows(ctx, Object, principal.ActionUpdate) {
		source.Availability, source.Code = crmcontracts.HealthSourceAvailabilityWithheld, string(crmcontracts.HealthSourceAvailabilityWithheld)
		return source, nil, nil
	}
	var gaps []Gap
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		gaps, err = GapsTx(ctx, tx, store)
		return err
	})
	if err != nil {
		return source, nil, err
	}
	source.ObservedAt, source.Condition, source.Code = &now, crmcontracts.HealthSourceConditionClear, "footer_complete"
	findings := make([]crmcontracts.HealthFinding, 0, len(gaps))
	for _, gap := range gaps {
		f := Finding(ctx, base(gap.HeldByUser+gap.HeldInBackground), gap)
		f.ObservedAt = &now
		findings = append(findings, f)
		source.Condition, source.Code = crmcontracts.HealthSourceConditionWaitingHuman, IncompleteCode
	}
	return source, findings, nil
}

// GapsTx names each entity and keeps the ones with a gap. store is the
// disclosure store the send path is wired with, so the warning and the held
// send read one answer. The default entity's name is its anchor company's.
func GapsTx(ctx context.Context, tx pgx.Tx, store *consent.Store) ([]Gap, error) {
	gaps, err := store.FooterGapsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := []Gap{}
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
			gap.Missing = append(gap.Missing, EntityName)
		}
		if name == "" {
			name = legalName
		}
		if len(gap.Missing) > 0 {
			out = append(out, Gap{FooterGap: gap, Name: name})
		}
	}
	return out, nil
}

// Finding completes base, a waiting-for-a-human finding counting the held
// messages, with what the gap names. A button the reader cannot use is off.
func Finding(ctx context.Context, base crmcontracts.HealthFinding, gap Gap) crmcontracts.HealthFinding {
	f := base
	f.Id += ":" + gap.Entity.String()
	f.Responsible = crmcontracts.HealthFindingResponsibleAdministrator
	f.Urgency = 1
	f.Recovery = SourceID
	if gap.HeldByUser+gap.HeldInBackground > 0 {
		f.Impact = crmcontracts.HealthFindingImpactMeasured
	}
	missing := make([]crmcontracts.HealthFooterGapMissing, 0, len(gap.Missing))
	for _, kind := range gap.Missing {
		missing = append(missing, crmcontracts.HealthFooterGapMissing(kind))
	}
	f.FooterGap = &crmcontracts.HealthFooterGap{
		LegalEntityId: openapi_types.UUID(gap.Entity), LegalEntityName: gap.Name, IsDefault: gap.IsDefault,
		Missing: missing, HeldByUser: gap.HeldByUser, HeldInBackground: gap.HeldInBackground,
	}
	f.Action.Destination = Destination(gap)
	if !remedyAllowed(ctx, f.Action.Destination) {
		reason := "permission"
		f.Action.Available, f.Action.Reason = false, &reason
	}
	return f
}

// Destination is where the first missing detail is filled in. A further
// entity states its own name and address on its record, and the default
// entity's name is its company's. Every other detail of a footer is the
// installation's, on the disclosure page.
func Destination(gap Gap) crmcontracts.HealthActionDestination {
	switch {
	case gap.Missing[0] == EntityName:
		return crmcontracts.HealthActionDestinationCompanyProfile
	case !gap.IsDefault && gap.Missing[0] == string(messagingrules.ControllerIdentity):
		return crmcontracts.HealthActionDestinationLegalEntities
	}
	return crmcontracts.HealthActionDestinationDisclosures
}

// remedyAllowed reports whether the reader may change what the destination
// holds. A seat that may only fill the disclosure page still sees a gap on
// the entity record, with its button off.
func remedyAllowed(ctx context.Context, destination crmcontracts.HealthActionDestination) bool {
	switch destination {
	case crmcontracts.HealthActionDestinationLegalEntities:
		return auth.Allows(ctx, "legal_entity", principal.ActionUpdate)
	case crmcontracts.HealthActionDestinationCompanyProfile:
		return auth.RequireAdmin(ctx) == nil
	}
	return true
}
