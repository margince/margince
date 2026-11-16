// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// advertisingPack registers a pack shaped like the Vietnamese one under a
// test code: three refusal means, an advertising language and the refusal
// section last. The installation and the env contact are both in it.
func advertisingPack(t *testing.T, e *channelConsentEnv, code string) *Store {
	t.Helper()
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: jurisdiction.Code(code), Version: 1,
		Disclosures: []messagingrules.Disclosure{
			{Kind: messagingrules.ControllerIdentity},
			{Kind: messagingrules.ObjectionRoute, MarketingOnly: true, Means: []messagingrules.ContactMeans{
				messagingrules.MeansWeb, messagingrules.MeansEmail, messagingrules.MeansPhone,
			}},
			{Kind: messagingrules.AdvertiserContact, MarketingOnly: true},
		},
		AdvertisingLanguage: "vi",
		RefusalSectionLast:  true,
	})
	livesIn(t, e, code)
	return e.store.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return jurisdiction.Code(code), nil
		}))
}

func TestSavingParticularsWithNoPhoneInTheObjectionRouteIsRefused(t *testing.T) {
	e := setupChannelConsent(t)
	store := advertisingPack(t, e, "zv")
	h := Handlers{store: store}.WithSettings(settingsStoreFor(t))

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/privacy/controller-particulars",
			strings.NewReader(body)).WithContext(operatorCtx(e))
		req.Header.Set("Content-Type", "application/json")
		h.SetControllerParticulars(rec, req)
		return rec
	}

	rec := put(`{"legal_name":"Example Trading GmbH",` +
		`"objection_route":"Write to privacy@example.com or open https://example.com/stop."}`)
	if rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "objection_route") ||
		!strings.Contains(rec.Body.String(), "phone number") {
		t.Fatalf("a route with no phone answered %d %s, want 422 naming the field and the phone",
			rec.Code, rec.Body.String())
	}

	rec = put(`{"legal_name":"Example Trading GmbH",` +
		`"objection_route":"Write to privacy@example.com, call +49 30 1234 5678 or open https://example.com/stop."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a route naming all three means answered %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"advertising_languages":["vi"]`) {
		t.Errorf("the answer does not say which language the pack asks for: %s", rec.Body.String())
	}
}

func TestAVietnameseAdvertisingMailPrintsTheStoredVietnameseRoute(t *testing.T) {
	e := setupChannelConsent(t)
	store := advertisingPack(t, e, "zu")
	route := "gửi email đến privacy@example.com, gọi số +84 00 0000 0000 hoặc dùng trang https://example.com/stop"
	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Example Trading GmbH", PostalAddress: "Hauptstraße 1\n10115 Berlin",
		ObjectionRoute:    "Write to privacy@example.com, call +49 30 1234 5678 or open https://example.com/stop.",
		AdvertiserContact: "+49 30 1234 5678, hello@example.com, https://example.com",
		Translations: []ParticularsTranslation{{
			Language: "vi", ObjectionRoute: route,
			AdvertiserContact: "Điện thoại +84 28 3822 9999, email hello@example.com, trang https://example.com",
		}},
	})

	lines := disclosures(t, e, store, commsauthz.CategoryMarketing)
	if len(lines) != 2 || lines[0].Kind != string(messagingrules.AdvertiserContact) ||
		lines[1].Kind != string(messagingrules.ObjectionRoute) {
		t.Fatalf("the advertising lines are %+v, want the advertiser block then the refusal", lines)
	}
	if !strings.Contains(lines[1].Text, route) {
		t.Errorf("the refusal reads %q, want the stored Vietnamese route", lines[1].Text)
	}

	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Example Trading GmbH", PostalAddress: "Hauptstraße 1\n10115 Berlin",
		ObjectionRoute:    "Write to privacy@example.com, call +49 30 1234 5678 or open https://example.com/stop.",
		AdvertiserContact: "+49 30 1234 5678, hello@example.com, https://example.com",
	})
	for _, line := range disclosures(t, e, store, commsauthz.CategoryMarketing) {
		if line.Kind != string(messagingrules.ControllerIdentity) && !line.Missing() {
			t.Errorf("%s printed %q with no Vietnamese version stated; advertising to this "+
				"recipient must be refused until it is", line.Kind, line.Text)
		}
	}
}
