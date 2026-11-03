// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

const previewOrigin = "https://crm.example.test"

// A host whose display name and meeting title carry markup, so every card
// below also proves the escaping.
func hostWithBookingPage(t *testing.T, e *integration.Env, enabled bool) string {
	t.Helper()
	e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, e.AdminUser, `Ada <script>alert("Host")</script>`)
	store := e.Activities.WithPublicBaseURL(previewOrigin).WithSchedulingCalendar(privateCalendar{}).WithSchedulingBrand(e.Contacts)
	profile, err := store.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{
		Enabled: enabled, Provider: "gcal", CalendarId: "primary", DurationMinutes: 45, NoticeMinutes: 120, HorizonDays: 30,
		Title: `Tea & <script>alert(1)</script>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Slug == nil || profile.PublicUrl == nil || *profile.PublicUrl != previewOrigin+"/book/"+*profile.Slug {
		t.Fatalf("the shared address is not the /book/ path: %+v", profile.PublicUrl)
	}
	return *profile.Slug
}

func fetchBookingPreview(t *testing.T, server http.Handler, method, slug string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(method, "/v1/public/booking/"+slug+"/preview", nil))
	return response
}

func expectNoMarginceOrHost(t *testing.T, body string) {
	t.Helper()
	for _, banned := range []string{"Margince", "margince.com", "Ada", "Acme", "og:image"} {
		if strings.Contains(body, banned) {
			t.Fatalf("card carries %q:\n%s", banned, body)
		}
	}
}

func TestBookingPreviewNamesTheHostAndCompanyWithTheirLogoAndNoMargince(t *testing.T) {
	e := integration.Setup(t)
	blob := blobstore.NewMemory()
	theCompanyExists(t, e)
	uploadMark(t, e, companyHandlers{store: e.Contacts, blob: blob}, logoFixture(t, 400, 100), "logo.png")
	slug := hostWithBookingPage(t, e, true)
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), withConnectedCalendar(), WithBlobstore(blob), WithPublicBaseURL(previewOrigin))

	response := fetchBookingPreview(t, server, http.MethodGet, slug)
	body := response.Body.String()
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("preview: %d %s", response.Code, body)
	}
	if response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Header().Get("X-Robots-Tag"), "noindex") {
		t.Fatalf("preview may be cached or indexed: %v", response.Header())
	}
	page := previewOrigin + "/book/" + slug
	for _, want := range []string{
		`<title>Book a meeting with Ada &lt;script&gt;alert(&#34;Host&#34;)&lt;/script&gt;</title>`,
		`<link rel="canonical" href="` + page + `">`,
		`<meta property="og:url" content="` + page + `">`,
		`<meta property="og:site_name" content="Acme GmbH">`,
		`<meta property="og:image" content="` + previewOrigin + `/v1/public/booking/company-logo">`,
		`<meta name="twitter:image" content="` + previewOrigin + `/v1/public/booking/company-logo">`,
		`<meta name="twitter:card" content="summary">`,
		`Tea &amp; &lt;script&gt;alert(1)&lt;/script&gt;: a 45-minute meeting with Ada &lt;script&gt;alert(&#34;Host&#34;)&lt;/script&gt; of Acme GmbH.`,
		`<a id="record" href="/#/book/` + slug + `">Open booking page</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview lacks %s:\n%s", want, body)
		}
	}
	for _, banned := range []string{"Margince", "margince.com", "<script>alert", `"Host"`} {
		if strings.Contains(body, banned) {
			t.Fatalf("preview carries %q:\n%s", banned, body)
		}
	}

	head := fetchBookingPreview(t, server, http.MethodHead, slug)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("HEAD: %d %q", head.Code, head.Body.String())
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		if refused := fetchBookingPreview(t, server, method, slug); refused.Code != http.StatusNotFound {
			t.Fatalf("%s admitted: %d", method, refused.Code)
		}
	}

	// A renamed company and a removed logo show on the next fetch.
	if _, err := e.Contacts.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Renamed GmbH"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ClearCompanyLogo(e.Admin(), contacts.LogoWide); err != nil {
		t.Fatal(err)
	}
	after := fetchBookingPreview(t, server, http.MethodGet, slug).Body.String()
	if !strings.Contains(after, `content="Renamed GmbH"`) || strings.Contains(after, "Acme") || strings.Contains(after, "og:image") {
		t.Fatalf("preview did not follow the company change:\n%s", after)
	}
}

func TestBookingPreviewWithoutALogoOrAnOriginIsATextCardWithNoURLs(t *testing.T) {
	e := integration.Setup(t)
	theCompanyExists(t, e)
	slug := hostWithBookingPage(t, e, true)
	// The page was published on an origin; this server has none configured.
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), withConnectedCalendar())
	response := fetchBookingPreview(t, server, http.MethodGet, slug)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "of Acme GmbH.") || !strings.Contains(body, `<meta name="twitter:card" content="summary">`) {
		t.Fatalf("preview: %d %s", response.Code, body)
	}
	for _, banned := range []string{"og:image", "twitter:image", "canonical", "og:url", "Margince", "https://"} {
		if strings.Contains(body, banned) {
			t.Fatalf("text card carries %q:\n%s", banned, body)
		}
	}
}

func TestPausedBookingPreviewNamesNobody(t *testing.T) {
	e := integration.Setup(t)
	theCompanyExists(t, e)
	slug := hostWithBookingPage(t, e, false)
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), withConnectedCalendar(), WithPublicBaseURL(previewOrigin))
	response := fetchBookingPreview(t, server, http.MethodGet, slug)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "<title>Booking unavailable</title>") {
		t.Fatalf("paused preview: %d %s", response.Code, body)
	}
	expectNoMarginceOrHost(t, body)
	// The guest's own page still shows who the paused page belongs to, and
	// nothing about the calendar behind it.
	profile := httptest.NewRecorder()
	server.ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/v1/public/booking/"+slug+"/profile", nil))
	body = profile.Body.String()
	if profile.Code != http.StatusOK || !strings.Contains(body, "Ada") || !strings.Contains(body, `"paused":true`) || strings.Contains(body, "Tea") {
		t.Fatalf("paused profile: %d %s", profile.Code, body)
	}
}

func TestRevokedAndMissingBookingPreviewsAre404WithNoHost(t *testing.T) {
	e := integration.Setup(t)
	theCompanyExists(t, e)
	old := hostWithBookingPage(t, e, true)
	replace := true
	store := e.Activities.WithPublicBaseURL(previewOrigin).WithSchedulingCalendar(privateCalendar{}).WithSchedulingBrand(e.Contacts)
	current, err := store.SchedulingProfile(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	current.ReplaceLink = &replace
	replaced, err := store.SaveSchedulingProfile(e.Admin(), current)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Slug == nil || *replaced.Slug == old {
		t.Fatalf("Replace address kept the slug: %+v", replaced.Slug)
	}
	server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), withConnectedCalendar(), WithPublicBaseURL(previewOrigin))
	for _, slug := range []string{old, "no-such-page"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			response := fetchBookingPreview(t, server, method, slug)
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s retired or unknown page: %d %s", method, response.Code, response.Body.String())
			}
			expectNoMarginceOrHost(t, response.Body.String())
			if strings.Contains(response.Body.String(), "og:") || strings.Contains(response.Body.String(), "canonical") {
				t.Fatalf("404 card carries metadata: %s", response.Body.String())
			}
		}
	}
	if live := fetchBookingPreview(t, server, http.MethodGet, *replaced.Slug); live.Code != http.StatusOK {
		t.Fatalf("replacement page: %d", live.Code)
	}
}

func TestABookingPageOfAHostWhoseAccountIsNotLiveIsGoneEverywhere(t *testing.T) {
	for name, retire := range map[string]string{
		"suspended":   `UPDATE app_user SET status = 'suspended' WHERE id = $1`,
		"deactivated": `UPDATE app_user SET status = 'deactivated' WHERE id = $1`,
		"archived":    `UPDATE app_user SET archived_at = now() WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			theCompanyExists(t, e)
			slug := hostWithBookingPage(t, e, true)
			server := New(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), withConnectedCalendar(), WithPublicBaseURL(previewOrigin))
			if live := fetchBookingPreview(t, server, http.MethodGet, slug); live.Code != http.StatusOK {
				t.Fatalf("live host: %d", live.Code)
			}
			e.WsExec(t, retire, e.AdminUser)
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := fetchBookingPreview(t, server, method, slug)
				if response.Code != http.StatusNotFound {
					t.Fatalf("%s preview: %d %s", method, response.Code, response.Body.String())
				}
				expectNoMarginceOrHost(t, response.Body.String())
			}
			// The booking page itself refuses the same way: one resolution path.
			for _, path := range []string{"/profile", "/availability?from=2026-10-05T00:00:00Z&to=2026-10-06T00:00:00Z"} {
				response := httptest.NewRecorder()
				server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/public/booking/"+slug+path, nil))
				if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "Ada") {
					t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
				}
			}
		})
	}
}
