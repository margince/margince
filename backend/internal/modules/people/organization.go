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

type CreateOrganizationInput struct {
	DisplayName string
	LegalName   *string
	// Description is the one-line summary the company page shows under the
	// title; nil leaves the column NULL, which the page renders as absent.
	Description *string
	Industry    *string
	SizeBand    *string
	OwnerID     *ids.UserID
	ParentOrgID *ids.OrganizationID
	Address     *crmcontracts.Address
	Domains     []OrgDomainInput
	Source      string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

func (s *Store) CreateOrganization(ctx context.Context, in CreateOrganizationInput) (crmcontracts.Organization, error) {
	if err := auth.Require(ctx, "organization", principal.ActionCreate); err != nil {
		return crmcontracts.Organization{}, err
	}
	by, err := s.readyOrganizationCreate(ctx, in)
	if err != nil {
		return crmcontracts.Organization{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	// The store-opened path reads the catalog through the unexported helper,
	// not ActiveOrganizationColumns: that one takes organization:read on the
	// caller's behalf, and a seat may hold create without it.
	active, err := s.activeColumns(ctx, "organization")
	if err != nil {
		return crmcontracts.Organization{}, err
	}

	var out crmcontracts.Organization
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createOrganizationInTx(ctx, tx, in, by, active)
		if err != nil {
			return err
		}
		return s.geocodeANewCompany(ctx, tx, out, in.Address)
	})
	return out, err
}

// CreateOrganizationTx is CreateOrganization for a caller that already opened a
// transaction — one whose own write must land with this organization or not at
// all. Same gates in the same order; only the transaction is borrowed.
//
// Custom fields are refused rather than dropped: the catalog they are matched
// against is read in a transaction of its own, which is exactly the second
// connection this seam exists to avoid taking.
func (s *Store) CreateOrganizationTx(ctx context.Context, tx pgx.Tx, in CreateOrganizationInput) (crmcontracts.Organization, error) {
	if err := auth.Require(ctx, "organization", principal.ActionCreate); err != nil {
		return crmcontracts.Organization{}, err
	}
	if err := refuseCustomFields(in.CustomFields); err != nil {
		return crmcontracts.Organization{}, err
	}
	by, err := s.readyOrganizationCreate(ctx, in)
	if err != nil {
		return crmcontracts.Organization{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	out, err := createOrganizationInTx(ctx, tx, in, by, nil)
	if err != nil {
		return out, err
	}
	return out, s.geocodeANewCompany(ctx, tx, out, in.Address)
}

// geocodeANewCompany queues the lookup a create earned.
//
// It sits on the two store entry points rather than inside
// createOrganizationInTx because that one is a free function with no store —
// which is exactly why the enqueue was missed when the update path got it.
// Both doors call this, so neither can create a company that never asks where
// it is.
//
// A create with no usable address queues nothing: the row is simply not a
// place yet, and the update path will queue when it becomes one.
func (s *Store) geocodeANewCompany(ctx context.Context, tx pgx.Tx,
	out crmcontracts.Organization, address *crmcontracts.Address,
) error {
	if !namesAPlace(address) {
		return nil
	}
	if err := s.enqueueGeocode(ctx, tx, ids.From[ids.OrganizationKind](ids.UUID(out.Id))); err != nil {
		return fmt.Errorf("locating a new company: %w", err)
	}
	return nil
}

// readyOrganizationCreate runs what a create settles BEFORE any transaction
// opens — the domain parse, the size-band vocabulary and the captured-by
// resolution — and answers the attribution the write shape stamps. Both entry
// points call it, so neither can drift from the other's validation.
func (s *Store) readyOrganizationCreate(ctx context.Context, in CreateOrganizationInput) (string, error) {
	if err := parseOrgDomains(in.Domains); err != nil {
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

// createOrganizationInTx is CreateOrganization's transactional body, shared by
// the store-opened and caller-opened entry points.
func createOrganizationInTx(ctx context.Context, tx pgx.Tx, in CreateOrganizationInput, by string,
	active []fieldcatalog.Column,
) (crmcontracts.Organization, error) {
	if err := ensureOrgDomainsUnclaimed(ctx, tx, in.Domains); err != nil {
		return crmcontracts.Organization{}, err
	}

	match, err := manualDedupeOrganization(ctx, tx, in)
	if err != nil {
		return crmcontracts.Organization{}, err
	}

	// Naming a parent is a read of the parent: the child discloses the
	// hierarchy edge, so the target must be visible under the caller's
	// row scope, not merely same-workspace (H1 — an FK argument to a
	// row-scoped record is a read of that record).
	if in.ParentOrgID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, "organization", in.ParentOrgID.UUID); err != nil {
			return crmcontracts.Organization{}, err
		}
	}

	id, err := createOrganization(ctx, tx, match, OrgSpec{
		DisplayName:  in.DisplayName,
		LegalName:    in.LegalName,
		Description:  in.Description,
		Industry:     in.Industry,
		SizeBand:     in.SizeBand,
		OwnerID:      in.OwnerID,
		ParentOrgID:  in.ParentOrgID,
		Address:      in.Address,
		Domains:      in.Domains,
		Source:       in.Source,
		CapturedBy:   by,
		CustomFields: in.CustomFields,
		Active:       active,
	})
	if err != nil {
		return crmcontracts.Organization{}, err
	}

	// The people already on this company's domains get their employment edge
	// now. They accumulated while nobody had a company for the domain: capture
	// creates the person and deliberately leaves the employer undecided, so by
	// the time a human records the company its whole roster is sitting there
	// attached to nothing. Without this the account shows one contact —
	// whichever sender writes next and earns an edge from their own ensure —
	// and the health card blames the whole relationship on that one person.
	//
	// The same plant the domain-triage verdict runs, so the human path and the
	// machine path wire the same backlog. It never reassigns anybody a human
	// already placed. The triage path reaches createOrganization directly and
	// plants for itself, so nothing here plants twice.
	for _, domain := range in.Domains {
		if _, err := plantDomainEmployment(ctx, tx, domain.Domain, id); err != nil {
			return crmcontracts.Organization{}, fmt.Errorf("attach the domain's people to the new company: %w", err)
		}
	}

	// A description supplied at create is authored the same way an edited one
	// is, and has to say so for the same reason: the site read asks
	// field_provenance whose sentence it is before replacing it, and a create
	// that stamped nothing would leave a person's own words unclaimed. `by` is
	// the authenticated principal, so an agent's create claims nothing.
	if in.Description != nil && *in.Description != "" {
		if err := stampDescriptionAuthor(ctx, tx, id, by); err != nil {
			return crmcontracts.Organization{}, err
		}
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "organization", id.UUID, nil, map[string]any{"display_name": in.DisplayName})
	if err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("audit organization create: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventOrganizationCreated{DisplayName: &in.DisplayName}); err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("emit organization.created: %w", err)
	}
	if err := match.recordIfReview(ctx, tx, id, in.DisplayName, in.Source, by); err != nil {
		return crmcontracts.Organization{}, err
	}
	out, err := readOrganization(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Organization{}, fmt.Errorf("read created organization: %w", err)
	}
	return out, nil
}
