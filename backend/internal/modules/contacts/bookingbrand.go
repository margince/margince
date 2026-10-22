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
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const bookingLogoPath = "/v1/public/booking/company-logo"

type anchorBrand struct {
	id      ids.CompanyID
	name    string
	logoKey *string
}

// PublicBookingBrand is the name and logo a host's public booking page shows:
// those of the legal entity the host works for. An entity with no logo shows
// its name alone, and never borrows another entity's.
func (s *Store) PublicBookingBrand(ctx context.Context, host ids.UserID) (string, *string, error) {
	var identity EntityIdentity
	err := s.tx(ctx, func(tx pgx.Tx) error {
		entity, err := UserLegalEntity(ctx, tx, host)
		if err != nil {
			return err
		}
		identity, err = ResolveEntityIdentity(ctx, tx, entity)
		return err
	})
	if err != nil || identity.LogoObjectKey == nil || *identity.LogoObjectKey == "" {
		return identity.DisplayName, nil, err
	}
	path := bookingLogoPath
	if !identity.IsDefault {
		path += "?entity=" + identity.ID.String()
	}
	return identity.DisplayName, &path, nil
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

// GetPublicBookingCompanyLogo publishes the logo a booking page links to: a
// live non-default entity's when one is named, and the anchor's otherwise.
// It publishes no other company's.
func (h Handlers) GetPublicBookingCompanyLogo(w http.ResponseWriter, r *http.Request, params crmcontracts.GetPublicBookingCompanyLogoParams) {
	if params.Entity != nil {
		key, err := h.store.publicEntityLogoKey(r.Context(), ids.From[ids.LegalEntityKind](ids.UUID(*params.Entity)))
		if err != nil {
			writeStoreErr(w, r, err)
			return
		}
		h.streamLogoKey(w, r, crmcontracts.Id(*params.Entity), LogoWide, key, "GetPublicBookingCompanyLogo", "no-cache", false)
		return
	}
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

// publicEntityLogoKey is the logo key of a live, non-default entity. The
// default entity wears the anchor's logo, which the unparameterised link serves.
func (s *Store) publicEntityLogoKey(ctx context.Context, id ids.LegalEntityID) (string, error) {
	var key *string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args legalEntityArgs
		return tx.QueryRow(ctx, `SELECT logo_object_key FROM legal_entity
		  WHERE id = `+args.add(id)+` AND NOT is_default AND archived_at IS NULL`, args...).Scan(&key)
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (key == nil || *key == "")) {
		return "", apperrors.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read a public legal entity logo: %w", err)
	}
	return *key, nil
}

// AnchorLogoKey is the object key of the workspace's own logo, empty when it
// has none. The send path embeds that logo in a signature.
func (s *Store) AnchorLogoKey(ctx context.Context) (string, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return "", err
	}
	brand, err := s.readAnchorBrand(ctx)
	if errors.Is(err, apperrors.ErrNotFound) {
		return "", nil
	}
	if err != nil || brand.logoKey == nil {
		return "", err
	}
	return *brand.logoKey, nil
}
