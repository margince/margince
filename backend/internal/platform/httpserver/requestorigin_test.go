// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestARequestOriginCarriesTheDeviceAndTheTrustedAddress(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "[::ffff:203.0.113.7]:5100"
	r.Header.Set("User-Agent", "Mozilla/5.0 (laptop)")
	r.Header.Set("X-Forwarded-For", "198.51.100.9")

	origin := RequestOriginFrom(WithRequestOrigin(context.Background(), r))
	if origin.UserAgent == nil || *origin.UserAgent != "Mozilla/5.0 (laptop)" {
		t.Errorf("user agent = %v, want the header the client sent", origin.UserAgent)
	}
	// The peer is not a trusted proxy, so its forwarded header is not believed.
	if origin.IP == nil || *origin.IP != "203.0.113.7" {
		t.Errorf("ip = %v, want the unmapped peer address", origin.IP)
	}
}

func TestAnOriginNotLearnedIsLeftEmpty(t *testing.T) {
	r := httptest.NewRequest("POST", "/login", nil)
	r.RemoteAddr = "pipe"
	r.Header.Del("User-Agent")

	origin := RequestOriginFrom(WithRequestOrigin(context.Background(), r))
	if origin.UserAgent != nil || origin.IP != nil {
		t.Errorf("origin = %+v, want nil fields for a blank device and an address that does not parse", origin)
	}
	if got := RequestOriginFrom(context.Background()); got.UserAgent != nil || got.IP != nil {
		t.Errorf("origin with no request bound = %+v, want the zero value", got)
	}
}
