// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Which contact an unsubscribe link speaks for.
//
// The send gate and the token mint each resolve a recipient address to a
// contact, and they have to reach the same one: the gate authorizes the send
// against whoever it found, and the mint decides whose consent record the
// emailed link opens. A mint that can name a different contact puts one
// recipient's credential in another recipient's mailbox.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedContactWithEmail creates a second subject in the same workspace carrying
// one address, live or archived, through the columns the real writer uses.
func seedContactWithEmail(t *testing.T, e *channelConsentEnv, address string, archived bool) ids.ContactID {
	t.Helper()
	contactID := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Second Holder', 'test', 'human:x')`, contactID); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	archivedAt := "NULL"
	if archived {
		archivedAt = "now()"
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by, archived_at)
		 VALUES ($1, lower($2), true, 'test', 'human:x', `+archivedAt+`)`,
		contactID, address); err != nil {
		t.Fatalf("seed contact_email: %v", err)
	}
	return contactID
}

// The defect: uq_contact_email_dedupe is partial on archived_at IS NULL, so one
// address can sit archived on one contact and live on another. A lookup that
// does not filter archived rows could resolve to the contact who no longer holds
// the address, and mail THEIR unsubscribe link to the contact who does.
func TestAnUnsubscribeLinkNamesTheLivingHolderOfTheAddress(t *testing.T) {
	e := setupChannelConsent(t)
	address := "shared-" + e.contact.String() + "@example.test"

	// The address as somebody's detached history, seeded FIRST so a lookup
	// deciding by row order would find this one.
	formerHolder := seedContactWithEmail(t, e, address, true)
	// And as the live address of the contact who actually holds it now.
	currentHolder := seedContactWithEmail(t, e, address, false)

	token, found, err := e.store.PreferenceTokenForEmail(e.ctx, address)
	if err != nil {
		t.Fatalf("mint a preference token: %v", err)
	}
	if !found {
		t.Fatal("no token minted for an address one contact live-holds")
	}

	ref, err := e.store.ResolvePreferenceToken(e.ctx, token)
	if err != nil {
		t.Fatalf("resolve the minted token: %v", err)
	}
	if ref.ContactID == formerHolder {
		t.Fatal("the link opens the consent record of the contact who ARCHIVED this address — " +
			"it would reach the current holder's mailbox and speak for somebody else")
	}
	if ref.ContactID != currentHolder {
		t.Fatalf("link names %s, want the live holder %s", ref.ContactID, currentHolder)
	}
}

// An address nobody live-holds carries no unsubscribe surface, which is
// found=false rather than a token for the contact who detached it.
func TestAnArchivedAddressAloneMintsNoLink(t *testing.T) {
	e := setupChannelConsent(t)
	address := "detached-" + e.contact.String() + "@example.test"
	seedContactWithEmail(t, e, address, true)

	_, found, err := e.store.PreferenceTokenForEmail(e.ctx, address)
	if err != nil {
		t.Fatalf("look up an address only held archived: %v", err)
	}
	if found {
		t.Error("minted a link for an address no live contact holds")
	}
}

// The preference centre must SAY which purposes it cannot grant, because the
// page can only withhold a switch it knows about. The defect this holds: the
// wire carried `locked` and nothing else, so a double-opt-in purpose rendered
// as an ordinary toggle and every grant through it was refused.
func TestThePreferenceCentreNamesThePurposesItCannotGrant(t *testing.T) {
	e := setupChannelConsent(t)

	choices, err := e.store.PublicPurposeStates(e.ctx, e.contact)
	if err != nil {
		t.Fatalf("read the preference centre: %v", err)
	}

	byKey := map[string]PurposeChoice{}
	for _, c := range choices {
		byKey[c.Key] = c
	}
	doi, ok := byKey["doi_newsletter"]
	if !ok {
		t.Fatal("the double-opt-in purpose is absent from the centre it is offered in")
	}
	if !doi.GrantNeedsConfirmation {
		t.Error("a double-opt-in purpose does not say its grant needs confirmation — " +
			"the page would offer a switch the write refuses")
	}
	plain, ok := byKey["newsletter"]
	if !ok {
		t.Fatal("the ordinary purpose is absent")
	}
	if plain.GrantNeedsConfirmation {
		t.Error("an ordinary purpose claims its grant needs confirmation, withholding a switch that works")
	}
}
