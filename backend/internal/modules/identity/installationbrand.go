// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The installation's own name and marks on /me. The anchor company is contacts'
// row, which identity never imports, so the composition root injects the read.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// InstallationBrand reads the anchor company's name and marks for the caller
// bound on the context, or nil when the installation has not described itself.
type InstallationBrand func(context.Context) (*crmcontracts.InstallationBrand, error)

// WithInstallationBrand binds the brand read /me carries. Without it /me omits
// the brand, and a client draws the product's own mark.
func (h Handlers) WithInstallationBrand(brand InstallationBrand) Handlers {
	h.installationBrand = brand
	return h
}

// me is /me's full answer: meResponse plus the one part of it that reads the
// database. The brand is read as the caller, so a URL is only handed to a
// reader the logo stream would serve.
func (h Handlers) me(ctx context.Context, id Identity) (crmcontracts.MeResponse, error) {
	resp := h.meResponse(ctx, id)
	if h.installationBrand == nil {
		return resp, nil
	}
	brand, err := h.installationBrand(actorCtx(ctx, id))
	if err != nil {
		return crmcontracts.MeResponse{}, err
	}
	resp.InstallationBrand = brand
	return resp, nil
}
