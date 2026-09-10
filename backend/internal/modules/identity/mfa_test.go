// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMFAEnrolRoutesAreTheOnlyEscapeFromConfinement(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{http.MethodGet, "/v1/me/mfa", true},
		{http.MethodPost, "/v1/me/mfa/totp", true},
		{http.MethodPost, "/v1/me/mfa/totp/confirm", true},
		// Disabling is not an escape: a required factor must not be removable from
		// inside the confinement.
		{http.MethodDelete, "/v1/me/mfa", false},
		// A business route stays confined.
		{http.MethodGet, "/v1/people", false},
	}
	for _, c := range cases {
		got := isMFAEnrolRequest(httptest.NewRequest(c.method, c.path, nil))
		if got != c.want {
			t.Errorf("isMFAEnrolRequest(%s %s) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

func TestReadSeatMayEnrolAMandatedFactor(t *testing.T) {
	// A read seat forced to enrol MFA must be able to POST the enrolment, which
	// mutates — enrolling a mandated factor is self-management, not a business
	// write the tier ceiling should block.
	if !readSeatMayMutate(httptest.NewRequest(http.MethodPost, "/v1/me/mfa/totp", nil)) {
		t.Error("a read seat cannot enrol a factor the installation requires of it")
	}
}
