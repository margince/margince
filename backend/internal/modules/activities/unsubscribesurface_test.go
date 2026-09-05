// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which messages offer the recipient a way to stop.
//
// The case these exist for: a reply that carries an unsubscribe footer. It is
// not a crash and no gate refuses it — the message goes out, correctly
// addressed, offering somebody the chance to opt out of a conversation they
// started. The only thing that catches it is a test that says which categories
// carry the surface and which do not.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestOnlyMarketingCarriesAnUnsubscribeSurface(t *testing.T) {
	t.Parallel()

	for _, category := range commsauthz.Categories() {
		want := category == commsauthz.CategoryMarketing
		if got := category.CarriesUnsubscribe(); got != want {
			t.Errorf("%s.CarriesUnsubscribe() = %v, want %v. An unsubscribe surface is an offer "+
				"to stop a stream the recipient can decline; every other category is a message "+
				"they cannot decline without declining the relationship", category, got, want)
		}
	}
}

// TestAReplyCarriesNoUnsubscribeSurface is the regression this change exists
// for, stated in the shape the composer now sends.
//
// A reply claims no category and carries no purpose key, because the engine
// derives reply_to_inbound from the anchor. Under the old rule that message was
// "not the locked transactional purpose", which resolved a token and wrote a
// footer.
func TestAReplyCarriesNoUnsubscribeSurface(t *testing.T) {
	t.Parallel()

	reply := surfaceFor(commsauthz.CategoryReplyToInbound, "", "")
	if reply.carries() {
		t.Error("a reply offers an unsubscribe link, so the recipient is invited to opt out of " +
			"the conversation they themselves started")
	}
}

func TestAMarketingSendCarriesItsTopicsSurface(t *testing.T) {
	t.Parallel()

	send := surfaceFor(commsauthz.CategoryMarketing, "product_news", "")
	if !send.carries() {
		t.Fatal("a marketing send carries no unsubscribe surface, which a mailbox provider " +
			"reads as unsolicited bulk mail")
	}
	// The link points at the TOPIC, because marketing consent is
	// purpose-specific: unsubscribing from product news is not unsubscribing
	// from everything.
	if send.purposeKey != "product_news" {
		t.Errorf("the unsubscribe link points at %q, want the marketing topic", send.purposeKey)
	}
}

// TestACallerThatNamesOnlyAKeyKeepsItsSurface holds the compatibility arm. An
// MCP tool or a stored scheduled send written before categories existed names a
// purpose key and no category, and must keep the behaviour it was written
// against.
func TestACallerThatNamesOnlyAKeyKeepsItsSurface(t *testing.T) {
	t.Parallel()

	legacy := surfaceFor("", "", "marketing_email")
	if !legacy.carries() {
		t.Error("a caller naming only a marketing purpose key lost its unsubscribe surface, so " +
			"a send that used to be compliant bulk mail now is not")
	}

	locked := surfaceFor("", "", "transactional")
	if locked.carries() {
		t.Error("the locked transactional key gained an unsubscribe surface")
	}

	// And the case that started this: no category, no key at all. Under the old
	// rule this was "not transactional" and therefore carried a footer.
	if surfaceFor("", "", "").carries() {
		t.Error("a send naming neither a category nor a key carries an unsubscribe surface — " +
			"which is exactly the reply case, reached through the legacy arm")
	}
}

// TestTheLegacyLockedKeyAgreesWithConsent pins the one key this package spells
// for itself against the module that owns the concept.
//
// activities may not import consent, so the value is repeated — and a repeated
// value is the thing that drifts. consent.LockedPurpose normalizes before
// comparing, so this does too.
func TestTheLegacyLockedKeyAgreesWithConsent(t *testing.T) {
	t.Parallel()

	for _, spelling := range []string{"transactional", "Transactional", "  TRANSACTIONAL  "} {
		if !lockedPurposeKey(spelling) {
			t.Errorf("lockedPurposeKey(%q) = false; consent.LockedPurpose normalizes case and "+
				"space before comparing, and a stricter copy here would put an unsubscribe "+
				"footer on a transactional send", spelling)
		}
	}
	if lockedPurposeKey("marketing_email") {
		t.Error("lockedPurposeKey treats a marketing key as locked, which would strip the " +
			"unsubscribe surface from real bulk mail")
	}
}
