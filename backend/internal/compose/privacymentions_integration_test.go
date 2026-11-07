// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func logMentionNote(t *testing.T, e *integration.Env, subject, body string, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	note, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: &subject, Body: &body, Source: "manual", Links: links})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(note.Id)
}

func TestContactErasureScrubsUnlinkedNoteMentionsAndKeepsHoldsAndLinkedNotes(t *testing.T) {
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Anna Berg", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "anna+crm@example.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Held company"})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE company SET legal_hold=true WHERE id=$1`, company.Id)
	other := e.SeedContact(t, "Other Contact", nil)
	body := "Write to (ANNA+CRM@example.test), then compare joanna+crm@example.test."
	scrubbed := logMentionNote(t, e, "Review with Anna   Berg", body)
	held := logMentionNote(t, e, "Anna Berg", body, activities.ActivityLinkInput{EntityType: "company", EntityID: ids.UUID(company.Id)})
	linked := logMentionNote(t, e, "Anna Berg", body, activities.ActivityLinkInput{EntityType: "contact", EntityID: other})
	unrelated := logMentionNote(t, e, "Joanna Bergen", "Use joanna+crm@example.test, or anna+crm@example.test.invalid.")
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB()))
	if err := eraser.EraseContact(e.Admin(), ids.UUID(contact.Id), "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT subject||' / '||body FROM activity WHERE id=$1`, scrubbed); got != "Review with Erased / Write to (Erased), then compare joanna+crm@example.test." {
		t.Fatalf("scrubbed note: %s", got)
	}
	for _, id := range []ids.UUID{held, linked} {
		if got := e.WsScalar(t, `SELECT subject||' / '||body FROM activity WHERE id=$1`, id); got != "Anna Berg / "+body {
			t.Fatalf("protected note changed: %s", got)
		}
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id=$1`, unrelated); got != "Use joanna+crm@example.test, or anna+crm@example.test.invalid." {
		t.Fatalf("longer addresses changed: %s", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1 AND concat(before::text,after::text,evidence::text) ILIKE '%anna%'`, scrubbed); n != 0 {
		t.Fatalf("kept %d personal audit images", n)
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1`, scrubbed)
	if err := eraser.EraseContact(e.Admin(), ids.UUID(contact.Id), "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity' AND entity_id=$1`, scrubbed); after != before {
		t.Fatalf("replay changed note history: %d -> %d", before, after)
	}
}

func TestContactErasureMatchesAOneWordNameOnlyAsAWholeWord(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Anna", nil)
	note := logMentionNote(t, e, "Follow Anna", "Anna spoke to Joanna.")
	if err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), contact, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT subject||' / '||body FROM activity WHERE id=$1`, note); got != "Follow Erased / Erased spoke to Joanna." {
		t.Fatalf("whole-name match: %s", got)
	}
}
