// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The card of a host's public booking page. nginx sends every fetch of
// /book/<slug> here, a browser's navigation included, so the head any reader
// gets before a script runs is this one. The booking edge (publicbooking.go)
// applies its rate limits, refuses a missing or revoked slug, and binds the
// system:public_booking principal before this file renders anything.
// The card never names Margince, guest details or a token.

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose/readerlocale"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
func (s Server) GetPublicBookingPreview(w http.ResponseWriter, r *http.Request, hostSlug string, params crmcontracts.GetPublicBookingPreviewParams) {
	preview, err := s.PublicBookingPreview(systemtext.WithLocale(r.Context(), readerlocale.Anonymous(r)), hostSlug)
	if err != nil {
		s.writeBookingPreviewUnavailable(w, r, hostSlug, err)
		return
	}
	s.writePreviewCard(w, r, bookingCard(hostSlug, params.L, preview, readerlocale.Anonymous(r)), "booking", nil)
}

// writeBookingPreviewUnavailable answers a missing, revoked, throttled or
// failed read with a card that names no host. Its status says which.
func (s Server) writeBookingPreviewUnavailable(w http.ResponseWriter, r *http.Request, hostSlug string, cause error) {
	s.writePreviewCard(w, r, previewCard{Title: systemtext.Copy("preview.page_unavailable", readerlocale.Anonymous(r)), Href: bookingAppHref(hostSlug, linkLength(r.URL.Query())), LinkText: systemtext.Copy("preview.open_booking", readerlocale.Anonymous(r))}, "booking", cause)
}

// linkLength is the meeting length a shared link asks for, nil when it names
// no positive whole number of minutes.
func linkLength(query url.Values) *int {
	if length, err := strconv.Atoi(query.Get("l")); err == nil && length > 0 {
		return &length
	}
	return nil
}

// bookingAppHref is where the card sends a browser: the fragment address, with
// the meeting length the shared link asked for. /book/<slug> itself would
// loop back to this card. The app then shows the /book/<slug> path itself.
func bookingAppHref(hostSlug string, length *int) string {
	href := "/#/book/" + hostSlug
	if length != nil {
		href += "?l=" + strconv.Itoa(*length)
	}
	return href
}

// bookingCard resolves the length the link asks for once, so the summary and
// the href name the length the booking page opens at.
func bookingCard(hostSlug string, requested *int, preview activities.BookingPreview, locales ...textlang.Lang) previewCard {
	language := textlang.English
	if len(locales) > 0 {
		language = locales[0]
	}
	preview, offered := preview.ForLength(requested)
	if !offered {
		requested = nil
	}
	card := previewCard{Href: bookingAppHref(hostSlug, requested), LinkText: systemtext.Copy("preview.open_booking", language), Canonical: preview.PageURL}
	// A card per length, so a chat app never reuses one length's card for another.
	if requested != nil && preview.PageURL != "" {
		card.Canonical += "?l=" + strconv.Itoa(*requested)
	}
	if !preview.Available {
		// A paused page keeps its host's identity in the app, where the guest
		// reads why it is unavailable. A card shared onward names nobody.
		card.Title = systemtext.Copy("preview.booking_unavailable", language)
		card.Summary = systemtext.Copy("preview.booking_paused", language)
		return card
	}
	card.Title = fmt.Sprintf(systemtext.Copy("preview.book_with", language), preview.HostName)
	card.Summary = fmt.Sprintf(systemtext.Copy("preview.book_summary", language), preview.Title, preview.DurationMinutes, preview.HostName)
	if preview.CompanyName != "" {
		card.SiteName = preview.CompanyName
		card.Summary = fmt.Sprintf(systemtext.Copy("preview.book_company_summary", language), preview.Title, preview.DurationMinutes, preview.HostName, preview.CompanyName)
	}

	if preview.LogoURL != "" {
		card.Image = preview.LogoURL
		card.ImageAlt = preview.CompanyName
		card.Icon = preview.LogoURL
	}
	return card
}
