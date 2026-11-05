// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/url"
	"strings"
)

// publicBookingURL is the address a host shares: /book/<slug> on the
// validated public origin, which nginx answers with the app or the preview.
// With no usable origin it answers false, never a relative or invented URL.
// Proposal and management tokens stay in the # fragment, never on a path.
func (s *Store) publicBookingURL(slug string) (string, bool) {
	if slug == "" || s.publicOriginUsable() != nil {
		return "", false
	}
	return strings.TrimRight(s.publicBaseURL, "/") + "/book/" + url.PathEscape(slug), true
}

// BookingPreview is what a shared booking link's card may say. It holds the
// facts the public profile already shows a guest. It never holds calendar
// contents, guest details or a token.
type BookingPreview struct {
	// Available is false for a paused page. The card then names no host.
	Available       bool
	HostName        string
	CompanyName     string
	Title           string
	DurationMinutes int
	// LogoURL and PageURL are absolute on the validated public origin, or
	// empty when the company has no logo or no usable origin is configured.
	LogoURL string
	PageURL string
}

// PublicBookingPreview reads the card the way the public profile is read.
// A missing or revoked slug is apperrors.ErrNotFound. The host's identity
// comes from the account and legal-entity branding, never from the request.
func (h Handlers) PublicBookingPreview(ctx context.Context, slug string) (BookingPreview, error) {
	page, err := h.store.ResolveBookingPage(ctx, slug)
	if err != nil {
		return BookingPreview{}, err
	}
	profile, err := h.store.hostSchedulingProfile(ctx, page.HostUserID)
	if err != nil {
		return BookingPreview{}, err
	}
	paused, err := h.store.pagePaused(ctx, page.HostUserID, profile)
	if err != nil || paused {
		return BookingPreview{}, err
	}
	profile = h.store.brandSchedulingProfile(ctx, page.HostUserID, profile)
	out := BookingPreview{Available: true, Title: profile.Title, DurationMinutes: profile.DurationMinutes}
	if profile.HostName != nil {
		out.HostName = *profile.HostName
	}
	if profile.CompanyName != nil {
		out.CompanyName = *profile.CompanyName
	}
	// brandSchedulingProfile makes the logo absolute only on a usable origin;
	// a relative one would be a broken image on every social card.
	if profile.LogoUrl != nil && h.store.publicOriginUsable() == nil {
		out.LogoURL = *profile.LogoUrl
	}
	// The host's shared address, the alias when there is one, whichever
	// address this fetch came in on.
	if profile.PublicUrl != nil {
		out.PageURL = *profile.PublicUrl
	}
	return out, nil
}
