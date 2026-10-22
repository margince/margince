// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
	"github.com/margince/margince/backend/pkg/extension"
)

// Only the development posture lets a unit's client reach loopback: test and
// production, and any value runtimeenv reads as production, keep refusing it.
func TestOnlyTheDevelopmentPostureLetsAUnitDialLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()
	t.Cleanup(func() { BindExtensionEgress(runtimeenv.Production) })
	for _, posture := range []string{"dev", "test", "production", "staging", ""} {
		BindExtensionEgress(runtimeenv.Parse(posture))
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := extension.OutboundClient().Do(request)
		if err == nil {
			if closeErr := response.Body.Close(); closeErr != nil {
				t.Errorf("closing the body: %v", closeErr)
			}
		}
		if reached, want := err == nil, posture == "dev"; reached != want {
			t.Errorf("MARGINCE_ENV=%q: reached loopback = %v, want %v (err: %v)", posture, reached, want, err)
		}
	}
}

// A unit's request carries the extension identity the core declares, so the
// provider's operator sees one name for every unit's call.
func TestAUnitsRequestCarriesTheDeclaredExtensionIdentity(t *testing.T) {
	t.Cleanup(func() { BindExtensionEgress(runtimeenv.Production) })
	BindExtensionEgress(runtimeenv.Production)
	request, err := extension.NewRequest(context.Background(), http.MethodGet, "https://provider.example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if agent := request.Header.Get("User-Agent"); agent != outbound.ExtensionHeader {
		t.Errorf("User-Agent = %q, want %q", agent, outbound.ExtensionHeader)
	}
}
