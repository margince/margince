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
// The third rule is untested here because no product path creates an agent row:
// it belongs to the predicate (identity/seatusage.go) and to the resident runner
// that will land under `is_agent`.
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
	// One granted seat, against two people. So over_limit is proven from a count
	// the predicate reached over real rows rather than from a number a test
	// chose.
	e := apptest.SetupAppWithOptions(t, compose.WithLicensePosture(func() licensecheck.Posture {
		return licensecheck.Posture{
			State:     licensecheck.StateValid,
			Grants:    licensecheck.Grants{licensecheck.SeatsAttribute: float64(1)},
			Issuer:    licensecheck.ProductionIssuer,
			License:   licensecheck.License{ID: "0199-integration", Subject: "integration", Expiry: time.Now().AddDate(1, 0, 0)},
			CheckedAt: time.Now(),
		}
	}))
	e.BootstrapWorkspace(t)

	// A SECOND person, written directly, and the directness is the point: the
	// invite endpoint cannot produce an over-limit installation, because the seat
	// ceiling refuses a seat the entitlement does not cover. The only way to be
	// over the limit is for the rows to have existed before the grant shrank, and
	// that is the state this row models.
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO app_user (email, display_name) VALUES ($1, 'Second Person')`,
		"second@example.com"); err != nil {
		t.Fatalf("seeding a second full seat: %v", err)
	}

	var seeded entitlement
	if status := e.Call(t, "GET", "/v1/installation/license", nil, nil, &seeded); status != http.StatusOK {
		t.Fatalf("read the entitlement → %d", status)
	}
	if seeded.SeatsUsed != 2 {
		t.Fatalf("seats in use = %d, want 2 (the admin and the second person) — the count is what "+
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
	// And it reaches ZERO, because every metered seat belongs to a person. An
	// agent row would survive the demotion — app_user_agent_is_full refuses to
	// demote one — and would still count, which is what stops an installation
	// acting without limit through agents. There is simply never one here.
	if afterDemotion.SeatsUsed != 0 {
		t.Errorf("seats in use = %d after every human became a read seat, want 0 — something is "+
			"metered that no person uses", afterDemotion.SeatsUsed)
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
