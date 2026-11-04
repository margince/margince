// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A request a contact made covers the addresses of the record it was made on,
// also after that record is merged. A wrong merge must not give one human's
// request to the other's addresses, in either direction.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	askerAddress = "asker@merge.test"
	otherAddress = "other@merge.test"
)

func TestAMergedRequestCoversOnlyTheAddressesItWasMadeOn(t *testing.T) {
	requests := map[string]func(t *testing.T, e *integration.Env, store *consent.Store, asker ids.UUID){
		"recorded by the rep": func(t *testing.T, e *integration.Env, store *consent.Store, asker ids.UUID) {
			if _, err := store.RecordQualifyingEvent(e.Admin(), ids.From[ids.ContactKind](asker), consent.RecordQualifyingEventInput{
				Kind: consent.KindRequestedBySubject, Note: "Asked for a quote on the phone.",
				OccurredAt: time.Now().Add(-time.Hour), AsksForQuote: true,
			}); err != nil {
				t.Fatalf("recording the request: %v", err)
			}
		},
		"recorded when acquired": func(*testing.T, *integration.Env, *consent.Store, ids.UUID) {},
	}
	for name, record := range requests {
		for _, askerSurvives := range []bool{false, true} {
			t.Run(name+survivorLabel(askerSurvives), func(t *testing.T) {
				e := integration.Setup(t)
				store := consent.NewStore(InstallationDB(e.Pool))
				acquisition := contacts.Acquisition{}
				if name == "recorded when acquired" {
					acquisition.Kind = contacts.AcquiredRequestedQuoteOrMeeting
				}
				asker := seedMergeContact(t, e, "Asker", askerAddress, acquisition)
				other := seedMergeContact(t, e, "Other", otherAddress, contacts.Acquisition{})
				record(t, e, store, asker)

				source, survivor := asker, other
				if askerSurvives {
					source, survivor = other, asker
				}
				merging := withContactMergeCarriers(InstallationDB(e.Pool), e.Contacts)
				if _, err := merging.MergeContact(e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](survivor), nil); err != nil {
					t.Fatalf("merging: %v", err)
				}

				gate := consent.NewGate(store)
				if d := quoteVerdictFor(t, e, gate, askerAddress); d.Verdict != commsauthz.VerdictAllow {
					t.Errorf("a quote to the address that asked = %s %q, want allowed", d.Verdict, d.ReasonCode)
				}
				if d := quoteVerdictFor(t, e, gate, otherAddress); d.Verdict == commsauthz.VerdictAllow {
					t.Errorf("a quote to the other record's address was allowed on a request it never made (%s)", d.Basis)
				}
			})
		}
	}
}

func survivorLabel(askerSurvives bool) string {
	if askerSurvives {
		return ", the asker survives"
	}
	return ", the asker is merged away"
}

func seedMergeContact(t *testing.T, e *integration.Env, name, address string, acquisition contacts.Acquisition) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Acquisition: acquisition,
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

func quoteVerdictFor(t *testing.T, e *integration.Env, gate *consent.Gate, address string) commsauthz.Decision {
	t.Helper()
	set, err := gate.Preview(e.Admin(), commsauthz.Request{
		Recipients: []connector.Recipient{{Email: address}},
		Context:    commsauthz.CategoryPrecontractQuote,
	})
	if err != nil {
		t.Fatalf("asking the gate about %s: %v", address, err)
	}
	if len(set.Decisions) != 1 {
		t.Fatalf("asked about one recipient, got %d decisions", len(set.Decisions))
	}
	return set.Decisions[0]
}
