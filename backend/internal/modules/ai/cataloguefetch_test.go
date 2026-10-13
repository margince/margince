// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/outbound"
)

// A body cut at the cap is not the vendor's list: half a JSON document parses
// as nothing or, worse, as a shorter list, so a read past the cap is refused.
func TestAVendorListLargerThanItsCapIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != outbound.ModelCatalogueHeader {
			t.Errorf("User-Agent = %q, want %q", got, outbound.ModelCatalogueHeader)
		}
		if _, err := w.Write([]byte(strings.Repeat("x", 11))); err != nil {
			t.Errorf("writing the body: %v", err)
		}
	}))
	defer srv.Close()

	capped := vendorCatalogueFetcher{vendor: "test", url: srv.URL, maxBytes: 10}
	if _, err := capped.Fetch(context.Background()); err == nil {
		t.Fatal("an 11-byte answer under a 10-byte cap was read as whole")
	}
	exact := vendorCatalogueFetcher{vendor: "test", url: srv.URL, maxBytes: 11}
	body, err := exact.Fetch(context.Background())
	if err != nil || len(body) != 11 {
		t.Fatalf("an answer exactly at the cap = %d bytes, %v; want all 11", len(body), err)
	}
}

func TestAVendorAnsweringOtherThan200IsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	f := vendorCatalogueFetcher{vendor: "test", url: srv.URL, maxBytes: 10}
	if _, err := f.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want the status named", err)
	}
}
