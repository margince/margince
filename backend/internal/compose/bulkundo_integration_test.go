// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Undoing a bulk change over a real Postgres, with every record, tag, list
// membership and link seeded through the writer that owns it. The doors are
// proven in integration/bulkundo_doors_integration_test.go.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// surroundedContact is a contact seeded with one of everything an archive
// takes down: an email, a tag, a list membership and an employment.
type surroundedContact struct {
	item     crmcontracts.BulkItem
	email    string
	tag      ids.UUID
	list     ids.UUID
	employer ids.UUID
}

// seedSurroundedContacts creates n contacts, each tagged, on one list and
// employed by one company, through the contact, collections and relationship
// writers.
func seedSurroundedContacts(t *testing.T, e *integration.Env, n int) []surroundedContact {
	t.Helper()
	ctx := e.Admin()
	lists := collections.NewStore(e.DB())
	tag, err := lists.CreateTag(ctx, "undo-"+ids.NewV7().String(), nil, nil)
	if err != nil {
		t.Fatalf("seeding a tag: %v", err)
	}
	list, err := lists.CreateList(ctx, collections.CreateListInput{Name: "Undo " + ids.NewV7().String(), EntityType: "contact"})
	if err != nil {
		t.Fatalf("seeding a list: %v", err)
	}
	employer := e.SeedCompany(t, "Undo Employer "+ids.NewV7().String()[:8], nil)
	out := make([]surroundedContact, 0, n)
	for i := range n {
		email := fmt.Sprintf("undo-%d-%s@fable.test", i, ids.NewV7().String())
		created, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
			FullName: fmt.Sprintf("Undo Contact %d %s", i, ids.NewV7().String()[:8]),
			Emails:   []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
		})
		if err != nil {
			t.Fatalf("seeding contact %d: %v", i, err)
		}
		id := ids.UUID(created.Id)
		if _, err := lists.ApplyTag(ctx, tag.ID, "contact", id); err != nil {
			t.Fatalf("tagging contact %d: %v", i, err)
		}
		if _, err := lists.AddMember(ctx, list.ID, collections.MemberChange{EntityType: "contact", EntityID: id, Reason: collections.ReasonChosen}); err != nil {
			t.Fatalf("listing contact %d: %v", i, err)
		}
		contactID, companyID := ids.From[ids.ContactKind](id), ids.From[ids.CompanyKind](employer)
		if _, err := e.Contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
			Kind: "employment", ContactID: &contactID, CompanyID: &companyID,
		}); err != nil {
			t.Fatalf("employing contact %d: %v", i, err)
		}
		version := e.WsCount(t, `SELECT version FROM contact WHERE id = $1`, id)
		out = append(out, surroundedContact{
			item:  crmcontracts.BulkItem{Id: created.Id, Version: int64(version)},
			email: email, tag: tag.ID.UUID, list: list.ID.UUID, employer: employer,
		})
	}
	return out
}

func itemsOf(seeded []surroundedContact) []crmcontracts.BulkItem {
	items := make([]crmcontracts.BulkItem, len(seeded))
	for i, s := range seeded {
		items[i] = s.item
	}
	return items
}

func archiveContacts(items []crmcontracts.BulkItem) bulkChange {
	return bulkChange{recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbArchive, items: items}
}

// surroundings counts what an archive takes down, for one contact.
func surroundings(t *testing.T, e *integration.Env, s surroundedContact) (live, emails, tags, members, links int) {
	t.Helper()
	id := s.item.Id
	live = e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL`, id)
	emails = e.WsCount(t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, id)
	tags = e.WsCount(t, `SELECT count(*) FROM taggable WHERE entity_type = 'contact' AND entity_id = $1 AND tag_id = $2`, id, s.tag)
	members = e.WsCount(t, `SELECT count(*) FROM list_member WHERE entity_type = 'contact' AND entity_id = $1 AND list_id = $2`, id, s.list)
	links = e.WsCount(t, `SELECT count(*) FROM relationship
		WHERE contact_id = $1 AND company_id = $2 AND kind = 'employment' AND archived_at IS NULL`, id, s.employer)
	return live, emails, tags, members, links
}

func TestUndoingABulkArchiveBringsBackEachContactWithItsTagsListsAndLinks(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedSurroundedContacts(t, e, 5)
	engine := bulkEngineFor(e)

	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil || archived.Changed != 5 {
		t.Fatalf("archiving five → %+v, %v", archived, err)
	}
	for _, s := range seeded {
		if live, emails, tags, members, links := surroundings(t, e, s); live+emails+tags+members+links != 0 {
			t.Fatalf("the archive left %d/%d/%d/%d/%d of contact %s standing", live, emails, tags, members, links, s.item.Id)
		}
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if undone.Changed != 5 || len(undone.Skipped) != 0 || undone.LeftBehind != nil {
		t.Fatalf("undo → %+v; want all five back with nothing left behind", undone)
	}
	for _, s := range seeded {
		if live, emails, tags, members, links := surroundings(t, e, s); live+emails+tags+members+links != 5 {
			t.Errorf("contact %s came back with %d/%d/%d/%d/%d of itself, its email, tag, list and employer",
				s.item.Id, live, emails, tags, members, links)
		}
	}
	undoBatch := ids.UUID(undone.BatchId)
	if undoBatch == ids.UUID(archived.BatchId) || undone.UndoOf == nil || *undone.UndoOf != archived.BatchId {
		t.Errorf("the undo answered batch %s undoing %v; want its own batch linked to %s", undoBatch, undone.UndoOf, archived.BatchId)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o
		ON a.id::text = o.envelope#>>'{trace,audit_log_id}'
		WHERE a.batch_id = $1 AND a.action = 'restore' AND o.envelope->>'type' = 'contact.restored'`, undoBatch); n != 5 {
		t.Errorf("%d restore audit rows under the undo's batch carry a contact.restored event, want five", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM bulk_operation u JOIN bulk_operation o ON o.id = u.undo_of
		WHERE u.id = $1 AND o.undone_by = u.id AND u.changed_count = 5`, undoBatch); n != 1 {
		t.Error("the undo's own bulk_operation row is not linked both ways to the change it undid")
	}
}

func TestUndoingABulkReassignHandsEachContactBackToItsOwner(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 3)
	engine := bulkEngineFor(e)
	moved, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil || moved.Changed != 3 {
		t.Fatalf("reassigning three → %+v, %v", moved, err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), "")
	if err != nil || undone.Changed != 3 {
		t.Fatalf("Undo → %+v, %v; want all three handed back", undone, err)
	}
	for _, item := range items {
		if got := contactOwner(t, e, item.Id); got != e.Rep1.String() {
			t.Errorf("contact %s is owned by %q after the undo, want the owner it had before", item.Id, got)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND action = 'update'`, undone.BatchId); n != 3 {
		t.Errorf("%d update audit rows carry the undo's batch id, want three", n)
	}
}

func TestAnUndoLeavesAContactEditedSinceTheBatch(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	engine := bulkEngineFor(e)
	moved, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	title := "Buyer"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(items[1].Id)),
		contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("editing the contact after the batch: %v", err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		items[1].Id: crmcontracts.BulkSkipReasonChangedSinceBatch,
	})
	if got := contactOwner(t, e, items[1].Id); got != e.Rep2.String() {
		t.Errorf("the edited contact was handed back anyway (owner %q)", got)
	}
	if got := contactOwner(t, e, items[0].Id); got != e.Rep1.String() {
		t.Errorf("the untouched contact was not handed back (owner %q)", got)
	}
}

func TestAnUndoLeavesAnErasedContactArchived(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedSurroundedContacts(t, e, 2)
	engine := bulkEngineFor(e)
	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	erased := seeded[0].item.Id
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), ids.UUID(erased), "subject request"); err != nil {
		t.Fatalf("erasing the archived contact: %v", err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		erased: crmcontracts.BulkSkipReasonErased,
	})
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL`, erased); n != 0 {
		t.Error("the erased contact came back")
	}
}

func TestAnUndoLeavesAContactWhoseEmailAnotherContactHoldsNow(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedSurroundedContacts(t, e, 2)
	engine := bulkEngineFor(e)
	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	holder, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "New Holder",
		Emails:   []contacts.ContactEmailInput{{Email: seeded[0].email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("giving the archived contact's address to a new contact: %v", err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	assertReasons(t, "undo", skipReasons(undone.Skipped), map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		seeded[0].item.Id: crmcontracts.BulkSkipReasonValueTaken,
	})
	if undone.Changed != 1 {
		t.Errorf("undo changed %d, want the other contact back", undone.Changed)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, holder.Id); n != 1 {
		t.Error("the new holder lost the address to the undo")
	}
}

func TestAnUndoRestoresTheRecordAndNamesAMembershipWhoseListWasArchived(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedSurroundedContacts(t, e, 1)
	engine := bulkEngineFor(e)
	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, err := collections.NewStore(e.DB()).ArchiveList(e.Admin(), ids.From[ids.ListKind](seeded[0].list)); err != nil {
		t.Fatalf("archiving the list: %v", err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil || undone.Changed != 1 {
		t.Fatalf("Undo → %+v, %v; want the contact back", undone, err)
	}
	if undone.LeftBehind == nil || len(*undone.LeftBehind) != 1 ||
		(*undone.LeftBehind)[0].Kind != crmcontracts.BulkLeftBehindKindList ||
		(*undone.LeftBehind)[0].RefId != openapi_types.UUID(seeded[0].list) {
		t.Errorf("left behind %v; want exactly the archived list's membership", undone.LeftBehind)
	}
	if _, _, tags, members, _ := surroundings(t, e, seeded[0]); tags != 1 || members != 0 {
		t.Errorf("tags %d, memberships %d; want the tag back and no membership on an archived list", tags, members)
	}
}

func TestABulkChangeIsUndoneOnceAndAnUndoIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	moved, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, seedBulkContacts(t, e, e.Rep1, 1)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	undone, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), "")
	if err != nil {
		t.Fatalf("first Undo: %v", err)
	}
	if _, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), ""); !errors.Is(err, errBulkAlreadyUndone) {
		t.Errorf("a second undo → %v, want it refused as already undone", err)
	}
	if _, err := engine.PreviewUndo(e.Admin(), ids.UUID(undone.BatchId)); !errors.Is(err, errBulkUndoOfUndo) {
		t.Errorf("undoing the undo → %v, want it refused", err)
	}
}

// Two undos that both read the change as not yet undone still produce one: the
// second meets the unique index when it records itself.
func TestTheOnceRuleHoldsWhenTheFastCheckWasPassed(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	moved, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, seedBulkContacts(t, e, e.Rep1, 1)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	change, err := engine.undoChange(e.Admin(), ids.UUID(moved.BatchId))
	if err != nil {
		t.Fatalf("reading the change: %v", err)
	}
	if _, err := engine.Undo(e.Admin(), ids.UUID(moved.BatchId), ""); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := engine.Execute(e.Admin(), change); !errors.Is(err, errBulkAlreadyUndone) {
		t.Errorf("an undo planned before the first one committed → %v, want it refused", err)
	}
}

func TestOnlyTheRequesterOrAnAdminReadsOrUndoesABatch(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.RepPerms)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	moved, err := engine.Execute(rep1, reassignTo(e.Rep2, items))
	if err != nil || moved.Changed != 2 {
		t.Fatalf("the rep's reassign → %+v, %v", moved, err)
	}
	batch := ids.UUID(moved.BatchId)

	status, err := engine.Status(rep1, batch)
	if err != nil || status.Changed != 2 || status.Verb != crmcontracts.BulkVerbReassignOwner || status.UndoneBy != nil {
		t.Fatalf("the requester's status read → %+v, %v", status, err)
	}
	if _, err := engine.Status(rep2, batch); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague's status read → %v, want not found", err)
	}
	if _, err := engine.Undo(rep2, batch, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a colleague's undo → %v, want not found", err)
	}
	undone, err := engine.Undo(e.Admin(), batch, "")
	if err != nil || undone.Changed != 2 {
		t.Fatalf("the admin's undo → %+v, %v", undone, err)
	}
	if status, err := engine.Status(rep1, batch); err != nil || status.UndoneBy == nil || *status.UndoneBy != undone.BatchId {
		t.Errorf("after the undo the requester reads %+v, %v; want undone_by naming the admin's undo", status, err)
	}
}

func TestAnUndoAboveTenNeedsItsPreviewsToken(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	items := seedBulkContacts(t, e, e.Rep1, 11)
	preview, err := engine.Preview(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	change := reassignTo(e.Rep2, items)
	change.confirmToken = *preview.ConfirmToken
	moved, err := engine.Execute(e.Admin(), change)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	batch := ids.UUID(moved.BatchId)
	if _, err := engine.Undo(e.Admin(), batch, ""); !errors.Is(err, errConfirmTokenRequired) {
		t.Errorf("undoing eleven without a token → %v, want the token required", err)
	}
	if _, err := engine.Undo(e.Admin(), batch, *preview.ConfirmToken); !errors.Is(err, errConfirmTokenRefused) {
		t.Errorf("undoing with the change's own token → %v, want it refused", err)
	}
	undoPreview, err := engine.PreviewUndo(e.Admin(), batch)
	if err != nil || undoPreview.Count != 11 || undoPreview.ConfirmToken == nil {
		t.Fatalf("PreviewUndo → %+v, %v", undoPreview, err)
	}
	if undone, err := engine.Undo(e.Admin(), batch, *undoPreview.ConfirmToken); err != nil || undone.Changed != 11 {
		t.Errorf("undoing with the undo preview's token → %+v, %v", undone, err)
	}
}

// An undo's token confirms that undo and nothing else, even an execution
// naming the very same records at the very same versions.
func TestAnUndoTokenDoesNotConfirmAForwardChange(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	seeded := seedSurroundedContacts(t, e, 2)
	archived, err := engine.Execute(e.Admin(), archiveContacts(itemsOf(seeded)))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	undoPreview, err := engine.PreviewUndo(e.Admin(), ids.UUID(archived.BatchId))
	if err != nil || undoPreview.ConfirmToken == nil {
		t.Fatalf("PreviewUndo → %+v, %v", undoPreview, err)
	}
	now := itemsOf(seeded)
	for i := range now {
		now[i].Version = int64(e.WsCount(t, `SELECT version FROM contact WHERE id = $1`, now[i].Id))
	}
	forward := archiveContacts(now)
	forward.confirmToken = *undoPreview.ConfirmToken
	if _, err := engine.Execute(e.Admin(), forward); !errors.Is(err, errConfirmTokenRefused) {
		t.Errorf("an archive presenting the undo's token → %v, want the token refused", err)
	}
}

// restoreIn runs one single-record restore on its own transaction.
func restoreIn(t *testing.T, e *integration.Env, restore func(tx pgx.Tx) (storekit.RestoreReport, error)) error {
	t.Helper()
	return e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := restore(tx)
		return err
	})
}

func TestEachSingleRecordRestoreEmitsItsEventAndNamesTheArchive(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Restored Contact", nil))
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Restored Company", nil))
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Restored Deal", pipeline, open, nil))
	if _, err := e.Contacts.ArchiveContact(ctx, contact, nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	if _, err := e.Contacts.ArchiveCompany(ctx, company, nil); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}
	if _, err := e.Deals.ArchiveDeal(ctx, deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	for _, restore := range []struct {
		entity, event string
		id            ids.UUID
		run           func(tx pgx.Tx) (storekit.RestoreReport, error)
	}{
		{"contact", "contact.restored", contact.UUID, func(tx pgx.Tx) (storekit.RestoreReport, error) {
			return e.Contacts.RestoreContactTx(ctx, tx, contact, nil, unarchiveWith(nil))
		}},
		{"company", "company.restored", company.UUID, func(tx pgx.Tx) (storekit.RestoreReport, error) {
			return e.Contacts.RestoreCompanyTx(ctx, tx, company, nil, unarchiveWith(nil))
		}},
		{"deal", "deal.restored", deal.UUID, func(tx pgx.Tx) (storekit.RestoreReport, error) {
			return e.Deals.RestoreDealTx(ctx, tx, deal, nil, unarchiveWith(nil))
		}},
	} {
		if err := restoreIn(t, e, restore.run); err != nil {
			t.Fatalf("restoring the %s: %v", restore.entity, err)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o
			ON a.id::text = o.envelope#>>'{trace,audit_log_id}'
			WHERE a.entity_id = $1 AND a.action = 'restore' AND o.envelope->>'type' = $2
			  AND a.evidence->>'restores_archive' = (SELECT id::text FROM audit_log
			      WHERE entity_id = $1 AND action = 'archive')`, restore.id, restore.event); n != 1 {
			t.Errorf("the %s restore wrote %d audit rows naming its archive with a %s event, want one", restore.entity, n, restore.event)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM `+restore.entity+` WHERE id = $1 AND archived_at IS NULL`, restore.id); n != 1 {
			t.Errorf("the %s is still archived", restore.entity)
		}
	}
}

func TestASingleRecordRestoreRefusesAMergedOrLiveContact(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	source := ids.From[ids.ContactKind](e.SeedContact(t, "Merged Away", nil))
	target := ids.From[ids.ContactKind](e.SeedContact(t, "Survivor", nil))
	if _, err := e.Contacts.MergeContact(ctx, source, target, nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	for name, id := range map[string]ids.ContactID{"merged": source, "not_archived": target} {
		err := restoreIn(t, e, func(tx pgx.Tx) (storekit.RestoreReport, error) {
			return e.Contacts.RestoreContactTx(ctx, tx, id, nil, unarchiveWith(nil))
		})
		var refusal *storekit.RestoreRefusal
		if !errors.As(err, &refusal) || string(refusal.Reason) != name {
			t.Errorf("restoring the %s contact → %v, want refused as %s", name, err, name)
		}
	}
}
