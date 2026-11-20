// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

func (e *resolveEnv) previewMarks(t *testing.T, req commsauthz.Request) bool {
	t.Helper()
	set, err := e.gate.Preview(e.ctx, req)
	if err != nil || len(set.Decisions) != 1 {
		t.Fatalf("preview = %+v (%v)", set, err)
	}
	return set.Decisions[0].LawfulBasisMissing
}

func (e *resolveEnv) confirm(t *testing.T, stated *commsauthz.CountryOverride) int {
	t.Helper()
	confirmed, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx, []ConfirmedSubject{
		{Kind: entityContact, ID: e.contact.UUID, Stated: stated},
	})
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	return len(confirmed)
}

func (e *resolveEnv) stagedReason(t *testing.T, req commsauthz.Request) string {
	t.Helper()
	d, err := e.stageStated(e.ctx, t, e.plantDelivery(t), req)
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	if d.Verdict == commsauthz.VerdictAllow {
		return ""
	}
	return d.ReasonCode
}

// A record outside the warning's law, and a country stated inside it. The
// preview warns, the confirmation is taken under the stated country, and
// staging spends it.
func TestAConfirmationFollowsAStatedCountryIntoTheWarning(t *testing.T) {
	e, p := statedEnv(t)
	placeContact(t, e.owner, e.contact, p.installation)
	stated := commsauthz.CountryOverride{Address: e.address, Country: string(p.recipient)}
	req := e.replyStating(stated)
	if !e.previewMarks(t, req) {
		t.Fatal("the preview under the stated country did not warn")
	}
	if got := e.confirm(t, nil); got != 0 {
		t.Errorf("a confirmation under the record's country recorded %d, want none: that law owes no warning", got)
	}
	if reason := e.stagedReason(t, req); reason != commsauthz.ReasonLawfulBasisUnconfirmed {
		t.Fatalf("staging before a confirmation = %q, want unconfirmed", reason)
	}
	if got := e.confirm(t, &stated); got != 1 {
		t.Fatalf("a confirmation under the stated country recorded %d, want one", got)
	}
	if reason := e.stagedReason(t, req); reason != "" {
		t.Errorf("staging after the confirmation = %q, want allowed", reason)
	}
}

// A record inside the warning's law, and the other way round. A cleared
// country owes no warning. A confirmation given under one country is not
// spent by a send judged under another.
func TestAConfirmationIsSpentOnlyUnderItsOwnCountry(t *testing.T) {
	e, _ := statedEnv(t)
	cleared := e.replyStating(commsauthz.CountryOverride{Address: e.address})
	if e.previewMarks(t, cleared) {
		t.Error("the preview warned for a cleared country")
	}
	if reason := e.stagedReason(t, cleared); reason != "" {
		t.Errorf("staging with the country cleared = %q, want allowed without a confirmation", reason)
	}

	other := testMessagingRules(t, messagingrules.Rules{
		Version: 1, NoLegitimateInterests: true,
		Disclosures: []messaging.Disclosure{{Kind: messaging.ControllerIdentity}},
	})
	statedOther := commsauthz.CountryOverride{Address: e.address, Country: string(other)}
	elsewhere := e.replyStating(statedOther)
	if got := e.confirm(t, nil); got != 1 {
		t.Fatalf("a confirmation under the record's country recorded %d, want one", got)
	}
	if reason := e.stagedReason(t, elsewhere); reason != commsauthz.ReasonLawfulBasisUnconfirmed {
		t.Errorf("a send under another country spent the record's confirmation: %q", reason)
	}
	if got := e.confirm(t, &statedOther); got != 1 {
		t.Fatalf("a confirmation under the stated country recorded %d, want one", got)
	}
	if reason := e.stagedReason(t, elsewhere); reason != "" {
		t.Errorf("staging after its own confirmation = %q, want allowed", reason)
	}
	if reason := e.stagedReason(t, e.replyStating()); reason != "" {
		t.Errorf("the record's confirmation was not left for a send under the record's country: %q", reason)
	}
}
