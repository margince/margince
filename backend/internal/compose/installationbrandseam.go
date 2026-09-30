// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The edge from identity's /me to contacts, which owns the anchor company whose
// name and marks every seat's brand block draws.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
)

func installationBrand(store *contacts.Store) identity.InstallationBrand {
	return func(ctx context.Context) (*crmcontracts.InstallationBrand, error) {
		brand, found, err := store.InstallationBrand(ctx)
		if err != nil || !found {
			return nil, err
		}
		return &crmcontracts.InstallationBrand{
			DisplayName: brand.DisplayName,
			LogoUrl:     brand.LogoURL,
			LogoIconUrl: brand.LogoIconURL,
		}, nil
	}
}
