// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Undoing what a machine did to a record through the module verb that reverses
// it: a create by archiving, an archive by un-archiving, a promotion by
// demoting, a fill by clearing. Every record here is written by the real module
// writer under a machine principal, the way an import or the mail reader writes
// it, and every undo goes through the restore route's own seam.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// machineCtx is a background job's principal: the system, under a job name.
func machineCtx(e *integration.Env) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test-import"})
}

// undoEntry presses Undo on one entry as the admin, pinned to the record's
// current version.
func undoEntry(t *testing.T, e *integration.Env, entityType string, id, auditID ids.UUID) error {
	t.Helper()
	_, err := restoreSeamFor(e).Restore(e.Admin(), entityType, id, auditID, currentVersion(t, e, entityType, id))
	return err
}

// refusedFor names the reason a refused undo gave, or fails the test when it
// was not a refusal at all.
func refusedFor(t *testing.T, err error) Reason {
	t.Helper()
	var refusal RefusedRestore
	if !errors.As(err, &refusal) {
		t.Fatalf("the undo answered %v, want a refusal naming its reason", err)
	}
	return refusal.Reason
}

// isArchived reads whether one record is archived now.
func isArchived(t *testing.T, e *integration.Env, entityType string, id ids.UUID) bool {
	t.Helper()
	archived, err := func() (bool, error) {
		var out bool
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = recordIsArchived(context.Background(), tx, entityType, id)
			return err
		})
		return out, err
	}()
	if err != nil {
		t.Fatalf("read whether the %s is archived: %v", entityType, err)
	}
	return archived
}

// A contact an import created is undone by archiving it, and the archive names
// the entry it reverses, so the create then reads as already undone. The
// archive is itself an entry, and undoing it brings the contact back.
func TestUndoingAMachineCreateArchivesTheRecordAndCanBeRedone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatalf("an import creating the contact: %v", err)
	}
	contact := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", contact, actionCreate)

	if answer := advisoryAnswer(e.Admin(), t, e, "contact", contact, createID); !answer.Undoable {
		t.Fatalf("the create of an untouched imported contact reads %+v, want undoable", answer)
	}
	if err := undoEntry(t, e, "contact", contact, createID); err != nil {
		t.Fatalf("undoing the create: %v", err)
	}
	if !isArchived(t, e, "contact", contact) {
		t.Fatal("the create was undone and the contact is still live")
	}
	if got := reversalsOf(t, e, createID); got != 1 {
		t.Errorf("%d audit rows name the create as the entry they reverse, want the one archive", got)
	}
	if answer := advisoryAnswer(e.Admin(), t, e, "contact", contact, createID); answer.Reason != string(ReasonAlreadyUndone) {
		t.Errorf("the undone create reads %+v, want %q", answer, ReasonAlreadyUndone)
	}

	archiveID := latestAuditRowID(t, e, "contact", contact, actionArchive)
	if err := undoEntry(t, e, "contact", contact, archiveID); err != nil {
		t.Fatalf("undoing the archive the undo wrote: %v", err)
	}
	if isArchived(t, e, "contact", contact) {
		t.Error("undoing the archive left the contact archived")
	}
}

// A colleague's work on the record wins: archiving it would take their edit out
// of view along with the machine's create.
func TestACreateAColleagueHasWorkedOnIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", contact, actionCreate)
	title := "Head of Fleet"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](contact), contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("a colleague editing the contact: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, createID)); reason != ReasonSuperseded {
		t.Errorf("the undo of a create a colleague has edited since refused %q, want %q", reason, ReasonSuperseded)
	}
	if isArchived(t, e, "contact", contact) {
		t.Error("the refused undo archived the contact anyway")
	}
}

// The mail reader archived a company; undoing that archive brings it back
// through the contacts module's own un-archive.
func TestUndoingAMachineArchiveBringsTheRecordBack(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Noise GmbH", nil)
	if _, err := NewProvider(e.Pool).Archive(machineCtx(e), datasource.EntityRef{Type: "company", ID: company}); err != nil {
		t.Fatalf("the machine archiving the company: %v", err)
	}
	archiveID := latestAuditRowID(t, e, "company", company, actionArchive)

	if answer := advisoryAnswer(e.Admin(), t, e, "company", company, archiveID); !answer.Undoable {
		t.Fatalf("the archive reads %+v, want undoable", answer)
	}
	if err := undoEntry(t, e, "company", company, archiveID); err != nil {
		t.Fatalf("undoing the archive: %v", err)
	}
	if isArchived(t, e, "company", company) {
		t.Fatal("the archive was undone and the company is still archived")
	}
	if got := reversalsOf(t, e, archiveID); got != 1 {
		t.Errorf("%d audit rows name the archive as the entry they reverse, want the one restore", got)
	}
	if answer := advisoryAnswer(e.Admin(), t, e, "company", company, archiveID); answer.Reason != string(ReasonAlreadyUndone) {
		t.Errorf("the undone archive reads %+v, want %q", answer, ReasonAlreadyUndone)
	}

	// Redo: undoing the un-archive archives the company again.
	restoreID := latestAuditRowID(t, e, "company", company, actionRestore)
	if err := undoEntry(t, e, "company", company, restoreID); err != nil {
		t.Fatalf("redoing the archive: %v", err)
	}
	if !isArchived(t, e, "company", company) {
		t.Error("the redo left the company live")
	}
}

// An archive already undone stays undone when the record is archived again:
// an un-archive now would reverse the LATER archive, not the one on screen.
func TestAnArchiveAlreadyUndoneStaysUndoneAfterALaterArchive(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Twice Archived AG", nil)
	provider := NewProvider(e.Pool)
	if _, err := provider.Archive(machineCtx(e), datasource.EntityRef{Type: "company", ID: company}); err != nil {
		t.Fatal(err)
	}
	first := latestAuditRowID(t, e, "company", company, actionArchive)
	if err := undoEntry(t, e, "company", company, first); err != nil {
		t.Fatalf("undoing the first archive: %v", err)
	}
	if _, err := provider.Archive(machineCtx(e), datasource.EntityRef{Type: "company", ID: company}); err != nil {
		t.Fatal(err)
	}
	if answer := advisoryAnswer(e.Admin(), t, e, "company", company, first); answer.Reason != string(ReasonAlreadyUndone) {
		t.Errorf("the first archive reads %+v, want %q", answer, ReasonAlreadyUndone)
	}
}

// A lead the router promoted is undone by demoting it: the lead is live again
// and the contact the promotion created is archived with it.
func TestUndoingAMachinePromotionDemotesTheLead(t *testing.T) {
	e := integration.Setup(t)
	name, email := "Lena Lead", "lena@prospect.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	contact, _, err := e.Contacts.PromoteLead(machineCtx(e), ids.From[ids.LeadKind](leadID), contacts.PromoteLeadInput{Trigger: "inbound_reply"})
	if err != nil {
		t.Fatalf("the machine promoting the lead: %v", err)
	}
	promoteID := latestAuditRowID(t, e, entityTypeLead, leadID, actionPromote)

	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeLead, leadID, promoteID); !answer.Undoable {
		t.Fatalf("the promotion reads %+v, want undoable", answer)
	}
	if err := undoEntry(t, e, entityTypeLead, leadID, promoteID); err != nil {
		t.Fatalf("undoing the promotion: %v", err)
	}
	if isArchived(t, e, entityTypeLead, leadID) {
		t.Error("the demoted lead is still archived")
	}
	if !isArchived(t, e, "contact", ids.UUID(contact.Id)) {
		t.Error("the contact the promotion created is still live after the demotion")
	}
	if answer := advisoryAnswer(e.Admin(), t, e, entityTypeLead, leadID, promoteID); answer.Reason != string(ReasonAlreadyUndone) {
		t.Errorf("the undone promotion reads %+v, want %q", answer, ReasonAlreadyUndone)
	}
}

// A colleague who tagged or shortlisted an imported contact did their work on
// the tag and the list, not the contact; archiving it would drop both.
func TestACreateAColleagueTaggedIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", contact, actionCreate)
	taggedAndListed(t, e, "contact", contact)

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, createID)); reason != ReasonSuperseded {
		t.Errorf("the undo of a create a colleague tagged refused %q, want %q", reason, ReasonSuperseded)
	}
	if isArchived(t, e, "contact", contact) {
		t.Error("the refused undo archived the contact anyway")
	}
}

// A colleague who merged another contact into an imported one made the
// imported one the survivor of their merge; archiving it would take the merged
// contact's data with it.
func TestACreateAColleagueMergedIntoIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	survivor := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", survivor, actionCreate)
	duplicate := e.SeedContact(t, "Ida Imported", nil)
	if _, err := e.Contacts.MergeContact(e.Admin(), ids.From[ids.ContactKind](duplicate), ids.From[ids.ContactKind](survivor)); err != nil {
		t.Fatalf("a colleague merging the duplicate in: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", survivor, createID)); reason != ReasonSuperseded {
		t.Errorf("the undo of a create a colleague merged into refused %q, want %q", reason, ReasonSuperseded)
	}
}

// A colleague who corrected what Margince read about an imported contact did
// that work in the correction ledger, not on the contact; archiving the contact
// would take it out of view.
func TestACreateAColleagueCorrectedIsNotUndone(t *testing.T) {
	e := integration.Setup(t)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import"})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	createID := latestAuditRowID(t, e, "contact", contact, actionCreate)
	corrected := "Head of Engineering"
	if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(e.Admin(), ai.RecordInput{
		SubjectType: "contact", SubjectID: contact, ClaimKind: ai.ClaimProfileField,
		ClaimPath: ai.ProfileFieldClaimPath("title"), Verdict: ai.VerdictCorrected, CorrectedValue: &corrected,
	}); err != nil {
		t.Fatalf("a colleague correcting the title: %v", err)
	}

	if reason := refusedFor(t, undoEntry(t, e, "contact", contact, createID)); reason != ReasonSuperseded {
		t.Errorf("the undo of a create a colleague corrected refused %q, want %q", reason, ReasonSuperseded)
	}
}

// A deal a machine made is undone through the deals module's own archive, and
// its archive is undone through the deals module's own un-archive.
func TestUndoingAMachineDealCreateAndItsArchive(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	created, err := e.Deals.CreateDeal(machineCtx(e), deals.CreateDealInput{Name: "Imported Fleet", PipelineID: pipeline, StageID: open})
	if err != nil {
		t.Fatalf("a machine creating the deal: %v", err)
	}
	deal := ids.UUID(created.Id)
	if err := undoEntry(t, e, entityTypeDeal, deal, latestAuditRowID(t, e, entityTypeDeal, deal, actionCreate)); err != nil {
		t.Fatalf("undoing the deal's create: %v", err)
	}
	if !isArchived(t, e, entityTypeDeal, deal) {
		t.Fatal("the create was undone and the deal is still live")
	}
	if err := undoEntry(t, e, entityTypeDeal, deal, latestAuditRowID(t, e, entityTypeDeal, deal, actionArchive)); err != nil {
		t.Fatalf("undoing the archive: %v", err)
	}
	if isArchived(t, e, entityTypeDeal, deal) {
		t.Error("the archive was undone and the deal is still archived")
	}
}

// A note a machine logged is undone through the activities module's archive.
func TestUndoingAMachineActivityCreateArchivesIt(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Noted Nora", nil)
	subject := "captured"
	logged, _, err := e.Activities.LogActivity(machineCtx(e), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "import",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("a machine logging the note: %v", err)
	}
	activity := ids.UUID(logged.Id)
	if err := undoEntry(t, e, entityTypeActivity, activity, latestAuditRowID(t, e, entityTypeActivity, activity, actionCreate)); err != nil {
		t.Fatalf("undoing the note's create: %v", err)
	}
	if !isArchived(t, e, entityTypeActivity, activity) {
		t.Error("the create was undone and the note is still live")
	}
}

// Undoing a create archives the record, which asks the delete grant: a seat
// that may edit contacts and not archive them is told so up front.
func TestACreateUndoIsRefusedToASeatThatMayNotArchive(t *testing.T) {
	e := integration.Setup(t)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Contacts.CreateContact(machineCtx(e), contacts.CreateContactInput{FullName: "Imported Ida", Source: "import", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.UUID(created.Id)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	answer := advisoryAnswer(rep, t, e, "contact", contact, latestAuditRowID(t, e, "contact", contact, actionCreate))
	if answer.Undoable || answer.Reason != string(ReasonNotWritableByCaller) {
		t.Errorf("a seat without contact delete was answered %+v, want %q", answer, ReasonNotWritableByCaller)
	}
}

// A demotion writes the contact as well as the lead, so a seat that may change
// leads and not contacts is told so up front.
func TestAPromotionUndoIsRefusedToASeatThatMayNotChangeTheContact(t *testing.T) {
	e := integration.Setup(t)
	name, email := "Lena Lead", "lena@prospect.test"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	if _, _, err := e.Contacts.PromoteLead(machineCtx(e), ids.From[ids.LeadKind](leadID), contacts.PromoteLeadInput{Trigger: "inbound_reply"}); err != nil {
		t.Fatal(err)
	}
	seat := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"lead": {Read: true, Update: true}, "contact": {Read: true}},
	})
	answer := advisoryAnswer(seat, t, e, entityTypeLead, leadID, latestAuditRowID(t, e, entityTypeLead, leadID, actionPromote))
	if answer.Undoable || answer.Reason != string(ReasonNotWritableByCaller) {
		t.Errorf("a seat without contact update was answered %+v, want %q", answer, ReasonNotWritableByCaller)
	}
}
