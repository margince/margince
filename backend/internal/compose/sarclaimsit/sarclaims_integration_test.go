// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package sarclaimsit_test holds what the access export hands a subject about
// the claims read out of their messages: the reading beside the words, and
// never the colleague who wrote it.
package sarclaimsit_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func seedContact(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Alice Kessler", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "alice.kessler@buyer.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the subject: %v", err)
	}
	return ids.UUID(created.Id)
}

func exportedClaims(t *testing.T, e *integration.Env, contact ids.UUID) []map[string]any {
	t.Helper()
	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("AssembleSAR → %v", err)
	}
	return pkg.ConversationClaims
}

// claimOnOwnMessage files a claim about the contact, read from a message linked
// to them, through the real claim writer.
func claimOnOwnMessage(t *testing.T, e *integration.Env, contact ids.UUID, quote string) ids.UUID {
	t.Helper()
	message := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Re: order', $2, now(), 'manual', 'human:seed')`, message, quote)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, message, contact)
	if _, err := e.Contacts.RecordConversationClaim(e.Admin(), contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](contact), Kind: "commitment_theirs",
		Body: "Sends the signed order by Friday", ActivityID: message, Quote: quote, Source: "manual",
	}); err != nil {
		t.Fatalf("filing the claim: %v", err)
	}
	return message
}

func TestTheAccessExportReturnsAClaimsReadingBesideTheWordsAndNotItsAuthor(t *testing.T) {
	e := integration.Setup(t)
	contact := seedContact(t, e)
	const quote = "I will send the signed order by Friday."
	claimOnOwnMessage(t, e, contact, quote)

	got := exportedClaims(t, e, contact)
	if len(got) != 1 {
		t.Fatalf("the export must carry the one claim about its subject: %+v", got)
	}
	claim := got[0]
	if claim["reading"] != "Sends the signed order by Friday" || claim["quote"] != quote || claim["derived"] != true {
		t.Errorf("the export must hand back the reading, marked derived, beside the quoted words: %+v", claim)
	}
	for _, author := range []string{"captured_by", "corrected_by_user_id", "taken_by_user_id"} {
		if _, ok := claim[author]; ok {
			t.Errorf("the export names the colleague who wrote the reading through %s: %+v", author, claim)
		}
	}
}

func TestTheAccessExportWithholdsAClaimsReadingOfALimitedMessage(t *testing.T) {
	e := integration.Setup(t)
	contact := seedContact(t, e)
	message := claimOnOwnMessage(t, e, contact, "I will send the signed order by Friday.")
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, message)

	got := exportedClaims(t, e, contact)
	if len(got) != 1 {
		t.Fatalf("the export must still disclose that a claim is held: %+v", got)
	}
	if got[0]["reading"] != nil || got[0]["quote"] != nil || got[0]["content_disclosed"] != false {
		t.Errorf("the export hands out a reading of a message whose body it withholds: %+v", got[0])
	}
}
