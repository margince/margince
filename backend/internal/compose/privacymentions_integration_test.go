// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin' ON CONFLICT DO NOTHING`, e.AdminUser)
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
	e.WsExec(t, `UPDATE activity SET raw='{"original":"Anna Berg"}' WHERE id=$1`, scrubbed)
	oldVersion, err := strconv.ParseInt(e.WsScalar(t, `SELECT version FROM activity WHERE id=$1`, scrubbed), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{scrubbed, held} {
		e.WsExec(t, `INSERT INTO embedding(entity_type,entity_id,embedding,chunk_hash,model) VALUES('activity',$1,'[0,1,0]'::vector,'note-copy','test')`, id)
		e.WsExec(t, `INSERT INTO field_provenance(object_type,object_id,field_name,source,captured_by,evidence_ref) VALUES('activity',$1,'body','manual','system:test','Anna Berg')`, id)
	}
	eraser := eraserWithGraph(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB()))
	if err := eraser.EraseContact(e.Admin(), ids.UUID(contact.Id), "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT subject||' / '||body FROM activity WHERE id=$1`, scrubbed); got != "Review with Erased / Write to (Erased), then compare joanna+crm@example.test." {
		t.Fatalf("scrubbed note: %s", got)
	}

	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](scrubbed), activities.UpdateActivityInput{Body: &body, IfVersion: &oldVersion}); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale editor restored erased text: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND (raw IS NOT NULL OR source_author_name IS NOT NULL)`, scrubbed); n != 0 {
		t.Fatal("kept original note text")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM embedding WHERE entity_type='activity' AND entity_id=$1`, scrubbed); n != 0 {
		t.Fatal("kept erased note vector")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM field_provenance WHERE object_type='activity' AND object_id=$1`, scrubbed); n != 0 {
		t.Fatal("kept erased note provenance")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM embedding WHERE entity_type='activity' AND entity_id=$1`, held); n != 1 {
		t.Fatal("removed held note vector")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_type='activity' AND target_id=$1 AND recipient_user_id=$2`, held, e.AdminUser); n != 1 {
		t.Fatal("held note lacks manual follow-up")
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

func TestContactErasureLeavesAmbiguousOneWordNameMentionsForReview(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin' ON CONFLICT DO NOTHING`, e.AdminUser)
	contact := e.SeedContact(t, "Anna", nil)
	note := logMentionNote(t, e, "Follow Anna", "Anna spoke to Joanna.")
	if err := eraserWithGraph(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), contact, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_type='activity' AND target_id=$1 AND recipient_user_id=$2`, note, e.AdminUser); n != 1 {
		t.Fatalf("missing ambiguous-name manual review: %d", n)
	}
	if got := e.WsScalar(t, `SELECT subject||' / '||body FROM activity WHERE id=$1`, note); got != "Follow Anna / Anna spoke to Joanna." {
		t.Fatalf("whole-name match: %s", got)
	}
}

func TestContactErasureFlagsRetainedDescriptionsSignaturesAndUploadedText(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='admin' ON CONFLICT DO NOTHING`, e.AdminUser)
	subject := e.SeedContact(t, "Anna Berg", nil)
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Review company"})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE company SET description='Anna Berg is mentioned here' WHERE id=$1`, company.Id)
	signature, corpus, document, chunk := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO email_signature(id,owner_id,body,html) VALUES(@id,@owner,'Keep signature','<p>Anna Berg</p>')`, pgx.StrictNamedArgs{"id": signature, "owner": e.Rep1})
	e.WsExec(t, `INSERT INTO knowledge_corpus(id,name,topic_statement,captured_by) VALUES($1,'Review files','Uploaded reference','system:test')`, corpus)
	e.WsExec(t, `INSERT INTO knowledge_document(id,corpus_id,filename,content_type,byte_size,storage_key,checksum,captured_by) VALUES($1,$2,'reference.pdf','application/pdf',123,'retained-reference','test-checksum','system:test')`, document, corpus)
	e.WsExec(t, `INSERT INTO knowledge_chunk(id,corpus_id,document_id,chunk_ix,text,chunk_hash) VALUES($1,$2,$3,0,'Anna Berg is mentioned in the uploaded file','test-chunk')`, chunk, corpus, document)
	if err := eraserWithGraph(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT description FROM company WHERE id=$1`, company.Id); got != "Anna Berg is mentioned here" {
		t.Fatal(got)
	}
	if got := e.WsScalar(t, `SELECT html FROM email_signature WHERE id=$1`, signature); got != "<p>Anna Berg</p>" {
		t.Fatal(got)
	}
	if got := e.WsScalar(t, `SELECT text FROM knowledge_chunk WHERE id=$1`, chunk); got != "Anna Berg is mentioned in the uploaded file" {
		t.Fatal(got)
	}
	if got := e.WsScalar(t, `SELECT storage_key FROM knowledge_document WHERE id=$1`, document); got != "retained-reference" {
		t.Fatal(got)
	}
	for _, target := range []ids.UUID{ids.UUID(company.Id), document} {
		if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_id=$1 AND recipient_user_id=$2`, target, e.AdminUser); n != 1 {
			t.Fatalf("controller notice count=%d for %s", n, target)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_id=$1 AND recipient_user_id=$2`, signature, e.Rep1); n != 1 {
		t.Fatalf("signature owner notice count=%d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND target_id=$1 AND recipient_user_id<>$2`, signature, e.Rep1); n != 0 {
		t.Fatal("signature sent to another reader")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM notice WHERE kind='privacy_review_required' AND concat(subject,body) ILIKE '%Anna%'`); n != 0 {
		t.Fatal("copied erased identity into notice")
	}
}

func TestContactErasureMatchesPunctuationAndUnicodeNamesAsWholeNames(t *testing.T) {
	for _, name := range []string{"Anna Berg.", "Élodie Müller", "Jean-Luc O’Neil"} {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			subject := e.SeedContact(t, name, nil)
			note := logMentionNote(t, e, "Review ("+name+")", "Keep context")
			longer := logMentionNote(t, e, "x"+name+"x", "Keep context")
			if err := eraserWithGraph(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
				t.Fatal(err)
			}
			if got := e.WsScalar(t, `SELECT subject FROM activity WHERE id=$1`, note); got != "Review (Erased)" {
				t.Fatalf("whole name remained: %s", got)
			}
			if got := e.WsScalar(t, `SELECT subject FROM activity WHERE id=$1`, longer); got != "x"+name+"x" {
				t.Fatalf("longer name changed: %s", got)
			}
		})
	}
}
