// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func boughtFieldOf(t *testing.T, e *Env, store *contacts.Store, subject ids.UUID, target string) crmcontracts.BoughtField {
	t.Helper()
	c, err := store.GetContact(e.Admin(), ids.From[ids.ContactKind](subject), storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range *c.BoughtFields {
		if f.Target == target {
			return f
		}
	}
	t.Fatalf("no bought_fields entry for %q", target)
	return crmcontracts.BoughtField{}
}

func TestAProviderValueIsPaidOnlyWhenAPersonsRunSpentCredits(t *testing.T) {
	cases := map[string]struct {
		trigger     string
		credits     int
		wantPaid    bool
		wantTrigger crmcontracts.ProviderRunTrigger
		wantSeat    bool
	}{
		"catch-up sweep":          {"automatic_backfill", 0, false, crmcontracts.ProviderRunTriggerAutomaticBackfill, false},
		"new contact":             {"automatic_create", 0, false, crmcontracts.ProviderRunTriggerAutomaticCreate, false},
		"person without spending": {"manual", 0, false, crmcontracts.ProviderRunTriggerManual, true},
		"person who spent":        {"manual", 3, true, crmcontracts.ProviderRunTriggerManual, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			store := contacts.NewStore(e.DB())
			subject := ids.NewV7()
			execAsOwner(t, e, `INSERT INTO contact (id, full_name, source, captured_by)
				VALUES ($1, 'Paid Subject', 'manual', 'human:x')`, subject)
			applyRun(t, e, subject, "surfe", tc.trigger, tc.credits, []provider.Claim{
				{Key: provider.ClaimLinkedInProfile, Value: []byte(`"https://www.linkedin.com/in/paid-subject"`)},
			})
			got := boughtFieldOf(t, e, store, subject, "linkedin")
			if got.Paid != tc.wantPaid || got.Trigger != tc.wantTrigger || (got.RequestedBy != nil) != tc.wantSeat {
				t.Fatalf("got paid=%v trigger=%s requested_by=%v, want paid=%v trigger=%s seat=%v",
					got.Paid, got.Trigger, got.RequestedBy, tc.wantPaid, tc.wantTrigger, tc.wantSeat)
			}
		})
	}
}
