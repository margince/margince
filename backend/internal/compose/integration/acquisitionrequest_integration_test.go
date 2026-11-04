// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A request a contact's acquisition records, withdrawn: the contacts module
// owns the row and the retraction, and the consent engine stops resting on it.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// queuedFollowUp stages a requested follow-up to address and answers a way to
// ask the transmit phase about it.
func queuedFollowUp(t *testing.T, e *Env, gate *consent.Gate, address string) func(attempt int) commsauthz.TransmitTicket {
	t.Helper()
	ctx := context.Background()
	activity, delivery := ids.NewV7(), ids.NewV7()
	owner := OwnerConn(t)
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'email', 'outbound', 'manual', now(), 'human:x')`, activity); err != nil {
		t.Fatalf("planting the activity: %v", err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		   consent_purpose, references_chain, status)
		VALUES ($1, $2, $3, 'gmail', $4, '[]'::jsonb, '[]'::jsonb, 'S', 'b',
		        'business_correspondence', '[]'::jsonb, 'pending')`,
		delivery, activity, e.AdminUser, "msg-"+delivery.String()); err != nil {
		t.Fatalf("planting the delivery: %v", err)
	}
	recipients := []connector.Recipient{{Email: address}}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		set, err := gate.AuthorizeStagingTx(e.Admin(), tx, delivery, commsauthz.Request{
			Recipients: recipients, Context: commsauthz.CategoryRequestedFollowup, Subject: "S", Body: "b",
		})
		if err != nil {
			return err
		}
		if d := set.Decisions[0]; d.Verdict != commsauthz.VerdictAllow {
			t.Fatalf("staging decision = %+v, want the requested follow-up allowed", d)
		}
		return nil
	}); err != nil {
		t.Fatalf("staging the follow-up: %v", err)
	}
	return func(attempt int) commsauthz.TransmitTicket {
		ticket, err := gate.AuthorizeTransmit(e.Admin(), commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients, Subject: "S", Body: "b",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		return ticket
	}
}

// A contact acquired because they asked for a quote may be followed up with no
// age limit. Once the request is withdrawn, a follow-up already queued on it is
// refused at transmit. The acquisition row stays as the record of how the
// contact was obtained. Both request kinds are covered.
//
// Mutation: drop "e.request_retracted_at IS NULL" from requestOnRecord and the
// second ticket still allows the message.
func TestAWithdrawnAcquisitionRequestStopsAQueuedFollowUp(t *testing.T) {
	for _, kind := range []string{contacts.AcquiredRequestedQuoteOrMeeting, contacts.AcquiredInPersonPermission} {
		t.Run(kind, func(t *testing.T) {
			e := Setup(t)
			address := "asked-" + kind + "@acquired.test"
			created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
				FullName: "Asked Somebody", Source: "manual",
				Acquisition: contacts.Acquisition{Kind: kind},
				Emails:      []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
			})
			if err != nil {
				t.Fatalf("creating the contact: %v", err)
			}
			contact := ids.From[ids.ContactKind](ids.UUID(created.Id))
			transmit := queuedFollowUp(t, e, consent.NewGate(consent.NewStore(e.DB())), address)

			if ticket := transmit(1); !ticket.Allowed {
				t.Fatalf("the queued follow-up was refused before the withdrawal: %q", ticket.Reason)
			}
			if err := e.Contacts.RetractAcquisitionRequest(e.Admin(), contact); err != nil {
				t.Fatalf("withdrawing the request: %v", err)
			}
			if ticket := transmit(2); ticket.Allowed {
				t.Fatal("the queued follow-up went out after its request was withdrawn")
			}
			if err := e.Contacts.RetractAcquisitionRequest(e.Admin(), contact); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("a second withdrawal: err = %v, want not found", err)
			}
			var kept int
			if err := OwnerConn(t).QueryRow(context.Background(), `
				SELECT count(*) FROM contact_acquisition_evidence
				 WHERE contact_id = $1 AND kind = $2 AND request_retracted_at IS NOT NULL`,
				contact, kind).Scan(&kept); err != nil {
				t.Fatalf("reading the acquisition row: %v", err)
			}
			if kept != 1 {
				t.Errorf("%d withdrawn acquisition rows, want the one row kept as the record", kept)
			}
		})
	}
}
