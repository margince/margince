// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const bookingLogoPath = "/v1/public/booking/company-logo"

type anchorBrand struct {
	id      ids.CompanyID
	name    string
	logoKey *string
}

// PublicBookingBrand deliberately publishes only the anchor's name and logo.
func (s *Store) PublicBookingBrand(ctx context.Context) (string, *string, error) {
	brand, err := s.readAnchorBrand(ctx)
	if errors.Is(err, apperrors.ErrNotFound) {
		return "", nil, nil
	}
	if err != nil || brand.logoKey == nil || *brand.logoKey == "" {
		return brand.name, nil, err
	}
	path := bookingLogoPath
	return brand.name, &path, nil
}

func (s *Store) readAnchorBrand(ctx context.Context) (anchorBrand, error) {
	var brand anchorBrand
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		brand.id, err = anchorCompany(ctx, tx, false)
		if err != nil {
			return err
		}
		args := []any{brand.id}
		return tx.QueryRow(ctx, fmt.Sprintf(`SELECT display_name, logo_object_key FROM company WHERE id=$%d`, len(args)), args...).Scan(&brand.name, &brand.logoKey)
	})
	return brand, err
}

// GetPublicBookingCompanyLogo publishes the anchor mark without exposing other companies.
func (h Handlers) GetPublicBookingCompanyLogo(w http.ResponseWriter, r *http.Request) {
	brand, err := h.store.readAnchorBrand(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if brand.logoKey == nil || *brand.logoKey == "" {
		writeStoreErr(w, r, apperrors.ErrNotFound)
		return
	}
	h.streamLogoKey(w, r, crmcontracts.Id(brand.id.UUID), LogoWide, *brand.logoKey, "GetPublicBookingCompanyLogo", "no-cache", false)
}
