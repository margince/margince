// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A point resolved on the company row moves beside the company. It keeps its
// status while it still describes the address, and turns stale once the
// address moved on. It never replaces an answer the worker already gave.
func TestTheCopyTaskMovesEachPointBesideItsCompany(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	// The columns the company row held its point in, written as rows from
	// before the move look: no writer reaches them any more.
	seed := func(name, line1 string) ids.UUID {
		company := ids.NewV7()
		e.WsExec(t, `INSERT INTO company (id, display_name, address_line1, address_city, source, captured_by)
			VALUES ($1, $2, $3, 'Berlin', 'manual', 'human:x')`, company, name, line1)
		address, ok, err := e.Contacts.AddressForGeocode(ctx, ids.From[ids.CompanyKind](company))
		if err != nil || !ok {
			t.Fatalf("reading %s's address: ok=%v err=%v", name, ok, err)
		}
		e.WsExec(t, `UPDATE company SET geocode_lat = 52.5, geocode_lon = 13.4, geocode_status = 'ok',
			geocode_provider = 'test', geocode_input_hash = $2, geocoded_at = now() WHERE id = $1`,
			company, address.InputHash)
		return company
	}
	kept := seed("Gleich GmbH", "Torstr. 1")
	moved := seed("Umgezogen GmbH", "Torstr. 2")
	e.WsExec(t, `UPDATE company SET address_line1 = 'Neue Str. 9' WHERE id = $1`, moved)
	answered := seed("Beantwortet GmbH", "Torstr. 3")
	e.WsExec(t, `INSERT INTO company_geocode_state (company_id, status) VALUES ($1, 'no_match')`, answered)

	runMachineAddressTask(ctx, t, e, geocodePointsDataTask())

	for _, tc := range []struct {
		company ids.UUID
		want    string
	}{{kept, contacts.GeocodeOK}, {moved, contacts.GeocodeStale}, {answered, contacts.GeocodeNoMatch}} {
		got := e.WsScalar(t, `SELECT coalesce(status, '') FROM company_geocode_state WHERE company_id = $1`, tc.company)
		if got != tc.want {
			t.Errorf("company %s carries status %q beside it, want %q", tc.company, got, tc.want)
		}
	}
}
