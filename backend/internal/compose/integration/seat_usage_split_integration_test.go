// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The seat COUNT read apart from the entitlement it normally sits beside.
//
// `GET /installation/license` answers capacity and commercial standing together
// and is governed by `license`, so a role could not be shown how many seats are
// in use without also being handed what the installation pays. Management needs
// the first and not the second, which is what `seat_usage` is for.
//
// What has to be true, and what these prove against a real database:
//
//   a seat_usage holder is admitted and a license holder still is
//   a rep holding neither is refused, by the STORE and not the transport
//   both doors report the SAME number
//
// That last one is the reason this is an integration test rather than a unit
// one. Two counts of one installation's seats would drift, and the one that
// drifted would be the one nobody was refused a seat by — a meter disagreeing
// with the ceiling it measures. The doors run one statement; a change that gave
// either its own query fails here.

import (
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatUsage is the wire shape, read back rather than restated, so a renamed
// field fails here instead of decoding into a zero that looks like an empty
// installation.
type seatUsage struct {
	SeatsUsed int `json:"seats_used"`
}

// seatUsageOnlyPerms is the reader this split exists for: the capacity grant and
// nothing commercial. `license` is absent on purpose — with it the case would
// pass through the OLD gate and prove nothing about the new one.
var seatUsageOnlyPerms = principal.Permissions{
	RoleKeys: []string{"management"},
	Objects:  map[string]principal.ObjectGrant{"seat_usage": {Read: true}},
	RowScope: principal.RowScopeAll,
}

// noSeatGrantPerms is a rep: neither grant, so both doors must refuse. Spelled
// here rather than reusing the entitlement file's licenseReaderPerms, whose name
// says the opposite of what this case means — a reader who HOLDS the license
// grant is precisely what it is not.
var noSeatGrantPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{"person": {Create: true, Read: true, Update: true}},
	RowScope: principal.RowScopeTeam,
}

// licenseOnlyPerms is the opposite reader, and it is here to prove the split did
// not quietly move the entitlement surface onto the new object: a holder of
// `license` alone must still read it.
var licenseOnlyPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects:  map[string]principal.ObjectGrant{"license": {Read: true}},
	RowScope: principal.RowScopeAll,
}

func TestSeatUsageAdmitsTheCapacityGrantAndRefusesAReader(t *testing.T) {
	e := Setup(t)
	store := identity.NewSeatUsage(e.DB())

	// Admitted: the grant this endpoint was split out for.
	management := e.As(e.Rep1, []ids.UUID{e.Team1}, seatUsageOnlyPerms)
	used, err := store.SeatsInUse(management)
	if err != nil {
		t.Fatalf("a seat_usage holder was refused the seat count: %v", err)
	}
	if used < 1 {
		// A zero would pass an assertion of "no error" while proving the count
		// never ran. The harness seeds users, so the meter has something to see.
		t.Fatalf("seats in use = %d, want at least the seeded users", used)
	}

	// Refused: a rep holds neither grant. The refusal is the STORE's, so no row
	// is counted before it happens.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, noSeatGrantPerms)
	if got, err := store.SeatsInUse(rep); err == nil {
		t.Fatalf("a rep counted the installation's seats and got %d", got)
	} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("refusal = %v, want ErrPermissionDenied", err)
	}
}

// The capacity grant does NOT open the entitlement surface, which is the whole
// point of splitting rather than widening: a reader who may see how full the
// installation is still may not see what it pays.
func TestSeatUsageDoesNotAdmitTheEntitlement(t *testing.T) {
	e := Setup(t)
	management := e.As(e.Rep1, []ids.UUID{e.Team1}, seatUsageOnlyPerms)

	if got, err := identity.NewSeatUsage(e.DB()).FullSeatsInUse(management); err == nil {
		t.Fatalf("a seat_usage holder read the entitlement count and got %d", got)
	} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("refusal = %v, want ErrPermissionDenied", err)
	}
}

// One meter, two doors. If either entry point ever grows a statement of its own
// this is what fails, and it fails on the number rather than on the shape — the
// drift a second query causes is silent everywhere else.
func TestBothDoorsReportOneMeter(t *testing.T) {
	e := Setup(t)
	store := identity.NewSeatUsage(e.DB())

	viaLicense, err := store.FullSeatsInUse(e.As(e.Rep1, []ids.UUID{e.Team1}, licenseOnlyPerms))
	if err != nil {
		t.Fatalf("a license holder was refused the entitlement count: %v", err)
	}
	viaSeatUsage, err := store.SeatsInUse(e.As(e.Rep1, []ids.UUID{e.Team1}, seatUsageOnlyPerms))
	if err != nil {
		t.Fatalf("a seat_usage holder was refused the seat count: %v", err)
	}
	if viaLicense != viaSeatUsage {
		t.Errorf("the two doors disagree: license says %d, seat_usage says %d", viaLicense, viaSeatUsage)
	}
}

// The endpoint over the wire, because the store test above proves the gate and
// not the route: a handler wired to the wrong store, or not wired at all, is
// invisible to every case above it.
func TestTheSeatUsageEndpointAnswersItsHolder(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var got seatUsage
	if status := e.Call(t, "GET", "/v1/installation/seat-usage", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /installation/seat-usage = %d, want 200", status)
	}
	if got.SeatsUsed < 1 {
		t.Errorf("seats_used = %d, want at least the seeded users", got.SeatsUsed)
	}
}
