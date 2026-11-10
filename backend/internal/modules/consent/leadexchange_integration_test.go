// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestLeadExchangesKeepTheirAddressCompanyQuoteAndRetractionScope(t *testing.T) {
	e := setupLeadConsent(t)
	at := time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC)
	request, err := e.store.RecordLeadQualifyingEvent(e.ctx, e.lead, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked by phone for the document.", OccurredAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	var entity ids.UUID
	if err := e.owner.QueryRow(t.Context(), `SELECT recorded_for_entities[1] FROM consent_qualifying_event WHERE id=@id`, pgx.StrictNamedArgs{"id": request.ID}).Scan(&entity); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, address string
		entity        ids.UUID
		quote, want   bool
	}{
		{"recorded recipient", e.leadEmail, entity, false, true},
		{"another address", "other@lead.test", entity, false, false},
		{"another company", e.leadEmail, ids.NewV7(), false, false},
		{"no quote requested", e.leadEmail, entity, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := followupRequests
			if tc.quote {
				scope = quoteRequests
			}
			var found bool
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				var err error
				found, err = requestOnRecord(e.ctx, tx, subjectRef{Kind: entityLead, ID: e.lead.String(), Address: tc.address, SendingEntity: tc.entity}, scope)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if found != tc.want {
				t.Fatalf("request found=%v, want %v", found, tc.want)
			}
		})
	}
	contact := ids.From[ids.ContactKind](e.lead.UUID)
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO contact(id,full_name,source,captured_by)
 VALUES(@id,'A different contact with the same UUID','manual','human:fixture')`, pgx.StrictNamedArgs{"id": contact}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.RecordQualifyingEvent(e.ctx, contact, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "A different contact asked.", OccurredAt: at, AsksForQuote: true,
	}); err != nil {
		t.Fatal(err)
	}
	eventID, err := ids.Parse(request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.RetractLeadQualifyingEvent(e.ctx, e.lead, eventID); err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		found, err := requestOnRecord(e.ctx, tx, subjectRef{Kind: entityLead, ID: e.lead.String(), Address: e.leadEmail, SendingEntity: entity}, followupRequests)
		if err == nil && found {
			t.Error("the lead borrowed a contact request after its own was retracted")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestALeadQuoteKeepsItsCheckedDealThroughStagingAndTransmit(t *testing.T) {
	e := setupResolve(t)
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Update: true}
	actor.Permissions.Objects["deal"] = principal.ObjectGrant{Read: true}
	e.ctx = principal.WithActor(e.ctx, actor)
	lead := ids.New[ids.LeadKind]()
	const address = "quote-request@lead.test"
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO lead(id,full_name,email,source,captured_by) VALUES(@id,'Quote requester',@address,'manual','human:fixture')`, pgx.StrictNamedArgs{"id": lead, "address": address}); err != nil {
		t.Fatal(err)
	}
	deal := e.openDeal(t, "open", false)
	recipient := connector.Recipient{Email: address}
	req := commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote, Evidence: commsauthz.Evidence{DealID: deal}, Recipients: []connector.Recipient{recipient}}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		denied, err := e.gate.decideOne(e.ctx, tx, recipient, req, commsauthz.PhaseStaging)
		if err == nil && (denied.Verdict == commsauthz.VerdictAllow || denied.Evidence.DealID != deal) {
			t.Errorf("refused quote lost its checked scope: %+v", denied)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.RecordLeadQualifyingEvent(e.ctx, lead, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked for this deal's quote.", OccurredAt: time.Date(2020, time.March, 2, 9, 0, 0, 0, time.UTC), AsksForQuote: true, AboutDeal: deal,
	}); err != nil {
		t.Fatal(err)
	}
	delivery := e.plantDelivery(t)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		staged, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
		if err != nil {
			return err
		}
		if len(staged.Decisions) != 1 || staged.Decisions[0].Verdict != commsauthz.VerdictAllow {
			t.Fatalf("staging=%+v", staged.Decisions)
		}
		claims, err := stagedClaims(e.ctx, tx, delivery)
		if err != nil {
			return err
		}
		rebuilt := stagedRequestFor(commsauthz.TransmitRequest{DeliveryID: delivery}, recipient, claims, "", nil)
		if rebuilt.Evidence.DealID != deal {
			t.Fatalf("transmit lost deal: %+v", rebuilt.Evidence)
		}
		decision, err := e.gate.decideOne(e.ctx, tx, recipient, rebuilt, commsauthz.PhaseTransmit)
		if err == nil && decision.Verdict != commsauthz.VerdictAllow {
			t.Errorf("transmit=%+v", decision)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
