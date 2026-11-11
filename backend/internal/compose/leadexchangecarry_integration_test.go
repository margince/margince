// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type recordedLeadExchange struct {
	e       *integration.Env
	ctx     context.Context
	records *contacts.Store
	leadID  ids.LeadID
	event   consent.QualifyingEvent
}

const leadExchangeAddress = "recorded-request@promotion.test"

func setupRecordedLeadExchange(t *testing.T) recordedLeadExchange {
	t.Helper()
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, perms)
	evidence := consent.NewStore(e.DB())
	records := e.Contacts.WithStopCarrier(evidence).WithSatelliteCarriers(evidence, nil)
	const address = leadExchangeAddress
	lead, _, err := records.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Requested information"), Email: new(address), Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.AdminUser))})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	event, err := evidence.RecordLeadQualifyingEvent(ctx, leadID, consent.RecordQualifyingEventInput{
		Kind: consent.KindRequestedBySubject, Note: "Asked by phone for the document.", OccurredAt: time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return recordedLeadExchange{e: e, ctx: ctx, records: records, leadID: leadID, event: event}
}

func TestALeadExchangeKeepsItsEvidenceThroughPromotionAndContactMerge(t *testing.T) {
	f := setupRecordedLeadExchange(t)
	e, ctx, records, leadID, event := f.e, f.ctx, f.records, f.leadID, f.event
	const address = leadExchangeAddress
	snapshot := func() string {
		t.Helper()
		var image string
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT (to_jsonb(q)-ARRAY['contact_id','lead_id'])::text
 FROM consent_qualifying_event q WHERE id=@id`, pgx.StrictNamedArgs{"id": event.ID}).Scan(&image)
		}); err != nil {
			t.Fatal(err)
		}
		return image
	}
	before := snapshot()
	promoted, _, err := records.PromoteLead(ctx, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatal(err)
	}
	survivor, err := records.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Surviving contact", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "other@promotion.test", EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	survivorID := ids.From[ids.ContactKind](ids.UUID(survivor.Id))
	if _, err := records.MergeContact(ctx, ids.From[ids.ContactKind](ids.UUID(promoted.Id)), survivorID, nil); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("carrying rewrote recorded scope or evidence:\nbefore %s\nafter %s", before, after)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var contact ids.UUID
		var retiredLead *ids.UUID
		var addresses []string
		err := tx.QueryRow(ctx, `SELECT contact_id,lead_id,recorded_for_addresses FROM consent_qualifying_event WHERE id=@id`, pgx.StrictNamedArgs{"id": event.ID}).Scan(&contact, &retiredLead, &addresses)
		if err == nil && (contact != survivorID.UUID || retiredLead != nil || len(addresses) != 1 || addresses[0] != address) {
			t.Errorf("carried contact=%s lead=%v addresses=%v", contact, retiredLead, addresses)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLeadExchangeProofIsExportedAndRemovedWithItsContactTwin(t *testing.T) {
	for _, historicalAddress := range []bool{false, true} {
		t.Run(fmt.Sprint("historical address ", historicalAddress), func(t *testing.T) {
			checkLeadExchangeExportAndErasure(t, historicalAddress)
		})
	}
}

func checkLeadExchangeExportAndErasure(t *testing.T, historicalAddress bool) {
	t.Helper()
	f := setupRecordedLeadExchange(t)
	address := "historical-subject@erasure.test"
	if historicalAddress {
		address = leadExchangeAddress
		if err := f.e.DB().Tx(f.ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(f.ctx, `UPDATE lead SET email='changed@erasure.test',email_domain_folded='erasure.test' WHERE id=@lead`, pgx.StrictNamedArgs{"lead": f.leadID})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	contact, err := f.records.CreateContact(f.ctx, contacts.CreateContactInput{
		FullName: "Historical subject", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true, Position: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	if !historicalAddress {
		if err := f.e.DB().Tx(f.ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(f.ctx, `UPDATE lead SET from_contact_id=@contact WHERE id=@lead`, pgx.StrictNamedArgs{"contact": contactID, "lead": f.leadID})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	exported, err := privacy.AssembleSAR(f.ctx, f.e.DB(), contactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.ConsentQualifyingEvents) != 1 {
		t.Fatalf("export contains %d recorded exchanges, want the lead's request", len(exported.ConsentQualifyingEvents))
	}
	if _, contains := exported.ConsentQualifyingEvents[0]["note"]; contains {
		t.Fatal("export disclosed a colleague's free-text note")
	}
	if err := eraserWithGraph(f.e.DB()).EraseContact(f.ctx, contactID.UUID, "Subject asked for erasure."); err != nil {
		t.Fatal(err)
	}
	if err := f.e.DB().Tx(f.ctx, func(tx pgx.Tx) error {
		var remaining int
		err := tx.QueryRow(f.ctx, `SELECT count(*) FROM consent_qualifying_event WHERE lead_id=@lead`, pgx.StrictNamedArgs{"lead": f.leadID}).Scan(&remaining)
		if err == nil && remaining != 0 {
			t.Errorf("erasure left %d lead exchanges", remaining)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
