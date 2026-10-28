// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The parts of /me that other modules own: the installation's name and marks, and the
// deal and lead list filters the caller is refused. The composition root injects them.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// InstallationBrand reads the anchor company's name and marks for the caller
// bound on the context, or nil when the installation has not described itself.
type InstallationBrand func(context.Context) (*crmcontracts.InstallationBrand, error)

// RefusedFilters names the filters of one record list that the caller bound on
// the context would be refused. It asks that list's own resolution and clauses.
type RefusedFilters func(context.Context) ([]string, error)

// WithInstallationBrand binds the brand read /me carries. Without it /me omits
// the brand, and a client draws the product's own mark.
func (h Handlers) WithInstallationBrand(brand InstallationBrand) Handlers {
	h.installationBrand = brand
	return h
}

// WithRefusedDealFilters binds the read behind the snapshot's
// refused_deal_filters. Without it /me omits them.
func (h Handlers) WithRefusedDealFilters(refused RefusedFilters) Handlers {
	h.refusedDealFilters = refused
	return h
}

// WithRefusedLeadFilters binds the read behind the snapshot's
// refused_lead_filters. Without it /me omits them.
func (h Handlers) WithRefusedLeadFilters(refused RefusedFilters) Handlers {
	h.refusedLeadFilters = refused
	return h
}

// me is /me's full answer: meResponse plus the parts other modules read. Both
// are read as the caller, so each answers for this caller's own grants.
func (h Handlers) me(ctx context.Context, id Identity) (crmcontracts.MeResponse, error) {
	resp := h.meResponse(ctx, id)
	asCaller := actorCtx(ctx, id)
	var err error
	if resp.Authorization.RefusedDealFilters, err = refusedFilters(asCaller, h.refusedDealFilters); err != nil {
		return crmcontracts.MeResponse{}, err
	}
	if resp.Authorization.RefusedLeadFilters, err = refusedFilters(asCaller, h.refusedLeadFilters); err != nil {
		return crmcontracts.MeResponse{}, err
	}
	if h.installationBrand == nil {
		return resp, nil
	}
	brand, err := h.installationBrand(asCaller)
	if err != nil {
		return crmcontracts.MeResponse{}, err
	}
	resp.InstallationBrand = brand
	return resp, nil
}

// refusedFilters asks one list which of its filters the caller is refused; nil
// when that list is not bound, so /me omits it.
func refusedFilters(asCaller context.Context, refused RefusedFilters) (*[]string, error) {
	if refused == nil {
		return nil, nil //nolint:nilnil // an unbound list has no answer to send, which /me says by omitting it
	}
	names, err := refused(asCaller)
	if err != nil {
		return nil, err
	}
	return &names, nil
}
