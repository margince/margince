// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package strengthdates_test

// Whether a day entry has happened depends on the installation zone's today.
// So after a zone move no stored strength date may differ from the history's
// in the new zone, either way across the local day.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/strengthdates"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// shiftedDay picks a zone whose today differs from UTC's at now. It also picks
// the date of a day entry that has begun in one of the two zones only.
func shiftedDay(now time.Time) (zone string, entry time.Time) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	if now.Hour() >= 10 {
		return "Etc/GMT-14", today.AddDate(0, 0, 1)
	}
	return "Etc/GMT+12", today
}

func TestAZoneMoveLeavesNoStoredStrengthDateTheHistoryWouldNotGive(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	admin := e.Admin()
	now := time.Now().UTC().Truncate(time.Microsecond)
	zone, entry := shiftedDay(now)
	setZone := func(name string) {
		t.Helper()
		if _, err := compose.NewInstallationSettingsFor(e.Pool).UpdateInstallation(admin, identity.InstallationPatch{Timezone: &name}); err != nil {
			t.Fatalf("saving the installation zone %s: %v", name, err)
		}
	}
	setZone("UTC")
	contact := e.SeedContact(t, "Zone Move Contact", &e.Rep1)
	for _, call := range []struct {
		at        time.Time
		precision string
	}{{now.AddDate(0, 0, -5), activities.PrecisionInstant}, {entry, activities.PrecisionDay}} {
		at, subject := call.at, "Call"
		if _, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
			Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual", OccurredPrecision: call.precision,
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		}); err != nil {
			t.Fatalf("logging a call: %v", err)
		}
	}
	system := principal.SystemActing(admin, "system:strength-dates-test")
	check := func(what string) {
		t.Helper()
		// The read's now comes before its transaction, as a request's does.
		ctx, at := e.Admin(), time.Now()
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			stored, err := contacts.StrengthForContacts(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, at)
			if err != nil {
				return err
			}
			live, err := contacts.StrengthForContactsAsOf(ctx, tx, []ids.ContactID{ids.From[ids.ContactKind](contact)}, at)
			if err != nil {
				return err
			}
			s, l := stored[0].Strength.LastInteraction, live[0].Strength.LastInteraction
			if (s == nil) != (l == nil) || (s != nil && !s.Equal(*l)) {
				t.Errorf("%s: the stored dates give last interaction %v, the history %v", what, s, l)
			}
			return nil
		}); err != nil {
			t.Fatalf("%s: reading the strength: %v", what, err)
		}
	}
	fold := func(what string) {
		t.Helper()
		if _, err := strengthdates.FoldUnfolded(system, e.DB()); err != nil {
			t.Fatalf("%s: folding: %v", what, err)
		}
		check(what + ", once folded again")
	}
	check("in UTC")
	for _, move := range []string{zone, "UTC"} {
		setZone(move)
		check("after the move to " + move)
		fold("after the move to " + move)
	}
}
