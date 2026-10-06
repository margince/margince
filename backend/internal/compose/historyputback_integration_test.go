// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Put back is the safety net under every change an assistant may self-approve,
// so each answer it gives has to be true: an entry it offers works, one it
// refuses was overtaken by somebody, and a restore that brought back less than
// the archive took down says so. Every case here seeds through the owning
// writer and asks the seam the history screen asks.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func opsSeat(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, integration.OpsPerms)
}

func TestAContactsSocialLinkAColleagueSetSinceIsNotPutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := e.SeedContact(t, "Social Contact", nil)
	contactID := ids.From[ids.ContactKind](id)
	for _, v := range []string{"a", "b"} {
		if _, err := e.Contacts.UpdateContact(admin, contactID, contacts.UpdateContactInput{Social: map[string]any{"linkedin": v}}); err != nil {
			t.Fatalf("set social %s: %v", v, err)
		}
	}
	entry := latestAuditRowID(t, e, "contact", id, "update")
	if _, err := e.Contacts.UpdateContact(opsSeat(e), contactID, contacts.UpdateContactInput{Social: map[string]any{"linkedin": "c"}}); err != nil {
		t.Fatalf("a colleague sets social: %v", err)
	}

	if got := advisoryAnswer(admin, t, e, "contact", id, entry); got.Undoable {
		t.Error("the listing offers Put back over a social link a colleague has changed since")
	}
	_, err := restoreSeamFor(e).Restore(admin, "contact", id, entry, currentVersion(t, e, "contact", id))
	var refusal RefusedRestore
	if !errors.As(err, &refusal) || refusal.Reason != ReasonSuperseded {
		t.Fatalf("putting back the earlier social link answered %v, want superseded", err)
	}
}

func TestACompanysDomainsAColleagueSetSinceAreNotPutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := e.SeedCompany(t, "Domain Co", nil)
	companyID := ids.From[ids.CompanyKind](id)
	set := func(ctx context.Context, domain string) {
		t.Helper()
		domains := []contacts.CompanyDomainInput{{Domain: domain}}
		if _, err := e.Contacts.UpdateCompany(ctx, companyID, contacts.UpdateCompanyInput{Domains: &domains}); err != nil {
			t.Fatalf("set domain %s: %v", domain, err)
		}
	}
	set(admin, "a-"+id.String()[:8]+".test")
	set(admin, "b-"+id.String()[:8]+".test")
	entry := latestAuditRowID(t, e, "company", id, "update")
	set(opsSeat(e), "c-"+id.String()[:8]+".test")

	if got := advisoryAnswer(admin, t, e, "company", id, entry); got.Undoable {
		t.Error("the listing offers Put back over domains a colleague has changed since")
	}
}

func TestAFreshActivityDateChangeCanBePutBackWhateverZoneTheWriterRunsIn(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	zone := time.FixedZone("ICT", 7*3600)
	subject := "Zoned task"
	due := time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)
	task, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{Kind: "task", Subject: &subject, DueAt: &due, Source: "manual"})
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	moved := time.Date(2026, 12, 9, 17, 0, 0, 0, zone)
	id := ids.UUID(task.Id)
	if _, err := e.Activities.UpdateActivity(admin, ids.From[ids.ActivityKind](id), activities.UpdateActivityInput{DueAt: &moved}); err != nil {
		t.Fatalf("move the due date: %v", err)
	}
	entry := latestAuditRowID(t, e, "activity", id, "update")
	if got := advisoryAnswer(admin, t, e, "activity", id, entry); !got.Undoable {
		t.Errorf("a due date nobody else touched reads %q (%s), want undoable", got.Reason, got.Detail)
	}
}

func TestAnAmountStepCanBePutBackOnceALaterMoneyChangeWasPutBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	pipeline, open, _ := integration.DealFixture(t, e)
	amount, currency := int64(100_000), "USD"
	deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{
		Name: "Money", PipelineID: pipeline, StageID: open, Source: "manual", AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("seed deal: %v", err)
	}
	id := ids.UUID(deal.Id)
	dealID := ids.From[ids.DealKind](id)
	two := int64(200_000)
	if _, err := e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{AmountMinor: &two}); err != nil {
		t.Fatal(err)
	}
	first := latestAuditRowID(t, e, "deal", id, "update")
	fifty, yen := int64(50), "JPY"
	if _, err := e.Deals.UpdateDeal(admin, dealID, deals.UpdateDealInput{AmountMinor: &fifty, Currency: &yen}); err != nil {
		t.Fatal(err)
	}
	second := latestAuditRowID(t, e, "deal", id, "update")
	if _, err := restoreSeamFor(e).Restore(admin, "deal", id, second, currentVersion(t, e, "deal", id)); err != nil {
		t.Fatalf("putting the currency change back: %v", err)
	}
	if got := advisoryAnswer(admin, t, e, "deal", id, first); !got.Undoable {
		t.Errorf("the earlier amount step reads %q (%s) though the deal holds what that step left", got.Reason, got.Detail)
	}
}

func TestRestoringAnArchivedCompanyNamesTheLinkItCouldNotBringBack(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	company := e.SeedCompany(t, "Old Employer", nil)
	other := e.SeedCompany(t, "New Employer", nil)
	contact := e.SeedContact(t, "Employee", nil)
	cID, coID, otherID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company), ids.From[ids.CompanyKind](other)
	if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &cID, CompanyID: &coID}); err != nil {
		t.Fatalf("first employment: %v", err)
	}
	if _, err := e.Contacts.ArchiveCompany(admin, coID, nil); err != nil {
		t.Fatalf("archive company: %v", err)
	}
	if _, err := e.Contacts.CreateRelationship(admin, contacts.CreateRelationshipInput{Kind: "employment", ContactID: &cID, CompanyID: &otherID}); err != nil {
		t.Fatalf("second employment: %v", err)
	}
	entry := latestAuditRowID(t, e, "company", company, "archive")
	answer, err := restoreSeamFor(e).Restore(admin, "company", company, entry, currentVersion(t, e, "company", company))
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(answer.LeftBehind) != 1 || answer.LeftBehind[0].Kind != "relationship" {
		t.Errorf("the restore answered %+v, want the one employment link it left archived", answer.LeftBehind)
	}
}

func TestUndoingABulkRemoveFromListKeepsEachMembersNoteAndAddedTime(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	lists := collections.NewStore(e.DB())
	list, err := lists.CreateList(admin, collections.CreateListInput{Name: "Fair " + ids.NewV7().String(), EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	items := seedBulkContacts(t, e, e.Rep1, 2)
	orig := "orig note"
	for _, it := range items {
		if _, err := lists.AddMember(admin, list.ID, collections.MemberChange{
			EntityType: "contact", EntityID: ids.UUID(it.Id), Note: &orig, Reason: collections.ReasonChosen,
		}); err != nil {
			t.Fatal(err)
		}
	}
	addedTimes := func() string {
		return e.WsScalar(t, `SELECT string_agg(created_at::text, ',' ORDER BY entity_id) FROM list_member WHERE list_id = $1`, list.ID.UUID)
	}
	wasAdded := addedTimes()
	engine := bulkEngineFor(e).withListsIf(true)
	bulkNote, listID := "bulk note", list.ID.UUID
	removed, err := engine.Execute(admin, bulkChange{
		recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbRemoveFromList,
		items: items, listID: &listID, note: &bulkNote,
	})
	if err != nil || removed.Changed != 2 {
		t.Fatalf("remove → %+v, %v", removed, err)
	}
	if _, err := engine.Undo(admin, ids.UUID(removed.BatchId), ""); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member WHERE list_id = $1 AND note = $2`, listID, orig); n != 2 {
		t.Errorf("%d members came back with their own note, want 2", n)
	}
	if got := addedTimes(); got != wasAdded {
		t.Errorf("members came back added at %s, want their original %s", got, wasAdded)
	}
}

// Walking back through one's own run of edits still works: each reversal writes
// the value the earlier entry left, so the earlier entry reads as unmoved.
func TestSocialLinksWalkBackThroughTheirOwnHistory(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := e.SeedContact(t, "Walk Back", nil)
	contactID := ids.From[ids.ContactKind](id)
	entries := map[string]ids.UUID{}
	for _, v := range []string{"a", "b", "c"} {
		if _, err := e.Contacts.UpdateContact(admin, contactID, contacts.UpdateContactInput{Social: map[string]any{"linkedin": v}}); err != nil {
			t.Fatalf("set social %s: %v", v, err)
		}
		entries[v] = latestAuditRowID(t, e, "contact", id, "update")
	}
	seam := restoreSeamFor(e)
	for _, v := range []string{"c", "b"} {
		if _, err := seam.Restore(admin, "contact", id, entries[v], currentVersion(t, e, "contact", id)); err != nil {
			t.Fatalf("putting back the entry that set %q: %v", v, err)
		}
	}
	got, err := e.Contacts.GetContact(admin, contactID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Social == nil || (*got.Social)["linkedin"] != "a" {
		t.Errorf("after walking back two steps the contact holds %v, want linkedin a", got.Social)
	}
}

// The seat that may read a record and write none is shown the refusal the
// press would give, not a button, on every record type with a write gate.
func TestAReadOnlySeatIsToldItMayNotWriteAContactCompanyOrDealChange(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	viewer := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.ReadOnlyPerms)
	title, industry, renamed := "VP", "Software", "Renamed"
	contact := e.SeedContact(t, "Read Only Contact", nil)
	if _, err := e.Contacts.UpdateContact(admin, ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	company := e.SeedCompany(t, "Read Only Co", nil)
	if _, err := e.Contacts.UpdateCompany(admin, ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{Industry: &industry}); err != nil {
		t.Fatal(err)
	}
	pipeline, open, _ := integration.DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{Name: "Read Only Deal", PipelineID: pipeline, StageID: open, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateDeal(admin, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	for entityType, id := range map[string]ids.UUID{"contact": contact, "company": company, "deal": ids.UUID(deal.Id)} {
		entry := latestAuditRowID(t, e, entityType, id, "update")
		if got := advisoryAnswer(viewer, t, e, entityType, id, entry); got.Undoable || got.Reason != string(ReasonNotWritableByCaller) {
			t.Errorf("%s: a read-only seat is told %+v, want not_writable_by_caller", entityType, got)
		}
	}
}

// Only a timestamp column is compared as an instant: a title that happens to
// spell the same instant in another zone is still somebody else's text.
func TestATextThatSpellsTheSameInstantIsStillAColleaguesEdit(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := e.SeedContact(t, "Look Alike", nil)
	contactID := ids.From[ids.ContactKind](id)
	set := func(ctx context.Context, title string) {
		t.Helper()
		if _, err := e.Contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{Title: &title}); err != nil {
			t.Fatalf("set title %q: %v", title, err)
		}
	}
	set(admin, "Chair")
	set(admin, "2026-12-09T10:00:00Z")
	entry := latestAuditRowID(t, e, "contact", id, "update")
	set(opsSeat(e), "2026-12-09T17:00:00+07:00")
	if got := advisoryAnswer(admin, t, e, "contact", id, entry); got.Undoable {
		t.Error("a colleague's different text was read as the same value because it parses as the same instant")
	}
}

// A company's domains come back from their table in no promised order, so a
// walk back through two edits must not read the same set as moved.
func TestDomainsWalkBackWhateverOrderTheyAreReadIn(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	id := e.SeedCompany(t, "Two Domains", nil)
	companyID := ids.From[ids.CompanyKind](id)
	tag := id.String()[:8]
	set := func(domains ...string) {
		t.Helper()
		in := make([]contacts.CompanyDomainInput, len(domains))
		for i, d := range domains {
			in[i] = contacts.CompanyDomainInput{Domain: d + "-" + tag + ".test", IsPrimary: i == 0}
		}
		if _, err := e.Contacts.UpdateCompany(admin, companyID, contacts.UpdateCompanyInput{Domains: &in}); err != nil {
			t.Fatalf("set domains %v: %v", domains, err)
		}
	}
	set("x", "y")
	two := latestAuditRowID(t, e, "company", id, "update")
	set("z")
	one := latestAuditRowID(t, e, "company", id, "update")
	seam := restoreSeamFor(e)
	if _, err := seam.Restore(admin, "company", id, one, currentVersion(t, e, "company", id)); err != nil {
		t.Fatalf("putting back the single domain: %v", err)
	}
	if got := advisoryAnswer(admin, t, e, "company", id, two); !got.Undoable {
		t.Errorf("the earlier two-domain edit reads %q (%s) after the later one was put back", got.Reason, got.Detail)
	}
}
