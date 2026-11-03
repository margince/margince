// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestBookingPreviewPathNamesExactlyOneSlug(t *testing.T) {
	for path, want := range map[string]string{
		"/v1/public/booking/Ab3_x-9Q/preview":        "Ab3_x-9Q",
		"/v1/public/booking/Ab3_x-9Q/preview/":       "",
		"/v1/public/booking/a/b/preview":             "",
		"/v1/public/booking//preview":                "",
		"/v1/public/booking/Ab3_x-9Q/profile":        "",
		"/v1/public/booking/Ab3_x-9Q/previewer":      "",
		"/v1/public/meeting/Ab3_x-9Q/preview":        "",
		"/v1/public/booking/company-logo":            "",
		"/v1/public/record-previews/deals/x/preview": "",
	} {
		slug, ok := bookingPreviewSlug(path)
		if (want == "") == ok || slug != want && ok {
			t.Errorf("%s: %q %v, want %q", path, slug, ok, want)
		}
	}
}

func TestBookingPreviewRefusesWritesAndEncodedPathsBeforeResolvingTheSlug(t *testing.T) {
	handler := publicBooking(nil, nil, newPublicBookingLimiters(), Server{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("preview reached the router")
	}))
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/v1/public/booking/Ab3_x-9Q/preview", nil),
		httptest.NewRequest(http.MethodPut, "/v1/public/booking/Ab3_x-9Q/preview", nil),
		httptest.NewRequest(http.MethodGet, "/v1/public/booking/Ab%33_x-9Q/preview", nil),
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s %s: %d", request.Method, request.URL, response.Code)
		}
	}
}

func TestBookingCardOmitsWhatItCannotStateTruthfully(t *testing.T) {
	render := func(card previewCard) string {
		var body strings.Builder
		if err := previewCardHTML.Execute(&body, card); err != nil {
			t.Fatal(err)
		}
		return body.String()
	}
	bare := render(bookingCard("slug", activities.BookingPreview{Available: true, HostName: "Ada", Title: "Intro", DurationMinutes: 30}))
	if !strings.Contains(bare, "Intro: a 30-minute meeting with Ada. Choose a time") {
		t.Fatal(bare)
	}
	for _, absent := range []string{"og:site_name", "og:image", "canonical", "og:url", "Margince"} {
		if strings.Contains(bare, absent) {
			t.Fatalf("card without company, logo or origin carries %s:\n%s", absent, bare)
		}
	}
	paused := render(bookingCard("slug", activities.BookingPreview{PageURL: "https://crm.example.test/book/slug"}))
	if !strings.Contains(paused, "<title>Booking unavailable</title>") || strings.Contains(paused, "Margince") {
		t.Fatal(paused)
	}
	// The record card keeps its Margince branding.
	record := render(recordPreviewCard(recordPreview{Title: "Deal", Summary: "Deal · Acme", Href: "/#/deals/x"}))
	if !strings.Contains(record, `<meta property="og:site_name" content="Margince">`) || !strings.Contains(record, "Open in Margince") {
		t.Fatal(record)
	}
}
