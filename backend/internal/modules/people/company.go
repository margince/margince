// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type CreateCompanyInput struct {
	DisplayName string
	LegalName   *string
	// Description is the one-line summary the company page shows under the
	// title; nil leaves the column NULL, which the page renders as absent.
	Description *string
	Industry    *string
	SizeBand    *string
	OwnerID     *ids.UserID
	ParentCompanyID *ids.CompanyID
	Address     *crmcontracts.Address
	Domains     []CompanyDomainInput
	Source      string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

func (s *Store) CreateCompany(ctx context.Context, in CreateCompanyInput) (crmcontracts.Company, error) {
	if err := auth.Require(ctx, "company", principal.ActionCreate); err != nil {
		return crmcontracts.Company{}, err
	}
	by, err := s.readyCompanyCreate(ctx, in)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	// The store-opened path reads the catalog through the unexported helper,
	// not ActiveCompanyColumns: that one takes company:read on the
	// caller's behalf, and a seat may hold create without it.
	active, err := s.activeColumns(ctx, "company")
	if err != nil {
		return crmcontracts.Company{}, err
	}

	var out crmcontracts.Company
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createCompanyInTx(ctx, tx, in, by, active)
		if err != nil {
			return err
		}
		return s.geocodeANewCompany(ctx, tx, out, in.Address)
	})
	return out, err
}

// CreateCompanyTx is CreateCompany for a caller that already opened a
// transaction — one whose own write must land with this company or not at
// all. Same gates in the same order; only the transaction is borrowed.
//
// Custom fields are refused rather than dropped: the catalog they are matched
// against is read in a transaction of its own, which is exactly the second
// connection this seam exists to avoid taking.
func (s *Store) CreateCompanyTx(ctx context.Context, tx pgx.Tx, in CreateCompanyInput) (crmcontracts.Company, error) {
	if err := auth.Require(ctx, "company", principal.ActionCreate); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := refuseCustomFields(in.CustomFields); err != nil {
		return crmcontracts.Company{}, err
	}
	by, err := s.readyCompanyCreate(ctx, in)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	out, err := createCompanyInTx(ctx, tx, in, by, nil)
	if err != nil {
		return out, err
	}
	return out, s.geocodeANewCompany(ctx, tx, out, in.Address)
}

// geocodeANewCompany queues the lookup a create earned.
//
// It sits on the two store entry points rather than inside
// createCompanyInTx because that one is a free function with no store —
// which is exactly why the enqueue was missed when the update path got it.
// Both doors call this, so neither can create a company that never asks where
// it is.
//
// A create with no usable address queues nothing: the row is simply not a
// place yet, and the update path will queue when it becomes one.
func (s *Store) geocodeANewCompany(ctx context.Context, tx pgx.Tx,
	out crmcontracts.Company, address *crmcontracts.Address,
) error {
	if !namesAPlace(address) {
		return nil
	}
	if err := s.enqueueGeocode(ctx, tx, ids.From[ids.CompanyKind](ids.UUID(out.Id))); err != nil {
		return fmt.Errorf("locating a new company: %w", err)
	}
	return nil
}

// readyCompanyCreate runs what a create settles BEFORE any transaction
// opens — the domain parse, the size-band vocabulary and the captured-by
// resolution — and answers the attribution the write shape stamps. Both entry
// points call it, so neither can drift from the other's validation.
func (s *Store) readyCompanyCreate(ctx context.Context, in CreateCompanyInput) (string, error) {
	if err := parseCompanyDomains(in.Domains); err != nil {
		return "", err
	}
	// Both write paths, not just the patch: a vocabulary checked on update and
	// not on create is a value the database refuses at birth and the transport
	// cannot name.
	if in.SizeBand != nil {
		if err := checkSizeBand(*in.SizeBand); err != nil {
			return "", err
		}
	}
	return storekit.CapturedBy(ctx)
}

// createCompanyInTx is CreateCompany's transactional body, shared by
// the store-opened and caller-opened entry points.
func createCompanyInTx(ctx context.Context, tx pgx.Tx, in CreateCompanyInput, by string,
	active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	if err := ensureCompanyDomainsUnclaimed(ctx, tx, in.Domains); err != nil {
		return crmcontracts.Company{}, err
	}

	match, err := manualDedupeCompany(ctx, tx, in)
	if err != nil {
		return crmcontracts.Company{}, err
	}

	// Naming a parent is a read of the parent: the child discloses the
	// hierarchy edge, so the target must be visible under the caller's
	// row scope, not merely same-workspace (H1 — an FK argument to a
	// row-scoped record is a read of that record).
	if in.ParentCompanyID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "company", in.ParentCompanyID.UUID); err != nil {
			return crmcontracts.Company{}, err
		}
	}

	id, err := createCompany(ctx, tx, match, CompanySpec{
		DisplayName:  in.DisplayName,
		LegalName:    in.LegalName,
		Description:  in.Description,
		Industry:     in.Industry,
		SizeBand:     in.SizeBand,
		OwnerID:      in.OwnerID,
		ParentCompanyID:  in.ParentCompanyID,
		Address:      in.Address,
		Domains:      in.Domains,
		Source:       in.Source,
		CapturedBy:   by,
		CustomFields: in.CustomFields,
		Active:       active,
	})
	if err != nil {
		return crmcontracts.Company{}, err
	}

	// A description supplied at create is authored the same way an edited one
	// is, and has to say so for the same reason: the site read asks
	// field_provenance whose sentence it is before replacing it, and a create
	// that stamped nothing would leave a person's own words unclaimed. `by` is
	// the authenticated principal, so an agent's create claims nothing.
	if in.Description != nil && *in.Description != "" {
		if err := stampDescriptionAuthor(ctx, tx, id, by); err != nil {
			return crmcontracts.Company{}, err
		}
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "company", id.UUID, nil, map[string]any{"display_name": in.DisplayName})
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("audit company create: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventCompanyCreated{DisplayName: &in.DisplayName}); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("emit company.created: %w", err)
	}
	if err := match.recordIfReview(ctx, tx, id, in.DisplayName, in.Source, by); err != nil {
		return crmcontracts.Company{}, err
	}
	out, err := readCompany(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read created company: %w", err)
	}
	return out, nil
}
