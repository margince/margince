// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
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
		{http.MethodGet, "/v1/contacts", false},
	}
	for _, c := range cases {
		got := isMFAEnrolRequest(httptest.NewRequest(c.method, c.path, nil))
		if got != c.want {
			t.Errorf("isMFAEnrolRequest(%s %s) = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

func TestRequireMFAPolicyCannotConfineWithoutAVault(t *testing.T) {
	// With no vault there is no enrolment, so a require-MFA policy that still
	// confined would trap every factorless member on routes that can only
	// refuse. The composition root reports the misconfiguration at boot;
	// admission must answer "not required" rather than convert it to a lockout.
	svc := &Service{requireMFA: func(context.Context) (bool, error) { return true, nil }}
	mandatory, err := svc.mfaMandatory(context.Background())
	if err != nil {
		t.Fatalf("mfaMandatory: %v", err)
	}
	if mandatory {
		t.Error("require-MFA confines although no vault exists to enrol against")
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

func TestReadSeatMayDisableItsOwnFactor(t *testing.T) {
	// The mirror of enrolment: a read seat that can enrol a factor must be able
	// to remove it. Enrol-only would strand a read seat that lost its
	// authenticator — DELETE blocked by the tier ceiling, re-enrol refused
	// because a confirmed factor already exists — with no self-service recovery
	// once its recovery codes run out.
	if !readSeatMayMutate(httptest.NewRequest(http.MethodDelete, "/v1/me/mfa", nil)) {
		t.Error("a read seat cannot disable the factor it enrolled — enrol-only is a lockout")
	}
}
