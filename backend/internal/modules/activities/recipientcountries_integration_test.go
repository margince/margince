// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// A saved draft gives back a cleared country as cleared and keeps no entry for
// a recipient the author stated nothing about, or one no longer on the draft.
func TestADraftKeepsAClearedCountryApartFromNone(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	store := draftStore(e)

	content := typedDraft()
	content.Cc = []string{"Boss@Example.test"}
	content.RecipientCountries = []commsauthz.CountryOverride{
		{Address: "buyer@example.test", Country: "at"},
		{Address: "boss@example.test"},
		{Address: "gone@example.test", Country: "de"},
	}
	if _, err := store.SaveMailDraft(ctx, anchor, content, nil); err != nil {
		t.Fatalf("saving the draft: %v", err)
	}
	got, err := store.GetMailDraft(ctx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	want := []commsauthz.CountryOverride{{Address: "buyer@example.test", Country: "at"}, {Address: "boss@example.test"}}
	if len(got.Content.RecipientCountries) != 2 || got.Content.RecipientCountries[0] != want[0] || got.Content.RecipientCountries[1] != want[1] {
		t.Fatalf("read back %+v, want %+v", got.Content.RecipientCountries, want)
	}
	wire, err := json.Marshal(mailDraftResponse(got).RecipientCountries)
	if err != nil {
		t.Fatal(err)
	}
	if string(wire) != `[{"address":"buyer@example.test","country":"at"},{"address":"boss@example.test","country":null}]` {
		t.Errorf("the wire draft carries %s, want the cleared country as null", wire)
	}

	none := typedDraft()
	if _, err := store.SaveMailDraft(ctx, anchor, none, &got.Version); err != nil {
		t.Fatal(err)
	}
	if got, err = store.GetMailDraft(ctx, anchor); err != nil || got.Content.RecipientCountries != nil ||
		mailDraftResponse(got).RecipientCountries != nil {
		t.Errorf("a draft with no statement reads %+v (%v), want none", got.Content.RecipientCountries, err)
	}
}

// An agent's draft keeps no stated country: only the author may move a
// recipient's jurisdiction.
func TestAnAgentsDraftKeepsNoStatedCountry(t *testing.T) {
	e := setupSend(t)
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: e.seedContact(t, "Buyer")}
	content := agentDraft("First go")
	content.RecipientCountries = []commsauthz.CountryOverride{{Address: "buyer@example.test"}}
	if _, err := draftStore(e).SaveMailDraft(e.asAgentFor(t, principal.RowScopeAll), anchor, content, nil); err != nil {
		t.Fatalf("an agent saving a draft: %v", err)
	}
	got, err := draftStore(e).GetMailDraft(e.as(principal.RowScopeAll), anchor)
	if err != nil || got.Content.RecipientCountries != nil {
		t.Errorf("the agent's draft reads countries %+v (%v), want none", got.Content.RecipientCountries, err)
	}
}

// The send's audit row names each recipient whose country the sender stated,
// and nobody else.
func TestTheSendsAuditNamesTheStatedCountries(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	anchor := e.seedAnchor(t, "", "")
	in := sendInput("transactional")
	in.Evidence.RecipientCountries = []commsauthz.CountryOverride{
		{Address: "buyer@example.test", Country: "at"},
		{Address: "boss@example.test"},
		{Address: "stranger@example.test", Country: "de"},
	}
	sent, err := draftStore(e).SendEmail(ctx, FromActivity(anchor), in, stubConsentGate{}, &recordingStager{})
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	var named map[string]string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT evidence->'recipient_countries' FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'create'`, ids.UUID(sent.Id)).Scan(&named); err != nil {
		t.Fatalf("reading the send's audit row: %v", err)
	}
	if len(named) != 2 || named["buyer@example.test"] != "at" || named["boss@example.test"] != "cleared" {
		t.Errorf("the audit row names %v, want buyer at, boss cleared and no stranger", named)
	}
}
