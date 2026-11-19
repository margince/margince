// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func faultCode(t *testing.T, err error) string {
	t.Helper()
	var parse *values.ParseError
	if !errors.As(err, &parse) {
		t.Fatalf("err = %v, want a value refusal", err)
	}
	return parse.Code
}

func TestALinkAddressIsAWebAddressAUserCanTrust(t *testing.T) {
	refused := map[string]string{
		"":                                  "required",
		"   ":                               "required",
		"docs.google.com/document/d/abc":    "invalid_url",
		"ftp://files.example.com/a":         "invalid_url",
		"javascript:alert(1)":               "invalid_url",
		"https://bank.example@evil.example": "invalid_url",
		"https://example.com/" + strings.Repeat("a", maxLinkAddressLength): "too_long",
	}
	for raw, code := range refused {
		if _, err := LinkAddress(raw); faultCode(t, err) != code {
			t.Errorf("LinkAddress(%q) code = %q, want %q", raw, faultCode(t, err), code)
		}
	}
	got, err := LinkAddress("  HTTPS://Example.com/Brief?x=1  ")
	if err != nil || got != "https://Example.com/Brief?x=1" {
		t.Fatalf("LinkAddress = %q, %v; want it trimmed with the scheme the column reads", got, err)
	}
}

func TestALinkTitleIsOneLineWithinTheColumnBound(t *testing.T) {
	got, err := linkTitle("  Rollout\tplan\n2027  ")
	if err != nil || got != "Rollout plan 2027" {
		t.Fatalf("linkTitle = %q, %v", got, err)
	}
	if _, err := linkTitle(strings.Repeat("é", maxLinkTitleRunes)); err != nil {
		t.Fatalf("a title at the bound: %v", err)
	}
	if code := faultCode(t, func() error { _, err := linkTitle(strings.Repeat("é", maxLinkTitleRunes+1)); return err }()); code != "too_long" {
		t.Fatalf("code = %q, want too_long", code)
	}
}

func TestALinkIsFiledByWhatItIsNotHowItArrived(t *testing.T) {
	for _, category := range []string{"", "contract", "offer", "legal", "other"} {
		if err := refuseUnaddableCategory(category); err != nil {
			t.Errorf("%q refused: %v", category, err)
		}
	}
	for _, category := range []string{"link", "email_attachment", "message_attachment"} {
		if code := faultCode(t, refuseUnaddableCategory(category)); code != "category_not_assertable" {
			t.Errorf("%q code = %q", category, code)
		}
	}
}

type stubPages struct {
	page  webread.Page
	err   error
	asked []string
}

func (s *stubPages) FetchPage(_ context.Context, rawURL string) (webread.Page, error) {
	s.asked = append(s.asked, rawURL)
	return s.page, s.err
}

func TestALinkPreviewNamesThePage(t *testing.T) {
	pages := &stubPages{page: webread.Page{Title: strings.Repeat("t", maxLinkTitleRunes+5), SiteName: "Acme"}}
	got, err := previewLink(context.Background(), pages, "https://acme.example/pricing")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Read || got.Host != "acme.example" || got.SiteName == nil || *got.SiteName != "Acme" {
		t.Fatalf("preview = %+v", got)
	}
	if got.Title == nil || len(*got.Title) > maxLinkTitleRunes {
		t.Fatalf("title = %v, want one the link can be saved with", got.Title)
	}
}

func TestALinkPreviewThatCannotReadThePageStillAnswers(t *testing.T) {
	got, err := previewLink(context.Background(), &stubPages{err: errors.New("robots.txt disallows")}, "https://acme.example/")
	if err != nil || got.Read || got.Title != nil {
		t.Fatalf("preview = %+v, %v; want an unread answer, no error", got, err)
	}
}

func TestALinkPreviewLeavesAGoogleFileUnfetched(t *testing.T) {
	pages := &stubPages{}
	got, err := previewLink(context.Background(), pages, "https://docs.google.com/spreadsheets/d/abc/edit")
	if err != nil || got.Read || len(pages.asked) != 0 {
		t.Fatalf("preview = %+v, %v, fetched %v; want the sign-in page left alone", got, err, pages.asked)
	}
}
