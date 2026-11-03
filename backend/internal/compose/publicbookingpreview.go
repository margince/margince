// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The link-preview card of a host's public booking page. The web tier sends a
// non-browser fetch of /book/<slug> to /v1/public/booking/<slug>/preview
// (frontend/nginx.conf), and the booking edge (publicbooking.go) answers it
// here: the same rate limits, the same slug resolution that refuses a missing
// or revoked page, and the same system:public_booking principal as the rest of
// the booking edge. The record-card principal is not involved.
//
// The card says what the public profile already shows a guest: host, company,
// meeting title and duration, and the company logo. It never names Margince,
// and it never carries guest details or a proposal or management token.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/modules/activities"
)

const publicBookingPreviewSuffix = "/preview"

// bookingPreviewSlug answers the slug when path is a booking page's preview.
func bookingPreviewSlug(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, publicBookingPrefix)
	if !ok {
		return "", false
	}
	slug, ok := strings.CutSuffix(rest, publicBookingPreviewSuffix)
	return slug, ok && slug != "" && !strings.Contains(slug, "/")
}

// GetPublicBookingPreview renders the card. The booking edge has already
// resolved the slug and bound the public booking principal.
func (s Server) GetPublicBookingPreview(w http.ResponseWriter, r *http.Request, hostSlug string) {
	preview, err := s.activitiesHandlers.PublicBookingPreview(r.Context(), hostSlug)
	if err != nil {
		s.writeBookingPreviewUnavailable(w, r, hostSlug, err)
		return
	}
	s.writePreviewCard(w, r, bookingCard(hostSlug, preview), "booking", nil)
}

// writeBookingPreviewUnavailable answers a missing, revoked, throttled or
// failed read with a card that names no host. Its status says which.
func (s Server) writeBookingPreviewUnavailable(w http.ResponseWriter, r *http.Request, hostSlug string, cause error) {
	s.writePreviewCard(w, r, previewCard{Title: "Booking page unavailable", Href: bookingAppHref(hostSlug), LinkText: "Open booking page"}, "booking", cause)
}

// bookingAppHref is where the card sends a browser: the app's legacy fragment
// address, never /book/<slug> itself, which would send a browser that sends
// no Sec-Fetch-Dest straight back to this card. The app then shows the
// /book/<slug> path in the address bar (frontend/src/lib/recordaddress.ts).
func bookingAppHref(hostSlug string) string {
	return "/#/book/" + hostSlug
}

func bookingCard(hostSlug string, preview activities.BookingPreview) previewCard {
	card := previewCard{Href: bookingAppHref(hostSlug), LinkText: "Open booking page", Canonical: preview.PageURL}
	if !preview.Available {
		// A paused page keeps its host's identity in the app, where the guest
		// reads why it is unavailable. A card shared onward names nobody.
		card.Title = "Booking unavailable"
		card.Summary = "This booking page is not taking new meetings right now."
		return card
	}
	card.Title = "Book a meeting with " + preview.HostName
	with := preview.HostName
	if preview.CompanyName != "" {
		with += " of " + preview.CompanyName
		card.SiteName = preview.CompanyName
	}
	card.Summary = fmt.Sprintf("%s: a %d-minute meeting with %s. Choose a time that suits you.", preview.Title, preview.DurationMinutes, with)
	if preview.LogoURL != "" {
		card.Image = preview.LogoURL
		card.ImageAlt = preview.CompanyName
	}
	return card
}
