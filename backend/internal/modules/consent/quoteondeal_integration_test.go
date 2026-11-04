// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What a quote on a named deal rests on, and whom a request reaches.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// offerOn plants an offer on a deal in one status.
func (e *resolveEnv) offerOn(t *testing.T, deal ids.UUID, status string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	var accepted *time.Time
	if status == "accepted" {
		now := clocktest.Now(t)
		accepted = &now
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO offer (id, deal_id, offer_number, status, accepted_at, currency, net_minor, gross_minor, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'EUR', 100, 100, 'manual', 'human:x')`,
		id, deal, fmt.Sprintf("A-%d", binary.BigEndian.Uint32(id[12:])), status, accepted); err != nil {
		t.Fatalf("planting the offer: %v", err)
	}
	return id
}

// ownerExec runs one fixture statement as the owner.
func (e *resolveEnv) ownerExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("fixture %q: %v", sql, err)
	}
}

// anotherContact plants a second contact with its own address.
func (e *resolveEnv) anotherContact(t *testing.T, address string) *resolveEnv {
	t.Helper()
	other := *e
	other.contact, other.address = ids.New[ids.ContactKind](), address
	e.ownerExec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Other Contact', 'manual', 'human:x')`, other.contact)
	e.ownerExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:x')`, other.contact, address)
	return &other
}

// quoteOn asks the engine about a quote naming a deal, with no request on record.
func (e *resolveEnv) quoteOn(t *testing.T, deal ids.UUID) resolution {
	t.Helper()
	return e.resolve(t, commsauthz.Request{
		Context: commsauthz.CategoryPrecontractQuote, Evidence: commsauthz.Evidence{DealID: deal},
	})
}

// quoteRefusedOn reports whether a quote naming the deal is refused. A deal the
// caller can no longer reach is refused as not found, which is a refusal too.
func (e *resolveEnv) quoteRefusedOn(t *testing.T, deal ids.UUID) bool {
	t.Helper()
	var out resolution
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.resolveCategory(e.ctx, tx, commsauthz.Request{
			Context: commsauthz.CategoryPrecontractQuote, Evidence: commsauthz.Evidence{DealID: deal},
		}, subjectRef{Kind: entityContact, ID: e.contact.String(), Address: e.address})
		return err
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return true
	}
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	return !out.Supported
}

// A sent or accepted offer on an open deal to a current stakeholder bears out
// the quote. Every other shape of offer, deal or stakeholder does not.
//
// Mutation: drop any one clause of quoteOnDeal's query and its own case is
// allowed.
func TestAQuoteRestsOnASentOfferToACurrentStakeholderOfAnOpenDeal(t *testing.T) {
	for _, status := range []string{"sent", "accepted"} {
		t.Run("allowed: "+status, func(t *testing.T) {
			e := setupResolve(t)
			deal := e.openDeal(t, "open", true)
			e.offerOn(t, deal, status)
			if got := e.quoteOn(t, deal); !got.Supported || got.Basis != commsauthz.BasisPrecontractRequest {
				t.Fatalf("a %s offer on an open deal did not bear out the quote: %+v", status, got)
			}
		})
	}
	for name, spoil := range map[string]func(t *testing.T, e *resolveEnv) ids.UUID{
		"draft offer":      func(t *testing.T, e *resolveEnv) ids.UUID { return e.dealWithOffer(t, "draft") },
		"rejected offer":   func(t *testing.T, e *resolveEnv) ids.UUID { return e.dealWithOffer(t, "rejected") },
		"expired offer":    func(t *testing.T, e *resolveEnv) ids.UUID { return e.dealWithOffer(t, "expired") },
		"superseded offer": func(t *testing.T, e *resolveEnv) ids.UUID { return e.dealWithOffer(t, "superseded") },
		"archived offer": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.openDeal(t, "open", true)
			offer := e.offerOn(t, deal, "sent")
			e.ownerExec(t, `UPDATE offer SET archived_at = now() WHERE id = $1`, offer)
			return deal
		},
		"closed deal": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.openDeal(t, "won", true)
			e.offerOn(t, deal, "sent")
			return deal
		},
		"archived deal": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.dealWithOffer(t, "sent")
			e.ownerExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, deal)
			return deal
		},
		"not a stakeholder": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.openDeal(t, "open", false)
			e.offerOn(t, deal, "sent")
			return deal
		},
		"ended stakeholder": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.dealWithOffer(t, "sent")
			e.ownerExec(t, `UPDATE relationship SET ended_at = now() WHERE deal_id = $1`, deal)
			return deal
		},
		"removed stakeholder": func(t *testing.T, e *resolveEnv) ids.UUID {
			deal := e.dealWithOffer(t, "sent")
			e.ownerExec(t, `UPDATE relationship SET archived_at = now() WHERE deal_id = $1`, deal)
			return deal
		},
		"offer on a different deal": func(t *testing.T, e *resolveEnv) ids.UUID {
			named := e.openDeal(t, "open", true)
			e.offerOn(t, e.openDeal(t, "open", true), "sent")
			return named
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			e := setupResolve(t)
			if !e.quoteRefusedOn(t, spoil(t, e)) {
				t.Fatalf("a quote on a %s was borne out", name)
			}
		})
	}
}

// dealWithOffer plants an open deal the contact is a stakeholder on, with one
// offer in status.
func (e *resolveEnv) dealWithOffer(t *testing.T, status string) ids.UUID {
	t.Helper()
	deal := e.openDeal(t, "open", true)
	e.offerOn(t, deal, status)
	return deal
}

// A request reaches the contact who made it and nobody else. A colleague copied
// on the same message and another contact at their company are not reached.
//
// Mutation: drop the contact_id clause from requestOnRecord and both others
// are allowed.
func TestARequestReachesOnlyTheContactWhoMadeIt(t *testing.T) {
	e := setupResolve(t)
	e.recordRequest(t, clocktest.Now(t).Add(-time.Hour), true)
	company := e.company(t)
	e.employ(t, company)
	copied := e.anotherContact(t, "copied@buyer.test")
	colleague := e.anotherContact(t, "colleague@buyer.test")
	colleague.employ(t, company)

	for _, category := range []commsauthz.Category{commsauthz.CategoryRequestedFollowup, commsauthz.CategoryPrecontractQuote} {
		var set commsauthz.DecisionSet
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			for _, r := range []connector.Recipient{{Email: e.address}, {Email: copied.address}, {Email: colleague.address}} {
				d, err := e.gate.decideOne(e.ctx, tx, r, commsauthz.Request{Context: category}, commsauthz.PhaseStaging)
				if err != nil {
					return err
				}
				set.Decisions = append(set.Decisions, d)
			}
			return nil
		}); err != nil {
			t.Fatalf("deciding: %v", err)
		}
		if set.Decisions[0].Verdict != commsauthz.VerdictAllow {
			t.Errorf("%s: the contact who asked was refused: %q", category, set.Decisions[0].ReasonCode)
		}
		for i, who := range []string{"the copied contact", "the colleague at the company"} {
			if d := set.Decisions[i+1]; d.Verdict == commsauthz.VerdictAllow {
				t.Errorf("%s: %s was allowed on somebody else's request", category, who)
			}
		}
	}
}

// A quote queued on a request, withdrawn before it goes, is refused at transmit.
//
// Mutation: drop "retracted_at IS NULL" from requestOnRecord and the second
// ticket still allows the quote.
func TestAQueuedQuoteWhoseRequestIsWithdrawnIsRefusedAtTransmit(t *testing.T) {
	e := setupResolve(t)
	event := recordedID(t, e.recordRequest(t, clocktest.Now(t).Add(-time.Hour), true))
	delivery := e.plantDelivery(t)
	recipients := []connector.Recipient{{Email: e.address}}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		set, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients, Context: commsauthz.CategoryPrecontractQuote, Subject: "S", Body: "b",
		})
		if err != nil {
			return err
		}
		if d := set.Decisions[0]; d.Verdict != commsauthz.VerdictAllow {
			t.Fatalf("staging decision = %+v, want the requested quote allowed", d)
		}
		return nil
	}); err != nil {
		t.Fatalf("staging the quote: %v", err)
	}
	transmit := func(attempt int) commsauthz.TransmitTicket {
		ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients, Subject: "S", Body: "b",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		return ticket
	}

	if ticket := transmit(1); !ticket.Allowed {
		t.Fatalf("the queued quote was refused before the withdrawal: %q", ticket.Reason)
	}
	if err := e.store.RetractQualifyingEvent(e.seatCtx(e.user), e.contact, event); err != nil {
		t.Fatalf("withdrawing the request: %v", err)
	}
	if ticket := transmit(2); ticket.Allowed {
		t.Fatal("the queued quote went out after its request was withdrawn")
	}
}
