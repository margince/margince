// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"slices"
	"testing"
)

// grantedPurposesOf lists the keys of every purpose the subject has granted.
func grantedPurposesOf(t *testing.T, e *channelConsentEnv) []string {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT cp.key FROM contact_consent cc
		  JOIN consent_purpose cp ON cp.id = cc.purpose_id
		 WHERE cc.contact_id = $1 AND cc.state = 'granted'
		 ORDER BY cp.key`, e.contact)
	if err != nil {
		t.Fatalf("reading the granted purposes: %v", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("reading a granted purpose: %v", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the granted purposes: %v", err)
	}
	return keys
}

// purposeKeyOf names the key of the env's double-opt-in purpose.
func purposeKeyOf(t *testing.T, e *channelConsentEnv) string {
	t.Helper()
	var key string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT key FROM consent_purpose WHERE id = $1`, e.doiNews).Scan(&key); err != nil {
		t.Fatalf("reading the purpose key: %v", err)
	}
	return key
}

// Consent to advertising is given per purpose. The record page's subscribe
// answer grants the advertising purpose alone, while a second live purpose
// stands beside it.
func TestTheSubscribeAnswerGrantsTheAdvertisingPurposeAlone(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	record := issueLink(t, e)

	if _, err := e.store.SubmitConfirmation(e.ctx, record.Token, ConfirmSubmission{
		MarketingChoice: string(StateGranted), MarketingWording: "News from time to time.",
	}); err != nil {
		t.Fatalf("subscribing on the record page: %v", err)
	}
	if got := grantedPurposesOf(t, e); !slices.Equal(got, []string{PurposeMarketingEmail}) {
		t.Errorf("the subscribe answer granted %v, want only %q", got, PurposeMarketingEmail)
	}
}

// A subscription link grants the one purpose it was minted for, and not the
// advertising purpose beside it.
func TestASubscriptionLinkGrantsItsOwnPurposeAlone(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)

	subscription, err := e.store.IssueConsentLink(e.ctx, e.contact, e.doiNews, "", "")
	if err != nil {
		t.Fatalf("minting a subscription link: %v", err)
	}
	if _, err := e.store.SubmitConfirmation(e.ctx, subscription.Token, ConfirmSubmission{
		MarketingChoice: string(StateGranted), MarketingWording: "News from time to time.",
	}); err != nil {
		t.Fatalf("confirming the subscription: %v", err)
	}
	if got, want := grantedPurposesOf(t, e), purposeKeyOf(t, e); !slices.Equal(got, []string{want}) {
		t.Errorf("the subscription link granted %v, want only %q", got, want)
	}
}
