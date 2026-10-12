// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"log/slog"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// SchedulingBrand exposes only the installation's public company identity.
type SchedulingBrand interface {
	PublicBookingBrand(context.Context) (string, *string, error)
}

// WithSchedulingBrand binds installation branding without importing its owner.
func (s *Store) WithSchedulingBrand(brand SchedulingBrand) *Store {
	clone := *s
	clone.schedulingBrand = brand
	return &clone
}

// WithSchedulingBrand gives HTTP profiles the same identity as the tool surface.
func (h Handlers) WithSchedulingBrand(brand SchedulingBrand) Handlers {
	h.store = h.store.WithSchedulingBrand(brand)
	return h
}

func (s *Store) brandSchedulingProfile(ctx context.Context, profile crmcontracts.SchedulingProfile) crmcontracts.SchedulingProfile {
	profile.CompanyName, profile.LogoUrl = nil, nil
	if s.schedulingBrand == nil {
		return profile
	}
	name, logo, err := s.schedulingBrand.PublicBookingBrand(ctx)
	if err != nil {
		slog.WarnContext(ctx, "booking company branding unavailable", "err", err)
		return profile
	}
	name = schedulingDisplayName(name)
	if logo != nil && s.publicOriginUsable() == nil {
		absolute := strings.TrimRight(s.publicBaseURL, "/") + *logo
		logo = &absolute
	}
	profile.CompanyName, profile.LogoUrl = &name, logo
	return profile
}

// Public profiles keep their contract bound while account and company names may be longer.
func schedulingDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 200 {
		return textcut.Runes(name, 199) + "…"
	}
	return name
}
