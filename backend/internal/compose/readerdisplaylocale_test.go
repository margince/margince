// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func TestOnlyHumanDisplayChoicesOverrideTheSavedReaderLanguage(t *testing.T) {
	for _, tc := range []struct {
		name, header, want string
		human              bool
	}{
		{"explicit choice", "vi", "vi", true},
		{"invalid choice", "fr", "de", true},
		{"saved choice", "", "de", true},
		{"agent header ignored", "vi", "de", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := principal.PrincipalAgent
			if tc.human {
				kind = principal.PrincipalHuman
			}
			ctx := principal.WithActor(context.Background(), principal.Principal{Type: kind, Locale: "de"})
			request := httptest.NewRequest(http.MethodGet, "/v1/activities", nil).WithContext(ctx)
			request.Header.Set("X-Margince-Display-Locale", tc.header)
			request.Header.Set("Accept-Language", "en")
			response := httptest.NewRecorder()
			handler := readerDisplayLocale(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if got := string(systemtext.Locale(r.Context())); got != tc.want {
					t.Fatalf("locale = %q, want %q", got, tc.want)
				}
				if systemtext.HasLocale(r.Context()) != tc.human {
					t.Fatal("nonhuman acquired a display override")
				}
			}))
			handler.ServeHTTP(response, request)
		})
	}
}

func TestPublicPreviewLanguageHasNoAccountDependency(t *testing.T) {
	for _, tc := range []struct{ query, display, browser, want string }{
		{"?locale=vi", "de", "en", "vi"},
		{"?locale=fr", "de", "en", "de"},
		{"", "", "de-DE,de;q=0.9,en;q=0.5", "de"},
		{"", "", "vi-VN", "vi"},
		{"", "", "invalid;header", "en"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/booking"+tc.query, nil)
		request.Header.Set("X-Margince-Display-Locale", tc.display)
		request.Header.Set("Accept-Language", tc.browser)
		if got := string(publicPreviewLocale(request)); got != tc.want {
			t.Fatalf("%+v: %s", tc, got)
		}
	}
}
