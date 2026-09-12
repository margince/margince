// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Which address the preference page names.
//
// The token used to carry only contact_id, so the page masked whichever address
// the record calls primary. For a contact holding more than one — a personal
// address and a shared team one, say — a link delivered to the second opened a
// page showing the first character and the full domain of the first. The
// holder of that link was never written at that address, and the link stays
// valid for thirty days.
//
// The mint knows the address: it resolved the recipient FROM it. So the token
// records which one, keyed on the pair rather than on the contact, and the page
// reads it back.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// address gives the env's contact a live address. The shared harness seeds a
// Telegram-only subject with no contact_email row at all, which is the right
// default for the suites around this one and no use here: the disclosure this
// file is about needs a contact who holds MORE THAN ONE address.
func address(t *testing.T, e *channelConsentEnv, email string, primary bool) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_email (id, contact_id, email, is_primary, source, captured_by)
		 VALUES ($1, $2, lower($3), $4, 'manual', 'system:test')`,
		ids.NewV7(), e.contact, email, primary); err != nil {
		t.Fatalf("seeding %s: %v", email, err)
	}
}

func TestThePreferencePageNamesTheAddressTheLinkWentTo(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:public_preferences",
	})

	const own = "hedda@anon.test"
	const shared = "team@anon.test"
	address(t, e, own, true)
	address(t, e, shared, false)

	// Minted the way a send mints it: from the address the message is going to.
	token, found, err := e.store.PreferenceTokenForEmail(ctx, shared)
	if err != nil {
		t.Fatalf("minting the token: %v", err)
	}
	if !found {
		t.Fatal("no token for a live address the contact carries")
	}

	ref, err := e.store.ResolvePreferenceToken(ctx, token)
	if err != nil {
		t.Fatalf("resolving the token: %v", err)
	}
	view, err := e.store.PublicPreferenceView(ctx, ref)
	if err != nil {
		t.Fatalf("reading the page: %v", err)
	}

	if want := MaskEmail(shared); view.MaskedEmail != want {
		t.Errorf("the page names %q, want %q — the link went to %s, and naming any other address "+
			"discloses one its holder was never written at",
			view.MaskedEmail, want, shared)
	}
}

// A token minted before the address was recorded still opens a working page.
// There is no way to learn after the fact which address it went to, so the
// primary is the honest fallback — and it is what the page always showed.
func TestAPreferenceTokenWithNoAddressStillOpensThePage(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:public_preferences",
	})

	const own = "hedda@anon.test"
	address(t, e, own, true)

	token := seedPreferenceToken(t, e)
	ref, err := e.store.ResolvePreferenceToken(ctx, token)
	if err != nil {
		t.Fatalf("resolving the token: %v", err)
	}
	if ref.EmailID != nil {
		t.Fatalf("the seeded token names an address (%s) — this case is about the ones that do not",
			*ref.EmailID)
	}
	view, err := e.store.PublicPreferenceView(ctx, ref)
	if err != nil {
		t.Fatalf("reading the page: %v", err)
	}
	if want := MaskEmail(own); view.MaskedEmail != want {
		t.Errorf("the page names %q, want the primary %q — a token minted before the column "+
			"existed must still open, and the primary is what it always showed",
			view.MaskedEmail, want)
	}
}

// Two addresses on one contact are two credentials, not one reused. A single
// live token per CONTACT would hand the second send the first send's token, and
// the page would name the first address again — the defect one level down from
// the column.
func TestEachAddressGetsItsOwnPreferenceToken(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:public_preferences",
	})

	const own = "hedda@anon.test"
	const shared = "team@anon.test"
	address(t, e, own, true)
	address(t, e, shared, false)

	first, foundFirst, err := e.store.PreferenceTokenForEmail(ctx, own)
	if err != nil {
		t.Fatalf("minting for the primary address: %v", err)
	}
	second, foundSecond, err := e.store.PreferenceTokenForEmail(ctx, shared)
	if err != nil {
		t.Fatalf("minting for the shared address: %v", err)
	}
	// Both have to have been MINTED, or the inequality below is two empty
	// strings failing to differ and the case proves nothing.
	if !foundFirst || !foundSecond {
		t.Fatalf("minted %t and %t: an address the contact carries must yield a token",
			foundFirst, foundSecond)
	}
	if first == second {
		t.Fatal("both addresses were handed one token, so the second send's link opens a page " +
			"naming the first address — which is the disclosure this change is about")
	}

	// And the reuse a send depends on still holds: asking again for the same
	// address answers the same credential rather than rotating it, or the link
	// in a message already delivered stops working.
	again, foundAgain, err := e.store.PreferenceTokenForEmail(ctx, shared)
	if err != nil {
		t.Fatalf("minting again for the shared address: %v", err)
	}
	if !foundAgain {
		t.Fatal("the second ask for one address yielded no token at all")
	}
	if again != second {
		t.Error("a second send to one address minted a new token: the link in the message already " +
			"delivered is dead, which is what the reuse exists to prevent")
	}
}
