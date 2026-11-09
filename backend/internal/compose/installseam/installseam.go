// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package installseam names the installation values a module reads and the
// readers that answer them. It is a leaf: it imports the module that OWNS the
// settings and the modules that consume them, and nothing imports it back.
//
// It exists so there is ONE spelling of that wiring. Compose owns cross-module
// edges (ADR-0054), but the integration harness cannot reach compose — the
// harness is a non-test file in a package compose's own tests import, so the
// import is a cycle — and a second copy built by hand there is exactly the
// drift NewSettingsStore was extracted to prevent.
package installseam

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Deals is the installation seam the deals module reads through.
func Deals() deals.Installation {
	return deals.Installation{
		Name: identity.NameOf,
		// contacts owns `company` and its provenance sidecar, so the "has a
		// human confirmed what this installation is called" read lives there
		// and the edge is injected here.
		IssuerLegalName: contacts.ConfirmedIssuerLegalName,
		BaseCurrency:    identity.BaseCurrencyOf,
		Country:         installationCountry,
		KnownCountry: func(code string) (bool, error) {
			return contacts.IsAssignedCountry(code), nil
		},
		Timezone: identity.TimezoneOf,
		// The Value sort reads both ungated: a seat that may read deals may
		// order them, whatever its installation_settings grant.
		BaseCurrencyApplied: identity.BaseCurrencyAppliedTx,
		TimezoneApplied:     identity.TimezoneAppliedTx,
		// activities owns `activity`, so the stamp's write lives there and the
		// edge is injected here (ADR-0054).
		StampCorrespondence: activities.StampCorrespondenceForDeal,
		// contacts owns `partner`, so the "is this company a partner" read lives
		// there and the edge is injected here for the same reason.
		EnsurePartner: contacts.EnsureCompanyIsPartner,
		// projects owns `project`, so the attach check and the delivery advance
		// live there and the edges are injected here for the same reason.
		EnsureProjectAttachable: projects.EnsureAttachable,
		StartDeliveryForWonDeal: projects.StartDeliveryForWonDeal,
		// approvals owns the pending proposals that follow a deal to its new
		// owner, so the hand-over lives there and the edge is injected here.
		FollowOwner: approvals.FollowDealOwnerInTx,
		// contracts owns the contract row rule, which stage evidence asks
		// before it calls a cited contract openable.
		ContractVisible: contracts.VisibleClause,
		// contacts owns the legal entities, so which one an author works for
		// and what an offer prints about it are read there.
		AuthorEntity: contacts.UserLegalEntity,
		IssuerEntity: dealsIssuerEntity,
	}
}

// dealsIssuerEntity resolves the entity an offer names, the default entity
// for none, in the shape deals prints from.
func dealsIssuerEntity(ctx context.Context, tx pgx.Tx, id *ids.LegalEntityID) (deals.IssuerEntity, error) {
	var entityID ids.LegalEntityID
	if id != nil {
		entityID = *id
	} else {
		var err error
		if entityID, err = contacts.DefaultLegalEntity(ctx, tx); err != nil {
			return deals.IssuerEntity{}, err
		}
	}
	e, live, err := contacts.ResolveLegalEntity(ctx, tx, entityID)
	if err != nil {
		return deals.IssuerEntity{}, err
	}
	text := func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}
	a := e.Address
	return deals.IssuerEntity{
		ID: e.ID, IsDefault: e.IsDefault, Live: live,
		DisplayName: e.DisplayName, LegalName: text(e.LegalName), Country: text(e.Country),
		Address: deals.IssuerAddress{
			Line1: text(a.Line1), Line2: text(a.Line2), PostalCode: text(a.PostalCode),
			City: text(a.City), Region: text(a.Region),
		},
		VatID: text(e.VatID), RegisterCourt: text(e.RegisterCourt), RegisterNumber: text(e.RegisterNumber),
		BankAccountHolder: text(e.BankAccountHolder), IBAN: text(e.IBAN), BIC: text(e.BIC),
		LogoKey: text(e.LogoObjectKey), DefaultLanguage: text(e.DefaultLanguage),
	}, nil
}

// installationCountry is the installation's country in the lower-case spelling
// a tax rate's country takes.
func installationCountry(ctx context.Context, tx pgx.Tx) (string, error) {
	code, err := identity.CountryOf(ctx, tx)
	return strings.ToLower(string(code)), err
}
