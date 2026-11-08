// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A lead who pressed the all-marketing link stays stopped once promoted: the
// contact it becomes reads as suppressed for marketing. The control lead never
// pressed it, so its contact's marketing answer carries no stop.
func TestALeadsStopHoldsForTheContactItBecomes(t *testing.T) {
	for _, pressed := range []bool{false, true} {
		t.Run(map[bool]string{false: "a lead that never stopped", true: "a lead that pressed the link"}[pressed], func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			consents := consent.NewStore(e.DB())
			store := e.Contacts.WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
			newsletter, err := consents.CreatePurpose(ctx, "newsletter", "Newsletter", false)
			if err != nil {
				t.Fatal(err)
			}
			const address = "promoted-stop@example.test"
			lead, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Lee Stop"), Email: new(address), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.AdminUser))})
			if err != nil {
				t.Fatal(err)
			}
			leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
			if _, err := consents.Record(ctx, consent.RecordInput{LeadID: leadID, PurposeID: newsletter.ID, NewState: "granted", PolicyText: &grantedWording}); err != nil {
				t.Fatal(err)
			}
			if pressed {
				if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
					_, err := consents.StopForCredentialTx(ctx, tx, consent.WithdrawalRef{LeadID: leadID, Address: address, Scope: consent.WithdrawalScopeAllMarketing})
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			contact, _, err := store.PromoteLead(ctx, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
			if err != nil {
				t.Fatal(err)
			}

			guard, err := consents.ContactConsentGuard(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
			if err != nil {
				t.Fatal(err)
			}
			var reason string
			for _, entry := range guard.Entries {
				if entry.PurposeKey == "newsletter" {
					reason = string(entry.ReasonCode)
				}
			}
			if stopped := reason == consent.ReasonSuppressedObjection; stopped != pressed {
				t.Fatalf("the promoted contact's newsletter answer is %q; the lead pressed the link: %v", reason, pressed)
			}
		})
	}
}
