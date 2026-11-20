// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/dormancy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The list filter puts each company on the rung the page's reading names, on
// both sides of every boundary day. The installation zone is Berlin, where half
// past midnight is still the day before in UTC, so only the zone gets it right.
func TestTheDormancyFilterAgreesWithTheLadderOnEveryBoundaryDay(t *testing.T) {
	e := setupDedupe(t)
	owner := e.as()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := clocktest.Now(t).In(berlin)
	e.store.SetClock(func() time.Time { return now })
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(owner, `INSERT INTO setting (key, value) VALUES ('installation.timezone', '"Europe/Berlin"')
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`)
		return err
	}); err != nil {
		t.Fatalf("setting the installation zone: %v", err)
	}

	lastActivity := map[ids.UUID]*time.Time{}
	for _, days := range []int{0, 29, 30, 59, 60, 89, 90} {
		day := now.AddDate(0, 0, -days)
		at := time.Date(day.Year(), day.Month(), day.Day(), 0, 30, 0, 0, berlin)
		company := seedLogoCompany(owner, t, e, fmt.Sprintf("Silent %d", days), fmt.Sprintf("silent%d.test", days))
		lastActivity[company.UUID] = &at
	}
	never := seedLogoCompany(owner, t, e, "Never Spoken", "never.test")
	lastActivity[never.UUID] = nil
	if err := e.store.tx(owner, func(tx pgx.Tx) error {
		for id, at := range lastActivity {
			if _, err := tx.Exec(owner, `UPDATE company SET last_activity_at = $2 WHERE id = $1`, id, at); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("placing the companies' clocks: %v", err)
	}

	named := make([]ids.UUID, 0, len(lastActivity))
	for id := range lastActivity {
		named = append(named, id)
	}
	for _, rung := range dormancy.Rungs() {
		listed, _, err := e.store.ListCompanies(owner, ListCompaniesInput{IDs: named, Dormancy: new(string(rung))})
		if err != nil {
			t.Fatalf("listing the %s companies: %v", rung, err)
		}
		for id, at := range lastActivity {
			want := dormancy.Default.Read(at, now, berlin).Rung == rung
			got := slices.ContainsFunc(listed, func(c crmcontracts.Company) bool { return c.Id == openapiUUID(ids.From[ids.CompanyKind](id)) })
			if got != want {
				t.Errorf("rung %s: company with last activity %v listed=%v, want %v", rung, at, got, want)
			}
		}
	}

	if _, _, err := e.store.ListCompanies(owner, ListCompaniesInput{Dormancy: new("frozen")}); err == nil {
		t.Error("an unknown rung was accepted, want a validation error")
	}
}
