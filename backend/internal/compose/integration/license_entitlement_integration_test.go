// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The seat count the entitlement surface reports, against a real database.
//
// Nothing else can prove it. The count is a SQL predicate over app_user — full
// seats that are not deactivated, agents included — and the three decisions it
// makes are the difference between a meter that bills honestly and one that
// bills for access the installation already withdrew:
//
//   a read seat is never counted        A62/ADR-0047: they are unlimited
//   a deactivated seat is not counted   the access is already gone
//   an agent seat IS counted            it acts on the estate like a human
//
// A unit test cannot see any of it: what is real here is the predicate running
// against rows a real database holds, and the verdict the server reaches with
// it.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entitlement is the wire shape, read back rather than restated: a field the
// contract renames has to fail here rather than decode into nothing.
type entitlement struct {
	State        string `json:"state"`
	SeatsUsed    int    `json:"seats_used"`
	SeatsGranted *int   `json:"seats_granted"`
	OverLimit    bool   `json:"over_limit"`
}

func TestLicenseEntitlementCountsTheSeatsThatAct(t *testing.T) {
	// The posture is injected, because no token this repository can mint is
	// accepted by the bundled keyset. What is REAL here is the seat count and
	// the verdict the server reaches with it — the half a fixture cannot fake.
	//
	// The grant is a VARIABLE the posture closes over, because the posture is a
	// func the surface calls per request. That is what lets both seats be written
	// by the product: an installation cannot be invited past its entitlement —
	// the seat ceiling refuses that, correctly — so the only way to be over the
	// limit is for the rows to predate the grant shrinking, and this reproduces
	// that in the order it really happens.
	seats := 5
	e := apptest.SetupAppWithOptions(t, compose.WithLicensePosture(func() licensecheck.Posture {
		return licensecheck.Posture{
			State:     licensecheck.StateValid,
			Grants:    licensecheck.Grants{licensecheck.SeatsAttribute: float64(seats)},
			Issuer:    licensecheck.ProductionIssuer,
			License:   licensecheck.License{ID: "0199-integration", Subject: "integration", Expiry: time.Now().AddDate(1, 0, 0)},
			CheckedAt: time.Now(),
		}
	}))
	e.BootstrapWorkspace(t)

	// A second person, through the members surface — the writer, not an insert.
	if status := e.Call(t, "POST", "/v1/users", map[string]any{
		"email": "second@example.com", "display_name": "Second Person", "role": "rep",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("invite a second member → %d, want 201", status)
	}

	// Now the licence shrinks under them.
	seats = 1

	var seeded entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &seeded); status != http.StatusOK {
		t.Fatalf("read the entitlement → %d", status)
	}
	if seeded.SeatsUsed != 2 {
		t.Fatalf("seats in use = %d, want 2 (the admin and the invited member) — the count is what "+
			"the over-limit verdict below is computed from", seeded.SeatsUsed)
	}
	if seeded.State != "valid" {
		t.Errorf("state = %q, want valid", seeded.State)
	}
	if seeded.SeatsGranted == nil || *seeded.SeatsGranted != 1 {
		t.Fatalf("seats granted = %v, want the injected 1", seeded.SeatsGranted)
	}
	// The verdict the client is not allowed to compute for itself, reached here
	// from rows the product wrote.
	if !seeded.OverLimit {
		t.Errorf("%d seats against a grant of 1 is not over the limit", seeded.SeatsUsed)
	}

	// A read seat does not act, so the meter must not count it. This is the one
	// rule a customer feels directly: read seats are how a workspace hands out
	// visibility without paying for it.
	before := seeded.SeatsUsed
	e.SetWorkspaceSeat(t, "read")

	var afterDemotion entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &afterDemotion); status != http.StatusOK {
		t.Fatalf("read the entitlement after the demotion → %d", status)
	}
	if afterDemotion.SeatsUsed >= before {
		t.Errorf("seats in use = %d after every human became a read seat, was %d — read seats are being counted",
			afterDemotion.SeatsUsed, before)
	}
	// And it reaches ZERO: every metered seat on this installation belongs to a
	// person, and no product path creates anything else.
	if afterDemotion.SeatsUsed != 0 {
		t.Errorf("seats in use = %d after every human became a read seat, want 0 — something is "+
			"metered that no person uses", afterDemotion.SeatsUsed)
	}

	// THE THIRD RULE, and this is the only place that holds it: an agent seat IS
	// counted. It used to be held by accident — bootstrap seeded an agent row, so
	// the count after this demotion could not reach zero — and retiring that seed
	// took the assertion with it.
	//
	// Written through the owner connection because nothing in the product creates
	// an agent row any more, which is what TestBootstrapMintsNoAgentSeat asserts.
	// A resident runner will land under this flag, and it must arrive metered:
	// excluding agents is what would let an installation act without limit
	// through them. `full` and `active` are spelled out because
	// app_user_agent_is_full admits no other seat type for an agent — the same
	// constraint that makes this row survive the demotion above.
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO app_user (email, display_name, is_agent, seat_type, status)
		 VALUES ('runner@example.com', 'A Runner', true, 'full', 'active')`); err != nil {
		t.Fatalf("seeding an agent identity: %v", err)
	}
	var withAgent entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &withAgent); status != http.StatusOK {
		t.Fatalf("read the entitlement with an agent identity → %d", status)
	}
	if withAgent.SeatsUsed != 1 {
		t.Errorf("seats in use = %d with one agent identity and every human demoted, want 1 — an "+
			"agent that is not counted is an installation acting without limit through agents",
			withAgent.SeatsUsed)
	}
}

// An unlicensed installation ANSWERS. Absent is a posture, not a refusal — every
// development and CI installation runs in it, and a 403 or a 501 here would tell
// an admin their entitlement surface was broken rather than that they hold no
// license.
func TestTheEntitlementSurfaceAnswersAnUnlicensedInstallation(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithLicensePosture(func() licensecheck.Posture {
		return licensecheck.Posture{State: licensecheck.StateAbsent, CheckedAt: time.Now()}
	}))
	e.BootstrapWorkspace(t)

	// An unlicensed installation still answers the admin: absent is a posture,
	// not a refusal.
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the admin reading an unlicensed installation → %d", status)
	}
}

// licenseReaderPerms is a rep: every grant a rep holds, and no `license` read.
// The entitlement is admin/ops-only, read included, because a seat meter is the
// installation's commercial standing (UC-ADMIN-03 F1).
var licenseReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{"person": {Create: true, Read: true, Update: true}},
	RowScope: principal.RowScopeTeam,
}

// The gate is the STORE's, not the transport's: a caller without the grant is
// refused before a single row is counted, so no seat count can reach a principal
// who may not have it.
func TestTheSeatCountRefusesAPrincipalWithoutTheLicenseGrant(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, licenseReaderPerms)

	used, err := identity.NewSeatUsage(e.DB()).FullSeatsInUse(rep)
	if err == nil {
		t.Fatalf("a rep counted the installation's seats and got %d", used)
	}
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("refusal = %v, want ErrPermissionDenied", err)
	}
}
