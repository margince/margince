// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The parts of /me that other modules own: the installation's name and marks,
// and the deal list filters the caller is refused. The composition root injects both.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// InstallationBrand reads the anchor company's name and marks for the caller
// bound on the context, or nil when the installation has not described itself.
type InstallationBrand func(context.Context) (*crmcontracts.InstallationBrand, error)

// RefusedDealFilters names the deal list filters the caller bound on the
// context would be refused, asked of the deal list's own list resolution and clauses.
type RefusedDealFilters func(context.Context) ([]string, error)

// WithInstallationBrand binds the brand read /me carries. Without it /me omits
// the brand, and a client draws the product's own mark.
func (h Handlers) WithInstallationBrand(brand InstallationBrand) Handlers {
	h.installationBrand = brand
	return h
}

// WithRefusedDealFilters binds the read behind the snapshot's
// refused_deal_filters. Without it /me omits them.
func (h Handlers) WithRefusedDealFilters(refused RefusedDealFilters) Handlers {
	h.refusedDealFilters = refused
	return h
}

// me is /me's full answer: meResponse plus the parts other modules read. Both
// are read as the caller, so each answers for this caller's own grants.
func (h Handlers) me(ctx context.Context, id Identity) (crmcontracts.MeResponse, error) {
	resp := h.meResponse(ctx, id)
	asCaller := actorCtx(ctx, id)
	if h.refusedDealFilters != nil {
		refused, err := h.refusedDealFilters(asCaller)
		if err != nil {
			return crmcontracts.MeResponse{}, err
		}
		resp.Authorization.RefusedDealFilters = &refused
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
