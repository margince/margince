// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
	bare := render(bookingCard("slug", nil, activities.BookingPreview{Available: true, HostName: "Ada", Title: "Intro", DurationMinutes: 30}))
	if !strings.Contains(bare, "Intro: a 30-minute meeting with Ada. Choose a time") || !strings.Contains(bare, `<link rel="icon" href="data:,">`) {
		t.Fatal(bare)
	}
	for _, absent := range []string{"og:site_name", "og:image", "canonical", "og:url", "Margince", "icon.svg"} {
		if strings.Contains(bare, absent) {
			t.Fatalf("card without company, logo or origin carries %s:\n%s", absent, bare)
		}
	}
	paused := render(bookingCard("slug", nil, activities.BookingPreview{PageURL: "https://crm.example.test/book/slug", CompanyName: "Acme", LogoURL: "https://crm.example.test/v1/public/booking/company-logo"}))
	if !strings.Contains(paused, "<title>Booking unavailable</title>") || !strings.Contains(paused, `<link rel="icon" href="data:,">`) {
		t.Fatal(paused)
	}
	for _, absent := range []string{"Margince", "Acme", "company-logo"} {
		if strings.Contains(paused, absent) {
			t.Fatalf("paused card carries %s:\n%s", absent, paused)
		}
	}
	// The record card keeps its Margince branding.
	record := render(recordPreviewCard(recordPreview{Title: "Deal", Summary: "Deal · Acme", Href: "/#/deals/x"}))
	if !strings.Contains(record, `<meta property="og:site_name" content="Margince">`) || !strings.Contains(record, "Open in Margince") || !strings.Contains(record, `<link rel="icon" href="/icon.svg">`) {
		t.Fatal(record)
	}
}

func TestBookingCardWearsTheCompanyLogoAsItsTabIcon(t *testing.T) {
	logo := "https://crm.example.test/v1/public/booking/company-logo?entity=01a00000-0000-7000-8000-000000000001"
	var body strings.Builder
	card := bookingCard("slug", nil, activities.BookingPreview{Available: true, HostName: "Ada", CompanyName: "Acme", Title: "Intro", DurationMinutes: 30, LogoURL: logo})
	if err := previewCardHTML.Execute(&body, card); err != nil {
		t.Fatal(err)
	}
	escaped := strings.ReplaceAll(logo, "&", "&amp;")
	for _, want := range []string{`<link rel="icon" href="` + escaped + `">`, `<meta property="og:image" content="` + escaped + `">`, `<meta property="og:site_name" content="Acme">`} {
		if !strings.Contains(body.String(), want) {
			t.Fatalf("card lacks %s:\n%s", want, body.String())
		}
	}
}

func TestBookingCardSendsABrowserOnWithTheMeetingLengthItAskedFor(t *testing.T) {
	for query, want := range map[string]string{
		"":                  "/#/book/slug",
		"l=30":              "/#/book/slug?l=30",
		"l=30&locale=de":    "/#/book/slug?l=30",
		"l=0":               "/#/book/slug",
		"l=-15":             "/#/book/slug",
		"l=30%22%3E%3Cx%3E": "/#/book/slug",
		"utm_source=mail":   "/#/book/slug",
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if got := bookingAppHref("slug", linkLength(values)); got != want {
			t.Errorf("?%s: %q, want %q", query, got, want)
		}
	}
}

func TestBookingCardNamesTheLengthTheBookingPageOpensAt(t *testing.T) {
	preview := activities.BookingPreview{Available: true, HostName: "Ada", Title: "Intro", DurationMinutes: 30, Lengths: []int{30, 15, 45}}
	for _, test := range []struct {
		requested *int
		summary   string
		href      string
	}{
		{new(15), "Intro: a 15-minute meeting with Ada.", "/#/book/slug?l=15"},
		{new(7), "Intro: a 30-minute meeting with Ada.", "/#/book/slug"},
		{nil, "Intro: a 30-minute meeting with Ada.", "/#/book/slug"},
	} {
		card := bookingCard("slug", test.requested, preview)
		if !strings.HasPrefix(card.Summary, test.summary) || card.Href != test.href {
			t.Errorf("requested %v: %q at %q, want %q at %q", test.requested, card.Summary, card.Href, test.summary, test.href)
		}
	}
}
