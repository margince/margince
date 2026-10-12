// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Undoing a signature fill, judging an undo with the record's own grant, and
// the receipt's lines for what a machine created and archived.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// seedSignatureFill lands a signature on a contact through the real applier,
// as the mail reader does, and answers the contact and the fill's entry.
func seedSignatureFill(t *testing.T, e *integration.Env) (ids.UUID, ids.UUID) {
	t.Helper()
	contact := seedEnrichContact(t, e, "bob@acme.example",
		"Best,\nBob Contact\nCTO\n+49 30 1234567\nAcme GmbH\nhttps://www.linkedin.com/in/bob-contact")
	var activity ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_link WHERE contact_id = $1`, contact).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	field := func(name, value string) contacts.SignatureField {
		return contacts.SignatureField{Name: name, Value: value, Evidence: value, Confidence: 0.9}
	}
	if _, err := e.Contacts.ApplySignatureFields(machineCtx(e), ids.From[ids.ContactKind](contact), activity,
		[]contacts.SignatureField{
			field("title", "CTO"), field("phone", "+49 30 1234567"), field("company_name", "Acme GmbH"),
			field("linkedin", "https://www.linkedin.com/in/bob-contact"),
		}); err != nil {
		t.Fatalf("the mail reader filling the contact: %v", err)
	}
	var fill ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id FROM audit_log
			 WHERE entity_type = 'contact' AND entity_id = $1 AND evidence->>'source' = 'capture_enrich'`,
			contact).Scan(&fill)
	}); err != nil {
		t.Fatalf("find the fill's entry: %v", err)
	}
	return contact, fill
}

// whatTheFillLeft counts what the fill put on the contact: a title, evidence
// rows, a live phone number and a LinkedIn slot.
func whatTheFillLeft(t *testing.T, e *integration.Env, contact ids.UUID) (title *string, evidence, phones, linkedin int) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		return tx.QueryRow(c, `
			SELECT (SELECT title FROM contact WHERE id = $1),
			       (SELECT count(*) FROM contact_profile_field WHERE contact_id = $1),
			       (SELECT count(*) FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL),
			       (SELECT count(*) FROM contact_social WHERE contact_id = $1 AND platform = 'linkedin')`,
			contact).Scan(&title, &evidence, &phones, &linkedin)
	}); err != nil {
		t.Fatal(err)
	}
	return title, evidence, phones, linkedin
}

// A signature that filled a title, a phone, a company name and a LinkedIn
// profile is undone by clearing all four.
func TestUndoingASignatureFillClearsEveryFieldItFilled(t *testing.T) {
	e := integration.Setup(t)
	contact, fill := seedSignatureFill(t, e)
	if title, evidence, phones, linkedin := whatTheFillLeft(t, e, contact); title == nil || evidence < 4 || phones != 1 || linkedin != 1 {
		t.Fatalf("the fill landed title=%v evidence=%d phones=%d linkedin=%d; the undo below would prove nothing",
			title, evidence, phones, linkedin)
	}

	if answer := advisoryAnswer(e.Admin(), t, e, "contact", contact, fill); !answer.Undoable {
		t.Fatalf("the signature fill reads %+v, want undoable", answer)
	}
	if err := undoEntry(t, e, "contact", contact, fill); err != nil {
		t.Fatalf("undoing the fill: %v", err)
	}
	title, evidence, phones, linkedin := whatTheFillLeft(t, e, contact)
	if title != nil || evidence != 0 || phones != 0 || linkedin != 0 {
		t.Errorf("after the undo: title=%v evidence=%d phones=%d linkedin=%d, want all cleared",
			title, evidence, phones, linkedin)
	}
	if answer := advisoryAnswer(e.Admin(), t, e, "contact", contact, fill); answer.Reason != string(ReasonAlreadyUndone) {
		t.Errorf("the undone fill reads %+v, want %q", answer, ReasonAlreadyUndone)
	}
}

// A title a colleague typed after the fill is theirs; the undo refuses rather
// than clearing it.
func TestAFillAColleagueChangedSinceIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	contact, fill := seedSignatureFill(t, e)
	typed := "Geschäftsführerin"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &typed}); err != nil {
		t.Fatalf("a colleague typing a title: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, fill)); reason != ReasonSuperseded {
		t.Errorf("the undo of a fill a colleague changed since refused %q, want %q", reason, ReasonSuperseded)
	}
	if title, _, _, _ := whatTheFillLeft(t, e, contact); title == nil || *title != typed {
		t.Errorf("title = %v, want the colleague's %q left standing", title, typed)
	}
}

// judgeOnTheReceipt asks the receipt's undo judge about one entry, as one seat.
func judgeOnTheReceipt(seat context.Context, t *testing.T, e *integration.Env, entityType string, auditID ids.UUID) *crmcontracts.MagicUndo {
	t.Helper()
	var answer *crmcontracts.MagicUndo
	if err := database.WithWorkspaceTx(seat, e.Pool, func(tx pgx.Tx) error {
		answers, err := magicUndoJudge{seam: restoreSeamFor(e)}.JudgeUndoPage(seat, tx,
			[]magic.UndoSubject{{AuditID: auditID, EntityType: entityType}})
		answer = answers[auditID]
		return err
	}); err != nil {
		t.Fatalf("judging entry %s: %v", auditID, err)
	}
	if answer == nil {
		t.Fatalf("entry %s was not judged", auditID)
	}
	return answer
}

// The receipt judges an undo with the grant of the entry's own record type: a
// seat that may change contacts and not deals is offered a contact undo, and a
// seat with the opposite grants is refused it.
func TestTheReceiptJudgesAnUndoWithTheRecordTypesOwnGrant(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Petra Machine", nil)
	title := "Buyer"
	if _, err := e.Contacts.UpdateContact(machineCtx(e), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("the machine changing the contact: %v", err)
	}
	change := latestAuditRowID(t, e, "contact", contact, "update")
	seat := func(objects map[string]principal.ObjectGrant) context.Context {
		return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeAll,
		})
	}

	contactsOnly := seat(map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}})
	if answer := judgeOnTheReceipt(contactsOnly, t, e, "contact", change); !answer.Undoable {
		t.Errorf("a seat that may change contacts and not deals was refused %v", answer.Reason)
	}
	dealsOnly := seat(map[string]principal.ObjectGrant{
		"contact": {Read: true}, "deal": {Read: true, Update: true},
	})
	answer := judgeOnTheReceipt(dealsOnly, t, e, "contact", change)
	if answer.Undoable || answer.Reason == nil || *answer.Reason != string(ReasonNotWritableByCaller) {
		t.Errorf("a seat that may change deals and not contacts was answered %+v, want %q", answer, ReasonNotWritableByCaller)
	}
}

// An import's creates and the mail reader's archives each fold into ONE line
// per job, kind of record and day, and the line opens to an undo per record.
func TestTheReceiptFoldsCreatesAndArchivesAndOffersAnUndoPerRecord(t *testing.T) {
	e := integration.Setup(t)
	since := time.Now().Add(-time.Hour)
	for _, name := range []string{"Ida Import", "Jan Import", "Kai Import"} {
		if _, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: name, Source: "import"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Noise One GmbH", "Noise Two GmbH"} {
		company := e.SeedCompany(t, name, nil)
		if _, err := NewProvider(e.Pool).Archive(machineCtx(e), datasource.EntityRef{Type: "company", ID: company}); err != nil {
			t.Fatal(err)
		}
	}
	svc := newMagicService(e.Pool, approvalsServiceWithEffects(e.Pool), time.Now)
	svc.WithUndoJudge(magicUndoJudge{seam: restoreSeamFor(e)})

	receipt, err := svc.Read(e.Admin(), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}
	lines := map[string]crmcontracts.MagicLine{}
	for _, line := range receipt.Done {
		lines[line.Summary.Key] = line
	}
	created, drawn := lines["magic.action.create_contact"]
	archived, archivedDrawn := lines["magic.action.archive_company"]
	if !drawn || !archivedDrawn {
		t.Fatalf("the receipt drew no create line (%v) or no archive line (%v): %+v", drawn, archivedDrawn, receipt.Done)
	}
	if created.Count == nil || *created.Count != 3 {
		t.Errorf("the import's creates drew %+v, want one line counting 3 contacts", created)
	}
	if archived.Count == nil || *archived.Count != 2 {
		t.Errorf("the archives drew %+v, want one line counting 2 companies", archived)
	}

	records, err := svc.LineRecords(e.Admin(), ids.UUID(created.Id), since, nil, 50)
	if err != nil {
		t.Fatalf("opening the create line: %v", err)
	}
	if len(records.Data) != 3 {
		t.Fatalf("the create line opened to %d records, want 3", len(records.Data))
	}
	for _, record := range records.Data {
		if !record.Undo.Undoable || record.Undo.AuditId == nil || record.Undo.Version == nil {
			t.Errorf("record %v offers %+v, want an undo it can send", record.Entity.Label, record.Undo)
		}
		if len(record.Changes) != 0 {
			t.Errorf("record %v lists %d field changes, want none: a create is the whole record", record.Entity.Label, len(record.Changes))
		}
	}
}

// fillEntryOf is the newest fill entry a pass wrote on one contact.
func fillEntryOf(t *testing.T, e *integration.Env, contact ids.UUID, source string) ids.UUID {
	t.Helper()
	var fill ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id FROM audit_log
			 WHERE entity_type = 'contact' AND entity_id = $1 AND evidence->>'source' = $2
			 ORDER BY occurred_at DESC, id DESC LIMIT 1`, contact, source).Scan(&fill)
	}); err != nil {
		t.Fatalf("find the %s fill's entry: %v", source, err)
	}
	return fill
}

// A later signature that only repeats a number already on the record did not
// add it; undoing that signature must not take the number away.
func TestUndoingASignatureThatOnlyConfirmedANumberLeavesTheNumber(t *testing.T) {
	e := integration.Setup(t)
	contact, _ := seedSignatureFill(t, e)
	later := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		c := context.Background()
		if _, err := tx.Exec(c, `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by, occurred_at)
			VALUES ($1, 'email', 'again', 'Best,\nBob Contact\n+49 30 1234567', 'inbound', 'gmail', $2, 'gmail:seed', 'connector:gmail', now() + interval '1 day')`,
			later, later.String()); err != nil {
			return err
		}
		if _, err := tx.Exec(c, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, later, contact); err != nil {
			return err
		}
		_, err := tx.Exec(c, `INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, 'bob@acme.example', 'from')`, later, contact)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ApplySignatureFields(machineCtx(e), ids.From[ids.ContactKind](contact), later,
		[]contacts.SignatureField{{Name: "phone", Value: "+49 30 1234567", Evidence: "+49 30 1234567", Confidence: 0.9}}); err != nil {
		t.Fatalf("the later signature repeating the number: %v", err)
	}
	confirm := fillEntryOf(t, e, contact, "capture_enrich")

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, confirm)); reason != ReasonNotRestorableByThisPath {
		t.Errorf("undoing a signature that only confirmed a number refused %q, want %q", reason, ReasonNotRestorableByThisPath)
	}
	if _, _, phones, _ := whatTheFillLeft(t, e, contact); phones != 1 {
		t.Errorf("%d live numbers after the refused undo, want the one the first signature added", phones)
	}
}

// A site read fills an empty title straight onto the column, with no title
// evidence row of its own. Undo clears that title too.
func TestUndoingASiteReadClearsTheTitleItFilled(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Sara Site", nil)
	company := e.SeedCompany(t, "Acme Site GmbH", nil)
	seedEmploymentEdge(t, e, contact, company)
	matched, err := e.Contacts.ApplySiteContactFields(machineCtx(e), ids.From[ids.CompanyKind](company), contacts.SiteContactFields{
		Name: "Sara Site", Role: "Head of Operations", EvidenceSnippet: "Sara Site, Head of Operations",
		SourceURL: "https://acme.test/team",
	})
	if err != nil || !matched {
		t.Fatalf("the site read filling the contact: matched=%v err=%v", matched, err)
	}
	fill := fillEntryOf(t, e, contact, "site_read")

	if err := undoEntry(t, e, "contact", contact, fill); err != nil {
		t.Fatalf("undoing the site read: %v", err)
	}
	if title, evidence, _, _ := whatTheFillLeft(t, e, contact); title != nil || evidence != 0 {
		t.Errorf("after the undo: title=%v evidence=%d, want both cleared", title, evidence)
	}
}

// A colleague who worked on the contact a promotion created keeps that work:
// the demotion would archive the contact.
func TestAPromotionWhoseContactAColleagueChangedIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	name, email := "Lena Lead", "lena@prospect.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	contact, _, err := e.Contacts.PromoteLead(machineCtx(e), ids.From[ids.LeadKind](leadID), contacts.PromoteLeadInput{Trigger: "inbound_reply"})
	if err != nil {
		t.Fatal(err)
	}
	title := "Fleet Manager"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(contact.Id)), contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("a colleague editing the promoted contact: %v", err)
	}
	promoteID := latestAuditRowID(t, e, entityTypeLead, leadID, actionPromote)

	if reason := refusedFor(t, undoEntry(t, e, entityTypeLead, leadID, promoteID)); reason != ReasonSuperseded {
		t.Errorf("the undo refused %q, want %q", reason, ReasonSuperseded)
	}
	if isArchived(t, e, "contact", ids.UUID(contact.Id)) {
		t.Error("the refused undo archived the colleague's contact anyway")
	}
}

// A colleague who linked an imported contact to their employer wrote the link,
// not the contact; archiving the contact would retire that link with it.
func TestACreateWhoseLinkAColleagueAddedIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", contact, actionCreate)
	seedEmploymentEdge(t, e, contact, e.SeedCompany(t, "Employer GmbH", nil))

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, createID)); reason != ReasonSuperseded {
		t.Errorf("the undo of a create whose link a colleague added refused %q, want %q", reason, ReasonSuperseded)
	}
}

// A number a colleague reclassified after the signature added it is theirs now;
// the undo refuses rather than archiving it.
func TestASignatureNumberAColleagueChangedIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	contact, fill := seedSignatureFill(t, e)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{
		Phones: []contacts.ContactPhoneInput{{Phone: "+49 30 1234567", PhoneType: "mobile", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("a colleague reclassifying the number: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, fill)); reason != ReasonSuperseded {
		t.Errorf("the undo refused %q, want %q", reason, ReasonSuperseded)
	}
	if _, _, phones, _ := whatTheFillLeft(t, e, contact); phones != 1 {
		t.Errorf("%d live numbers after the refused undo, want the colleague's one", phones)
	}
}

// A signature that states the title a colleague already typed confirms it and
// fills nothing, so undoing the signature leaves the title.
func TestUndoingASignatureLeavesATitleItOnlyConfirmed(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@acme.example", "Best,\nBob Contact\nCTO\nAcme GmbH")
	typed := "CTO"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &typed}); err != nil {
		t.Fatal(err)
	}
	var activity ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_link WHERE contact_id = $1`, contact).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ApplySignatureFields(machineCtx(e), ids.From[ids.ContactKind](contact), activity,
		[]contacts.SignatureField{
			{Name: "title", Value: "CTO", Evidence: "CTO", Confidence: 0.9},
			{Name: "company_name", Value: "Acme GmbH", Evidence: "Acme GmbH", Confidence: 0.9},
		}); err != nil {
		t.Fatalf("the signature: %v", err)
	}
	fill := fillEntryOf(t, e, contact, "capture_enrich")
	if err := undoEntry(t, e, "contact", contact, fill); err != nil {
		t.Fatalf("undoing the signature: %v", err)
	}
	title, evidence, _, _ := whatTheFillLeft(t, e, contact)
	if title == nil || *title != typed {
		t.Errorf("title = %v, want the colleague's %q kept", title, typed)
	}
	if evidence != 1 {
		t.Errorf("%d evidence rows after the undo, want only the title's confirmation: the company name it filled is cleared", evidence)
	}
}

// A LinkedIn handle a colleague saved after the fill is theirs, even when it
// reads the same as the one the fill claimed.
func TestASignatureLinkedinAColleagueSavedSinceIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	contact, fill := seedSignatureFill(t, e)
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{
		Social: map[string]any{"linkedin": "https://www.linkedin.com/in/bob-contact"},
	}); err != nil {
		t.Fatalf("a colleague saving the handle: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, fill)); reason != ReasonSuperseded {
		t.Errorf("the undo refused %q, want %q", reason, ReasonSuperseded)
	}
	if _, _, _, linkedin := whatTheFillLeft(t, e, contact); linkedin != 1 {
		t.Errorf("%d LinkedIn handles after the refused undo, want the colleague's one", linkedin)
	}
}

// A title a colleague saved again after a site read filled it is theirs, even
// though it reads the same and the read left no title evidence row.
func TestASiteReadTitleAColleagueSavedAgainIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Sara Site", nil)
	company := e.SeedCompany(t, "Acme Site GmbH", nil)
	seedEmploymentEdge(t, e, contact, company)
	if matched, err := e.Contacts.ApplySiteContactFields(machineCtx(e), ids.From[ids.CompanyKind](company), contacts.SiteContactFields{
		Name: "Sara Site", Role: "Head of Operations", EvidenceSnippet: "Sara Site, Head of Operations",
		SourceURL: "https://acme.test/team",
	}); err != nil || !matched {
		t.Fatalf("the site read: matched=%v err=%v", matched, err)
	}
	fill := fillEntryOf(t, e, contact, "site_read")
	same := "Head of Operations"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &same}); err != nil {
		t.Fatal(err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, fill)); reason != ReasonSuperseded {
		t.Errorf("the undo refused %q, want %q", reason, ReasonSuperseded)
	}
}

// A signature written before confirmations were named says it filled a title
// even where it only repeated one. Its undo leaves a title the trail shows was
// already set, and clears what it did fill.
func TestUndoingALegacySignatureLeavesATitleTheRecordAlreadyHad(t *testing.T) {
	e := integration.Setup(t)
	contact := seedEnrichContact(t, e, "bob@acme.example", "Best,\nBob Contact\nCTO\nAcme GmbH")
	typed := "CTO"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &typed}); err != nil {
		t.Fatal(err)
	}
	var activity ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT activity_id FROM activity_link WHERE contact_id = $1`, contact).Scan(&activity)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.ApplySignatureFields(machineCtx(e), ids.From[ids.ContactKind](contact), activity,
		[]contacts.SignatureField{
			{Name: "title", Value: "CTO", Evidence: "CTO", Confidence: 0.9},
			{Name: "company_name", Value: "Acme GmbH", Evidence: "Acme GmbH", Confidence: 0.9},
		}); err != nil {
		t.Fatal(err)
	}
	// The same statement as the shape it was audited in before: title named as
	// filled, and no confirmed list.
	legacy := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after, evidence)
		VALUES ($1, 'agent', 'agent:enrich', 'update', 'contact', $2,
		        '{"title":null,"company_name":null}', '{"title":"filled","company_name":"filled"}',
		        jsonb_build_object('source', 'capture_enrich', 'source_ref', 'activity:' || $3::text))`,
		legacy, contact, activity)

	if err := undoEntry(t, e, "contact", contact, legacy); err != nil {
		t.Fatalf("undoing the legacy signature: %v", err)
	}
	title, evidence, _, _ := whatTheFillLeft(t, e, contact)
	if title == nil || *title != typed {
		t.Errorf("title = %v, want the colleague's %q kept", title, typed)
	}
	if evidence != 1 {
		t.Errorf("%d evidence rows after the undo, want the title's alone", evidence)
	}
}
