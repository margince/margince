// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package netguard

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// The incident this exists for: a real message went out carrying
// http://localhost:8080, which on the recipient's machine means their own
// computer.
func TestALoopbackOriginIsRefusedInProduction(t *testing.T) {
	for _, origin := range []string{
		"http://localhost:8080",
		"https://localhost",
		"https://app.localhost",
		"http://127.0.0.1:5173",
		"https://[::1]",
		"https://10.0.0.5",
		"https://192.168.1.10",
		"https://169.254.1.1",
		"https://0.0.0.0",
	} {
		t.Run(origin, func(t *testing.T) {
			err := RequirePublicOrigin("--public-base-url", origin, runtimeenv.Production)
			if err == nil {
				t.Fatal("admitted an origin no recipient could open")
			}
			if !errors.Is(err, ErrOriginNotPublic) {
				t.Errorf("error = %v, want it to wrap ErrOriginNotPublic", err)
			}
		})
	}
}

// Cleartext is refused even on a public name: the link lands in a mailbox
// and is clicked from networks nobody here controls.
func TestAPublicOriginMustBeHTTPSInProduction(t *testing.T) {
	if err := RequirePublicOrigin("--public-base-url", "http://crm.example.com", runtimeenv.Production); err == nil {
		t.Error("admitted a cleartext public origin")
	}
}

func TestAPublicHTTPSOriginIsAdmitted(t *testing.T) {
	for _, origin := range []string{"https://crm.example.com", "https://crm.example.com:8443", "https://93.184.216.34"} {
		if err := RequirePublicOrigin("--public-base-url", origin, runtimeenv.Production); err != nil {
			t.Errorf("RequirePublicOrigin(%q) = %v, want admitted", origin, err)
		}
	}
}

// A hostname is admitted WITHOUT resolving it. Resolving here would make a
// boot decision depend on DNS at the moment of the check, so the honest
// scope is "not obviously broken", never "proven reachable".
func TestAHostnameIsAdmittedWithoutResolving(t *testing.T) {
	if err := RequirePublicOrigin("--public-base-url",
		"https://internal-only.invalid", runtimeenv.Production); err != nil {
		t.Errorf("refused an unresolvable hostname: %v — the check is syntactic by design", err)
	}
}

// The dev stack defaults to http://localhost and must keep working.
func TestDevelopmentAndTestAdmitLocalhost(t *testing.T) {
	for _, env := range []runtimeenv.Environment{runtimeenv.Development, runtimeenv.Test} {
		if err := RequirePublicOrigin("--public-base-url", "http://localhost:5173", env); err != nil {
			t.Errorf("posture %v refused the dev default: %v", env, err)
		}
	}
}

// runtimeenv parses anything it does not recognise as production, and this
// carve-out must inherit that direction rather than opening on a typo.
func TestAnUnknownPostureIsTreatedAsProduction(t *testing.T) {
	if err := RequirePublicOrigin("--public-base-url",
		"http://localhost:8080", runtimeenv.Parse("staging")); err == nil {
		t.Error("an unrecognised posture admitted a loopback origin")
	}
}

// An empty origin is refused rather than passed through as "nothing to check".
func TestAnEmptyOriginIsRefused(t *testing.T) {
	if err := RequirePublicOrigin("--public-base-url", "  ", runtimeenv.Production); err == nil {
		t.Error("admitted an empty origin")
	}
}

// A refusal must not echo an origin that could carry a credential.
func TestARefusalDoesNotEchoUserinfo(t *testing.T) {
	err := RequirePublicOrigin("--public-base-url", "http://user:hunter2@localhost", runtimeenv.Production)
	if err == nil {
		t.Fatal("admitted a loopback origin carrying userinfo")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("the refusal echoed the credential: %v", err)
	}
}
