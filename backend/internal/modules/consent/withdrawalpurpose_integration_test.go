// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Which purpose an unsubscribe link may name, and what its press stops. The
// mint and the press read one rule. A link the product built never answers its
// own press with an error, or with a stop it did not make.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedClassedPurpose writes a purpose of the given class, archived or live.
func seedClassedPurpose(t *testing.T, e *channelConsentEnv, key, class string, archived bool) ids.PurposeID {
	t.Helper()
	var id ids.PurposeID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO consent_purpose (key, label, class, archived_at)
		VALUES ($1, $1, $2, CASE WHEN $3 THEN now() END) RETURNING id`, key, class, archived).Scan(&id); err != nil {
		t.Fatalf("seeding purpose %s: %v", key, err)
	}
	return id
}

// A link names a purpose only when it is a live marketing purpose. Every other
// key gets the all-marketing credential, whose link names none.
func TestTheMintNamesOnlyAPurposeALinkMayName(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	address := "subject-" + e.contact.String() + "@example.test"
	seedClassedPurpose(t, e, "wp_news", "marketing", false)
	seedClassedPurpose(t, e, "wp_corr", "business_correspondence", false)
	seedClassedPurpose(t, e, "wp_retired", "marketing", true)

	for key, want := range map[string]string{
		"": "", "no_such_purpose": "", "wp_corr": "", "wp_retired": "", "wp_news": "wp_news",
	} {
		_, purpose, _, err := e.store.WithdrawalTokenForEmail(e.ctx, address, key)
		if err != nil {
			t.Fatalf("minting for %q: %v", key, err)
		}
		if purpose != want {
			t.Errorf("a send under %q got a link naming %q, want %q", key, purpose, want)
		}
	}
}

// An all-marketing credential's press naming a key no link may name stops all
// marketing with a 200. It never stops correspondence, and never answers 4xx.
func TestAnAllMarketingPressNamingAnUnlinkableKeyStopsAllMarketing(t *testing.T) {
	for _, key := range []string{"wp_corr", "wp_retired", "no_such_purpose"} {
		t.Run(key, func(t *testing.T) {
			e := setupChannelConsent(t)
			seedSubjectAddress(t, e)
			news := seedClassedPurpose(t, e, "wp_news", "marketing", false)
			corr := seedClassedPurpose(t, e, "wp_corr", "business_correspondence", false)
			seedClassedPurpose(t, e, "wp_retired", "marketing", true)
			token := mintWithdrawal(t, e, WithdrawalMintInput{
				Address: "subject-" + e.contact.String() + "@example.test", ContactID: e.contact,
				Scope: WithdrawalScopeAllMarketing,
			})

			named := key
			if stopped := pressUnsubscribe(t, e, token, &named, "List-Unsubscribe=One-Click"); len(stopped) == 0 {
				t.Error("the press answered no stop, which the page shows as already switched off")
			}
			if got := consentStateOf(t, e, news); got != string(StateWithdrawn) {
				t.Errorf("marketing is %q after the press, want withdrawn", got)
			}
			if got := consentStateOf(t, e, corr); got == string(StateWithdrawn) {
				t.Error("the press withdrew correspondence, which ends replies to the recipient's own enquiries")
			}
		})
	}
}

// A named credential stops its purpose by id. A stored key in mixed case must
// not miss a lookup and widen the press to all marketing.
func TestANamedCredentialWithAMixedCaseKeyStopsOnlyItsPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	news := seedClassedPurpose(t, e, "wp_news", "marketing", false)
	promo := seedClassedPurpose(t, e, "WP_Promo", "marketing", false)
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: "subject-" + e.contact.String() + "@example.test", ContactID: e.contact,
		Scope: WithdrawalScopeNamedPurpose, PurposeID: promo.UUID,
	})

	if stopped := pressUnsubscribe(t, e, token, nil, "List-Unsubscribe=One-Click"); len(stopped) != 1 {
		t.Errorf("the press stopped %d purposes, want the one the link was minted for", len(stopped))
	}
	if got := consentStateOf(t, e, promo); got != string(StateWithdrawn) {
		t.Errorf("the link's own purpose is %q, want withdrawn", got)
	}
	if got := consentStateOf(t, e, news); got == string(StateWithdrawn) {
		t.Error("a named link withdrew another marketing purpose: it widened to all marketing")
	}
}
