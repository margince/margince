// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The mode guard in front of the rejection.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An overlay workspace's native organization table holds none of its records,
// so a rejection there would answer "not found" about a company the reader is
// looking at — and record a domain refusal for a capture path that is not the
// one creating the records. ADR-0018 takes the other answer: a capability that
// is not served says so with the declared sentinel.
func TestRejectingACompanyIsRefusedInOverlayMode(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/x/reject", http.NoBody)

	if refused := refuseInOverlayMode(rec, req, overlayMode()); !refused {
		t.Fatal("an overlay workspace was let through to the native store")
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	// The same machine code every other native-only capability answers: a
	// caller must not have to know which one it asked for to recognise a
	// declared gap.
	if !strings.Contains(rec.Body.String(), "unsupported_by_sor") {
		t.Errorf("body = %s, want the unsupported_by_sor sentinel", rec.Body.String())
	}
}

// The control: a native workspace reaches the transport, and the guard writes
// nothing on the way. Without it the case above would pass against a guard that
// refused every workspace, which would take the verb off the product.
func TestRejectingACompanyReachesTheStoreInNativeMode(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/x/reject", http.NoBody)

	if refused := refuseInOverlayMode(rec, req, nativeMode()); refused {
		t.Fatal("a native workspace was refused its own rejection")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("the guard wrote %s for a native workspace", rec.Body.String())
	}
}
