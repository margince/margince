// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The installation's own company as every seat sees it on screen: its name and
// its marks, which the app's brand block draws above every page.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Brand is the anchor company's name and the URLs of its two marks. A nil URL
// is a slot the company wears no mark in, or one this reader could not load.
type Brand struct {
	DisplayName string
	LogoURL     *string
	LogoIconURL *string
}

// InstallationBrand reads the anchor company's name and marks for any seated
// member, and reports false when the installation has not described itself.
//
// This is the one read of the anchor that requireAnchorAdministrator does not
// hold. The name and the marks are what every seat already sees in the rail,
// so admitting every member to them discloses nothing; every other profile
// field stays behind GetAnchorCompany. A URL is only handed out when
// requireLogoReadable would stream it to this reader, so the rail never draws
// an image the server then refuses.
func (s *Store) InstallationBrand(ctx context.Context) (Brand, bool, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return Brand{}, false, err
	}
	var out Brand
	found := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		companyID, err := anchorCompany(ctx, tx, false)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var wide, icon *string
		if err := tx.QueryRow(ctx,
			`SELECT display_name, logo_object_key, logo_icon_object_key FROM company WHERE id = $1`,
			companyID).Scan(&out.DisplayName, &wide, &icon); err != nil {
			return fmt.Errorf("read the installation's brand: %w", err)
		}
		found = true
		err = requireLogoReadable(ctx, tx, companyID)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		out.LogoURL = LogoURL(companyID.UUID, wide, LogoWide)
		out.LogoIconURL = LogoURL(companyID.UUID, icon, LogoIcon)
		return nil
	})
	if err != nil {
		return Brand{}, false, err
	}
	return out, found, nil
}
