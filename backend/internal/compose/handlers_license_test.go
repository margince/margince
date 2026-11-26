// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
)

var resolvedAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

// The refusal reason never reaches the wire. It is the module's account of the
// installation's own configuration, and the module quotes token content it has
// not verified — so it belongs in the boot error and the log, not a response.
// The posture reaches the handler only through the option, because the assembly
// that wires the seat count runs BEFORE the options: capturing it there would
// capture nil and answer 501 for the life of the process.
func TestWithLicensePostureReachesTheEntitlementHandler(t *testing.T) {
	t.Parallel()
	// The handler set is embedded, so its fields are reached through Server
	// itself — `posture` here IS licenseHandlers.posture.
	var srv Server
	if srv.posture != nil {
		t.Fatal("a Server nobody configured already holds a posture")
	}
	WithLicensePosture(func() licensecheck.Posture {
		return licensecheck.Posture{State: licensecheck.StateAbsent, CheckedAt: resolvedAt}
	})(&srv, nil)

	if srv.posture == nil {
		t.Fatal("the option did not reach the entitlement handler; it would answer 501 forever")
	}
	if got := srv.posture().State; got != licensecheck.StateAbsent {
		t.Errorf("the handler's posture reports %q", got)
	}
	// The same option feeds the /metrics section: one wiring point, so the screen
	// and the exposition cannot disagree about what this process resolved.
	if srv.licensePosture == nil {
		t.Error("the option did not reach the /metrics accessor")
	}
}

// The option ADDS the posture and keeps the seat count, which is the invariant
// that lets the two halves be wired in two places.
//
// The count comes from the assembly and the posture from the option, so a
// WithLicensePosture that rebuilt the struct instead of adding to it would drop
// the store — and the drop is silent: both entitlement and capacity would answer
// 501 for the life of the process, in the one role that applies this option.
// Nothing else covers that branch, because the harness that exercises the
// capacity endpoint wires no posture and so never runs this code.
func TestWithLicensePostureKeepsTheSeatCountItWasGiven(t *testing.T) {
	t.Parallel()
	// Stand in for the assembly, which is what puts the store here. A real one
	// needs a pool; what this asserts is only that the option does not discard
	// whatever it found, so a non-nil marker is the whole fixture.
	srv := Server{
		seats: &identity.SeatUsageStore{},
	}

	WithLicensePosture(func() licensecheck.Posture {
		return licensecheck.Posture{State: licensecheck.StateAbsent, CheckedAt: resolvedAt}
	})(&srv, nil)

	if srv.seats == nil {
		t.Fatal("the option dropped the seat count; both license surfaces would answer 501")
	}
	if srv.posture == nil {
		t.Fatal("the option did not add the posture")
	}
}

// The licensee detail the module proved, onto the wire. What matters here is
// which claims survive and which verdicts the server states for the client.
// The holder rides only a verified license. An unlicensed installation has no
// licensee, and a refused one proved nothing about who holds it.
