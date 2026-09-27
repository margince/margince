// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The bulk-change engine over a real Postgres, under the principals whose
// authority it has to respect. The doors (HTTP and the tool) are proven alike
// in integration/bulkchange_doors_integration_test.go; this file is about what
// the engine decides per record.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func bulkEngineFor(e *integration.Env) *bulkEngine {
	return newBulkEngine(e.DB(), nil)
}

// seedBulkContacts creates n contacts owned by owner through the contact
// writer, and answers each as the item a list row would carry.
func seedBulkContacts(t *testing.T, e *integration.Env, owner ids.UUID, n int) []crmcontracts.BulkItem {
	t.Helper()
	items := make([]crmcontracts.BulkItem, 0, n)
	for i := range n {
		ownerID := ids.From[ids.UserKind](owner)
		created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
			FullName: fmt.Sprintf("Bulk Contact %d %s", i, ids.NewV7().String()[:8]), OwnerID: &ownerID,
		})
		if err != nil {
			t.Fatalf("seeding contact %d: %v", i, err)
		}
		items = append(items, crmcontracts.BulkItem{Id: created.Id, Version: *created.Version})
	}
	return items
}

func reassignTo(owner ids.UUID, items []crmcontracts.BulkItem) bulkChange {
	return bulkChange{
		recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbReassignOwner,
		items: items, ownerID: &owner,
	}
}

func contactOwner(t *testing.T, e *integration.Env, id openapi_types.UUID) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(owner_id::text, '') FROM contact WHERE id = $1`, id)
}

func skipReasons(skipped []crmcontracts.BulkSkip) map[openapi_types.UUID]crmcontracts.BulkSkipReason {
	out := map[openapi_types.UUID]crmcontracts.BulkSkipReason{}
	for _, s := range skipped {
		out[s.Id] = s.Reason
	}
	return out
}

func TestABulkReassignWritesEachRecordsOwnAuditRowAndEventUnderOneBatch(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 5)

	out, err := bulkEngineFor(e).Execute(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Changed != 5 || len(out.Skipped) != 0 {
		t.Fatalf("changed %d, skipped %v; want all five changed", out.Changed, out.Skipped)
	}
	for _, item := range items {
		if got := contactOwner(t, e, item.Id); got != e.Rep2.String() {
			t.Errorf("contact %s is owned by %q, want the new owner", item.Id, got)
		}
	}
	batch := ids.UUID(out.BatchId)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE batch_id = $1 AND action = 'update' AND entity_type = 'contact'`, batch); n != 5 {
		t.Errorf("%d update audit rows carry the batch id, want one per contact", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a
		ON a.id::text = o.envelope#>>'{trace,audit_log_id}'
		WHERE a.batch_id = $1 AND o.envelope->>'type' = 'contact.updated'`, batch); n != 5 {
		t.Errorf("%d contact.updated events trace to the batch's audit rows, want five", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM bulk_operation
		WHERE id = $1 AND changed_count = 5 AND skipped_count = 0 AND requested_by = $2
		  AND params->>'owner_id' = $3`, batch, "human:"+e.AdminUser.String(), e.Rep2.String()); n != 1 {
		t.Error("the change's own row does not record who asked, for whom, and how it went")
	}
}

func TestAPreviewExcludesWhatTheExecutionSkipsAndWritesNothing(t *testing.T) {
	e := integration.Setup(t)
	own := seedBulkContacts(t, e, e.Rep1, 2)
	colleagues := seedBulkContacts(t, e, e.Rep2, 1)
	foreign := seedBulkContacts(t, e, e.Rep3, 1)
	stale := own[1]
	stale.Version--
	missing := crmcontracts.BulkItem{Id: openapi_types.UUID(ids.NewV7()), Version: 1}
	items := []crmcontracts.BulkItem{own[0], stale, colleagues[0], foreign[0], missing}
	want := map[openapi_types.UUID]crmcontracts.BulkSkipReason{
		stale.Id:         crmcontracts.BulkSkipReasonChangedSincePreview,
		colleagues[0].Id: crmcontracts.BulkSkipReasonNoChange,
		foreign[0].Id:    crmcontracts.BulkSkipReasonNotWritable,
		missing.Id:       crmcontracts.BulkSkipReasonNotFound,
	}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	engine := bulkEngineFor(e)

	preview, err := engine.Preview(rep1, reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.Count != 1 || len(preview.Affected) != 1 || preview.Affected[0] != own[0].Id {
		t.Fatalf("preview affects %v, want only the caller's own current contact", preview.Affected)
	}
	assertReasons(t, "preview", skipReasons(preview.Excluded), want)
	if len(preview.Sample) != 1 || preview.Sample[0].After.OwnerId == nil ||
		ids.UUID(*preview.Sample[0].After.OwnerId) != e.Rep2 {
		t.Errorf("sample %+v does not show the new owner", preview.Sample)
	}
	if got := contactOwner(t, e, own[0].Id); got != e.Rep1.String() {
		t.Fatalf("the preview moved the contact to %q", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, own[0].Id); n != 0 {
		t.Fatalf("the preview left %d audit rows behind", n)
	}

	out, err := engine.Execute(rep1, reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Changed != 1 {
		t.Errorf("changed %d, want the one the preview named", out.Changed)
	}
	assertReasons(t, "execution", skipReasons(out.Skipped), want)
	if got := contactOwner(t, e, foreign[0].Id); got != e.Rep3.String() {
		t.Errorf("the contact the caller may not change was moved to %q", got)
	}
}

func assertReasons(t *testing.T, door string, got, want map[openapi_types.UUID]crmcontracts.BulkSkipReason) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s left %d records alone, want %d: %v", door, len(got), len(want), got)
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Errorf("%s gives %s the reason %q, want %q", door, id, got[id], reason)
		}
	}
}

func TestANamedOwnerWhoMayNotOwnRecordsRefusesTheWholeChange(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep3)

	for name, run := range map[string]func() error{
		"preview": func() error { _, err := bulkEngineFor(e).Preview(e.Admin(), reassignTo(e.Rep3, items)); return err },
		"execute": func() error { _, err := bulkEngineFor(e).Execute(e.Admin(), reassignTo(e.Rep3, items)); return err },
	} {
		if err := run(); !errors.As(err, new(*auth.AssigneeNotAllowedError)) {
			t.Errorf("%s to a suspended colleague → %v, want the assignee refusal", name, err)
		}
	}
	if got := contactOwner(t, e, items[0].Id); got != e.Rep1.String() {
		t.Errorf("a refused change still moved a contact to %q", got)
	}
}

func TestTheInstallationsOwnCompanyIsLeftOutOfABulkArchive(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	anchor, err := store.SaveCompany(e.Admin(), contacts.SaveCompanyInput{DisplayName: "Ourselves GmbH"})
	if err != nil {
		t.Fatalf("SaveCompany: %v", err)
	}
	other, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Customer AG", Source: "manual"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	anchorRow, err := store.GetCompany(e.Admin(), anchor.CompanyID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("GetCompany: %v", err)
	}
	change := bulkChange{
		recordType: crmcontracts.BulkRecordTypeCompany, verb: crmcontracts.BulkVerbArchive,
		items: []crmcontracts.BulkItem{
			{Id: anchorRow.Id, Version: *anchorRow.Version},
			{Id: other.Id, Version: *other.Version},
		},
	}

	out, err := bulkEngineFor(e).Execute(e.Admin(), change)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Changed != 1 || skipReasons(out.Skipped)[anchorRow.Id] != crmcontracts.BulkSkipReasonAnchorCompany {
		t.Fatalf("changed %d, skipped %v; want the customer archived and the anchor left out", out.Changed, out.Skipped)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company WHERE id = $1 AND archived_at IS NOT NULL`, other.Id); n != 1 {
		t.Error("the customer company was not archived")
	}
}

func TestAChangeOfMoreThanTenNeedsItsPreviewsTokenAndTheTokenOpensOneExecution(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 12)
	selection := items[:11]
	engine := bulkEngineFor(e)

	if _, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, selection)); !refusedWithCode(err, "confirm_token_required") {
		t.Fatalf("executing eleven records without a token → %v, want confirm_token_required", err)
	}
	preview, err := engine.Preview(e.Admin(), reassignTo(e.Rep2, selection))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !preview.RequiresConfirmation || preview.ConfirmToken == nil || preview.ExpiresAt == nil {
		t.Fatalf("an eleven-record preview carries no token: %+v", preview)
	}
	token := *preview.ConfirmToken
	withToken := func(change bulkChange) bulkChange { change.confirmToken = token; return change }

	swapped := append(append([]crmcontracts.BulkItem{}, items[1:11]...), items[11])
	for name, change := range map[string]bulkChange{
		"another selection": withToken(reassignTo(e.Rep2, swapped)),
		"another owner":     withToken(reassignTo(e.Rep3, selection)),
	} {
		if _, err := engine.Execute(e.Admin(), change); !refusedWithCode(err, "confirm_token_invalid") {
			t.Errorf("the token presented for %s → %v, want confirm_token_invalid", name, err)
		}
	}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, err := engine.Execute(rep1, withToken(reassignTo(e.Rep2, selection))); !refusedWithCode(err, "confirm_token_invalid") {
		t.Errorf("another user's token → %v, want confirm_token_invalid", err)
	}

	out, err := engine.Execute(e.Admin(), withToken(reassignTo(e.Rep2, selection)))
	if err != nil || out.Changed != 11 {
		t.Fatalf("executing with the token → %+v, %v; want all eleven changed", out, err)
	}
	if _, err := engine.Execute(e.Admin(), withToken(reassignTo(e.Rep2, selection))); !refusedWithCode(err, "confirm_token_invalid") {
		t.Errorf("a spent token → %v, want confirm_token_invalid", err)
	}
}

func refusedWithCode(err error, code string) bool {
	var detailed *httperr.DetailedError
	return errors.As(err, &detailed) && len(detailed.Fields) == 1 && detailed.Fields[0].Code == code
}

func TestARecordEditedAfterThePreviewIsSkippedNotOverwritten(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	engine := bulkEngineFor(e)
	if _, err := engine.Preview(e.Admin(), reassignTo(e.Rep2, items)); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	title := "Head of Purchasing"
	if _, err := e.Contacts.UpdateContact(e.Admin(), ids.From[ids.ContactKind](ids.UUID(items[1].Id)),
		contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("editing the contact after the preview: %v", err)
	}

	out, err := engine.Execute(e.Admin(), reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Changed != 1 || skipReasons(out.Skipped)[items[1].Id] != crmcontracts.BulkSkipReasonChangedSincePreview {
		t.Fatalf("changed %d, skipped %v; want the edited contact skipped", out.Changed, out.Skipped)
	}
	if got := contactOwner(t, e, items[1].Id); got != e.Rep1.String() {
		t.Errorf("the edited contact was overwritten to %q", got)
	}
}

// A row the database refuses mid-change rolls back to its own savepoint: the
// change goes on, and the rows around it land.
func TestARowTheDatabaseRefusesLeavesTheOtherRowsChanged(t *testing.T) {
	e := integration.Setup(t)
	items := seedBulkContacts(t, e, e.Rep1, 3)
	refused := items[1]
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), fmt.Sprintf(
		`ALTER TABLE contact ADD CONSTRAINT bulk_test_refuses_one CHECK (id <> '%s' OR archived_at IS NULL) NOT VALID`,
		ids.UUID(refused.Id))); err != nil {
		t.Fatalf("installing the refusal: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE contact DROP CONSTRAINT IF EXISTS bulk_test_refuses_one`); err != nil {
			t.Errorf("removing the refusal: %v", err)
		}
	})

	out, err := bulkEngineFor(e).Execute(e.Admin(), bulkChange{
		recordType: crmcontracts.BulkRecordTypeContact, verb: crmcontracts.BulkVerbArchive, items: items,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Changed != 2 || skipReasons(out.Skipped)[refused.Id] != crmcontracts.BulkSkipReasonRefused {
		t.Fatalf("changed %d, skipped %v; want the two others archived and the refused one reported", out.Changed, out.Skipped)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE id = ANY($1) AND archived_at IS NOT NULL`,
		[]openapi_types.UUID{items[0].Id, items[2].Id}); n != 2 {
		t.Errorf("%d of the two unrefused contacts were archived", n)
	}
}

func TestADeadlockedAttemptRunsOnceMoreAndNoMore(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	deadlock := &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}

	attempts := 0
	if err := engine.transact(e.Admin(), func(pgx.Tx) error {
		attempts++
		if attempts == 1 {
			return deadlock
		}
		return nil
	}); err != nil || attempts != 2 {
		t.Errorf("a deadlock then success → %v after %d attempts, want success after two", err, attempts)
	}

	attempts = 0
	if err := engine.transact(e.Admin(), func(pgx.Tx) error { attempts++; return deadlock }); !errors.Is(err, deadlock) || attempts != 2 {
		t.Errorf("two deadlocks → %v after %d attempts, want the deadlock after two", err, attempts)
	}
}

// A caller whose role may not change the record type at all is refused the
// whole change, rather than told every row is someone else's.
func TestACallerWithoutTheGrantIsRefusedTheWholeChange(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	company, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Grantless AG", Source: "manual"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	_, err = bulkEngineFor(e).Preview(rep1, bulkChange{
		recordType: crmcontracts.BulkRecordTypeCompany, verb: crmcontracts.BulkVerbArchive,
		items: []crmcontracts.BulkItem{{Id: company.Id, Version: *company.Version}},
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("archiving companies without the company grant → %v, want permission denied", err)
	}
}

// A preview can exclude a record only because another one ahead of it changes:
// archiving A leaves B the last company on their project. If A then moves before
// the execution, the execution must not archive B in its place — it may change
// only what the user was shown.
func TestAnExecutionNeverChangesARecordItsPreviewExcluded(t *testing.T) {
	e := integration.Setup(t)
	store := contacts.NewStore(e.DB())
	first, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "First GmbH", Source: "manual"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	second, err := store.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Second GmbH", Source: "manual"})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Shared rollout", CompanyID: ids.From[ids.CompanyKind](ids.UUID(first.Id)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := store.SetProjectCompany(e.Admin(), contacts.SetProjectCompanyInput{
		ProjectID: ids.From[ids.ProjectKind](ids.UUID(project.Id)), CompanyID: ids.From[ids.CompanyKind](ids.UUID(second.Id)),
	}); err != nil {
		t.Fatalf("SetProjectCompany: %v", err)
	}
	change := bulkChange{
		recordType: crmcontracts.BulkRecordTypeCompany, verb: crmcontracts.BulkVerbArchive,
		items: []crmcontracts.BulkItem{
			{Id: first.Id, Version: currentVersion(t, e, "company", ids.UUID(first.Id))},
			{Id: second.Id, Version: currentVersion(t, e, "company", ids.UUID(second.Id))},
		},
	}
	engine := bulkEngineFor(e)
	preview, err := engine.Preview(e.Admin(), change)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(preview.Affected) != 1 || preview.Affected[0] != first.Id || preview.ConfirmToken == nil {
		t.Fatalf("preview affects %v with token %v; want only the first company and a token", preview.Affected, preview.ConfirmToken)
	}
	if len(preview.Excluded) != 1 || preview.Excluded[0].Code == nil || *preview.Excluded[0].Code != "sole_project_company" {
		t.Fatalf("preview excludes %+v; want the second company refused as the project's last company", preview.Excluded)
	}

	industry := "Logistics"
	if _, err := store.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](ids.UUID(first.Id)),
		contacts.UpdateCompanyInput{Industry: &industry}); err != nil {
		t.Fatalf("editing the first company after the preview: %v", err)
	}
	change.confirmToken = *preview.ConfirmToken
	out, err := engine.Execute(e.Admin(), change)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	reasons := skipReasons(out.Skipped)
	if out.Changed != 0 || reasons[first.Id] != crmcontracts.BulkSkipReasonChangedSincePreview ||
		reasons[second.Id] != crmcontracts.BulkSkipReasonNotPreviewed {
		t.Fatalf("changed %d, skipped %v; want nothing changed, the first changed_since_preview and the second not_previewed",
			out.Changed, out.Skipped)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company WHERE id = $1 AND archived_at IS NULL`, second.Id); n != 1 {
		t.Error("the company the preview excluded was archived")
	}
}

// Two changes by one agent running at once, each of which alone fits the
// budget left and together do not: exactly one commits, and the counter
// ends at the limit rather than past it.
func TestTwoConcurrentChangesCannotBothSpendTheSameRemainder(t *testing.T) {
	e := integration.Setup(t)
	meter := agentvolume.New(redistest.Client(t), agentvolume.Limits{Writes: 30}, agentvolume.DefaultWindow)
	engine := newBulkEngine(e.DB(), auth.NewGate(nil, auth.WithVolumeMeter(meter)))
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if err := meter.Consume(agent, agentvolume.Writes, 20); err != nil {
		t.Fatalf("spending the window: %v", err)
	}
	batches := [][]crmcontracts.BulkItem{seedBulkContacts(t, e, e.Rep1, 10), seedBulkContacts(t, e, e.Rep1, 10)}

	errs := make([]error, len(batches))
	var wg sync.WaitGroup
	for i, items := range batches {
		wg.Go(func() {
			_, errs[i] = engine.Execute(agent, reassignTo(e.Rep2, items))
		})
	}
	wg.Wait()

	refused := 0
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.As(err, new(*auth.RecordWritesOverBudgetError)):
			refused++
		default:
			t.Fatalf("a change failed for another reason: %v", err)
		}
	}
	if refused != 1 {
		t.Fatalf("%d of the two changes were refused, want exactly one", refused)
	}
	if got := meter.Read(agent, agentvolume.Writes).Observed; got != 30 {
		t.Errorf("the window reads %d writes, want 30: twenty spent before plus the ten that committed", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE owner_id = $1`, e.Rep2); n != 10 {
		t.Errorf("%d contacts moved, want the ten of the one change that committed", n)
	}
}

// A confirmation nobody spent is deleted once it lapses, by the hourly
// transport retention pass, whether or not anyone previews again.
func TestTheRetentionPassDeletesLapsedConfirmationsAndKeepsLiveOnes(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	for range 2 {
		if _, err := engine.Preview(e.Admin(), reassignTo(e.Rep2, seedBulkContacts(t, e, e.Rep1, 1))); err != nil {
			t.Fatalf("Preview: %v", err)
		}
	}
	e.WsExec(t, `UPDATE bulk_confirmation SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 hour'
		WHERE token_hash = (SELECT token_hash FROM bulk_confirmation ORDER BY created_at LIMIT 1)`)

	sweeper := NewIdempotencyRetentionSweeper(e.Pool, slog.New(slog.DiscardHandler))
	if err := sweeper.SweepWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("SweepWorkspace: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM bulk_confirmation WHERE expires_at <= now()`); n != 0 {
		t.Errorf("%d lapsed confirmations survived the pass", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM bulk_confirmation WHERE expires_at > now()`); n != 1 {
		t.Errorf("%d live confirmations remain, want the one still inside its window", n)
	}
}

// A reservation for a change that then fails to commit is given back, so the
// agent is not charged for records that never changed.
func TestAChangeThatDoesNotCommitGivesItsReservationBack(t *testing.T) {
	e := integration.Setup(t)
	meter := agentvolume.New(redistest.Client(t), agentvolume.Limits{Writes: 30}, agentvolume.DefaultWindow)
	engine := newBulkEngine(e.DB(), auth.NewGate(nil, auth.WithVolumeMeter(meter)))
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	items := seedBulkContacts(t, e, e.Rep1, 3)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`ALTER TABLE bulk_operation ADD CONSTRAINT bulk_test_refuses_every_batch CHECK (changed_count < 0) NOT VALID`); err != nil {
		t.Fatalf("installing the refusal: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE bulk_operation DROP CONSTRAINT IF EXISTS bulk_test_refuses_every_batch`); err != nil {
			t.Errorf("removing the refusal: %v", err)
		}
	})

	if _, err := engine.Execute(agent, reassignTo(e.Rep2, items)); err == nil {
		t.Fatal("the change committed although its batch row was refused")
	}
	if got := meter.Read(agent, agentvolume.Writes).Observed; got != 0 {
		t.Errorf("the window reads %d writes after a change that did not commit, want 0", got)
	}
}
