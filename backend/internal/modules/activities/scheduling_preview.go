// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"net/url"
	"strings"
)

// publicBookingURL is the address a host shares: /book/<slug> on the
// validated public origin. The web tier serves the app there to a browser and
// the booking preview to any other fetch (frontend/nginx.conf), so the path is
// what a link-preview fetcher sees. With no usable origin there is no address
// to give, and ok is false rather than a relative or invented URL.
//
// Only a host's reusable page lives on a path. Proposal and management links
// keep their token in the # fragment (scheduling_proposal.go,
// scheduling_secrets.go), which no request carries to a server.
func (s *Store) publicBookingURL(slug string) (string, bool) {
	if slug == "" || s.publicOriginUsable() != nil {
		return "", false
	}
	return strings.TrimRight(s.publicBaseURL, "/") + "/book/" + url.PathEscape(slug), true
}

// BookingPreview is what a shared booking link's card may say about the page:
// the host's public identity and meeting title, the same facts the public
// profile already shows a guest. It never carries calendar contents, guest
// details or a token.
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

// PublicBookingPreview reads a booking page's card through the same path the
// public profile endpoint uses: a missing or revoked slug is
// apperrors.ErrNotFound, and the host's identity comes from the account and
// legal-entity branding, never from the request.
func (h Handlers) PublicBookingPreview(ctx context.Context, slug string) (BookingPreview, error) {
	page, err := h.store.ResolveBookingPage(ctx, slug)
	if err != nil {
		return BookingPreview{}, err
	}
	profile, err := h.store.hostSchedulingProfile(ctx, page.HostUserID)
	if err != nil {
		return BookingPreview{}, err
	}
	if !profile.Enabled {
		return BookingPreview{}, nil
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
	out.PageURL, _ = h.store.publicBookingURL(slug)
	return out, nil
}
