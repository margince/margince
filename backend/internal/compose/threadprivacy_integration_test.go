// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type threadPrivacyFixture struct {
	*integration.Env
	Subject         ids.ContactID
	SurvivorSubject ids.ContactID
	Erased          ids.ActivityID
	Survivor        ids.ActivityID
	Root            string
	Reply           string
	Raw             []byte
	Digest          string
}

func newThreadPrivacyFixture(t *testing.T, shared bool) threadPrivacyFixture {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Pat Counterparty", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "pat@counterparty.example", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	sam, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Sam Correspondent", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "sam@other.example", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	f := threadPrivacyFixture{Env: e, Subject: ids.From[ids.ContactKind](ids.UUID(contact.Id)), SurvivorSubject: ids.From[ids.ContactKind](ids.UUID(sam.Id)), Root: "erased-topic@counterparty.example", Reply: "surviving-reply@other.example"}
	address := seatAddress(t, e, e.Rep1)
	if shared {
		e.WsExec(t, `UPDATE capture_connection SET mail_posture='shared' WHERE user_id=$1`, e.Rep1)
	}
	f.Raw = subjectThreadMail(address, f.Root, "Demo purchase planning", "")
	captureThreadMail(t, e, e.Rep1, f.Raw)
	reply := subjectThreadMail(address, f.Reply, "", f.Root)
	reply = []byte(strings.Replace(string(reply), "From: Pat Counterparty <pat@counterparty.example>", "From: Sam Correspondent <sam@other.example>", 1))
	captureThreadMail(t, e, e.Rep1, reply)
	finishThreadRepairs(t, e)
	f.Erased = capturedThreadActivity(t, e, f.Root)
	f.Survivor = capturedThreadActivity(t, e, f.Reply)
	digest := sha256.Sum256([]byte("demo purchase planning"))
	f.Digest = hex.EncodeToString(digest[:])
	if got := e.WsScalar(t, `SELECT own_topic FROM capture_thread_topic WHERE activity_id=$1`, f.Erased); got != f.Digest {
		t.Fatalf("topic is not its canonical digest: %s", got)
	}
	if got := threadKeyOfMessage(t, e, f.Reply); got != f.Root {
		t.Fatalf("blank reply root=%s", got)
	}
	return f
}

func stageThreadPrivacyRepair(t *testing.T, f threadPrivacyFixture, quarantine bool) {
	t.Helper()
	run := beginUnavailableThread(t, f.Env, f.Root)
	apply := threadRepairApplier(f.Pool)
	if quarantine {
		apply.Activity = func(context.Context, pgx.Tx, ids.ActivityID, string, string) (bool, error) {
			return false, errors.New("injected source repair fault")
		}
	}
	for range 2000 {
		ready := false
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, apply); err != nil {
				return err
			}
			if quarantine {
				if _, err := tx.Exec(f.Admin(), `UPDATE capture_thread_repair SET retry_after='2000-01-01' WHERE state='backoff'`); err != nil {
					return err
				}
				return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE state='quarantined')`).Scan(&ready)
			}
			return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE phase='inherit')`).Scan(&ready)
		}); err != nil {
			t.Fatal(err)
		}
		if ready {
			return
		}
	}
	t.Fatal("repair did not reach its erasure checkpoint")
}

func eraseThreadPrivacySource(t *testing.T, f threadPrivacyFixture) {
	t.Helper()
	if err := ComposedEraser(f.Pool, nil, nil).EraseContact(f.Admin(), f.Subject.UUID, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation WHERE activity_id=$1`, f.Erased); count != 1 {
		t.Fatalf("durable invalidations=%d", count)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE activity_id=$1`, f.Erased); count != 0 {
		t.Fatalf("erased comparison cache rows=%d", count)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND thread_input_hash<>''`, f.Erased); count != 0 {
		t.Fatalf("erased replay fingerprints=%d", count)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_reply WHERE activity_id=$1`, f.Erased); count != 0 {
		t.Fatalf("erased reply ledger rows=%d", count)
	}
	var available bool
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(f.Admin(), `SELECT activity_thread_available(@key)`, pgx.NamedArgs{"key": f.Root}).Scan(&available)
	}); err != nil {
		t.Fatal(err)
	}
	if available {
		t.Fatal("invalidated conversation became available before cleanup")
	}
}

func TestErasingAThreadSourceInvalidatesActiveAndQuarantinedPlansBeforeReplay(t *testing.T) {
	for _, quarantine := range []bool{false, true} {
		t.Run(fmt.Sprint(quarantine), func(t *testing.T) {
			f := newThreadPrivacyFixture(t, false)
			stageThreadPrivacyRepair(t, f, quarantine)
			eraseThreadPrivacySource(t, f)
			finishThreadRepairs(t, f.Env)
			if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
				t.Fatalf("erased ancestor still grouped the survivor under%s", got)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation`); count != 0 {
				t.Fatalf("cleanup inbox retains%d rows", count)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_repair_node WHERE own_topic=$1 OR ancestor_topic=$1 OR frontier_topic=$1`, f.Digest); count != 0 {
				t.Fatalf("erased staged digests=%d", count)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE own_topic=$1 OR effective_topic=$1`, f.Digest); count != 0 {
				t.Fatalf("erased topic digests=%d", count)
			}
			parsed, err := mailmap.Parse(f.Raw, seatAddress(t, f.Env, f.Rep1))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := newCaptureSink(f.Pool, CaptureConfig{}).Upsert(threadConnectorCtx(f.Env, f.Rep1), parsed.ToRecord("gmail", f.Raw)); err != nil && !errors.Is(err, connector.ErrSkip) {
				t.Fatal(err)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE activity_id=$1`, f.Erased); count != 0 {
				t.Fatalf("replay rebuilt%d erased caches", count)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_reply WHERE activity_id=$1`, f.Erased); count != 0 {
				t.Fatalf("replay rebuilt%d erased reply ledgers", count)
			}
			if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE own_topic=$1 OR effective_topic=$1`, f.Digest); count != 0 {
				t.Fatalf("replay restored %d erased subject comparisons", count)
			}
		})
	}
}

func TestASubjectExportKeepsAuthoredEvidenceDuringThreadRepair(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprint(shared), func(t *testing.T) {
			f := newThreadPrivacyFixture(t, shared)

			company := f.SeedCompany(t, "Demo project company", nil)
			ownerID := ids.From[ids.UserKind](f.Rep1)
			project, err := f.Projects.CreateProject(f.Admin(), projects.CreateProjectInput{Name: "Demo protected correspondence", CompanyID: ids.From[ids.CompanyKind](company), OwnerID: &ownerID, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.Activities.RelinkActivity(partySeatContext(t, f.Env, f.Rep1), f.Erased, activities.RelinkActivityInput{EntityType: "project", EntityID: ids.UUID(project.Id)}); err != nil {
				t.Fatal(err)
			}
			if shared {
				owner := partySeatContext(t, f.Env, f.Rep1)
				decision, err := NewThreadAudienceSetter(f.Pool).Decide(owner, f.Root, true)
				if err != nil || !decision.Shared {
					t.Fatalf("sharing source: %+v %v", decision, err)
				}
			}
			stageThreadPrivacyRepair(t, f, false)
			if !shared {
				// Legacy sender-only records still belong in a subject export during a processing hold.
				f.WsExec(t, `DELETE FROM activity_link WHERE activity_id=$1`, f.Erased)
				f.WsExec(t, `DELETE FROM activity_participant WHERE activity_id=$1 AND role='from'`, f.Erased)
			}
			f.WsExec(t, `UPDATE activity SET restricted_at=statement_timestamp(),restricted_reason='Processing restricted',restricted_until='2100-01-01',archived_at=statement_timestamp() WHERE id=$1`, f.Erased)
			exported, err := privacy.AssembleSAR(f.Admin(), InstallationDB(f.Pool), f.Subject)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range exported.ConversationDerivations {
				if fmt.Sprint(row["activity_id"]) != f.Erased.String() {
					continue
				}
				found = true
				if row["publication_pending"] != true {
					t.Fatal("fixture is not behind an active repair barrier")
				}
				if shared && row["topic_digest"] != f.Digest {
					t.Fatalf("shared held source derivation=%v; audience=%s", row, f.WsScalar(t, `SELECT audience FROM activity WHERE id=$1`, f.Erased))
				}
				if !shared && row["topic_digest"] != nil {
					t.Fatal("private source comparison was disclosed")
				}
			}
			if !found {
				t.Fatal("active barrier omitted the sender-only subject evidence")
			}
		})
	}
}

func TestASubjectExportReportsInvalidatedThreadCleanup(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	stageThreadPrivacyRepair(t, f, false)
	eraseThreadPrivacySource(t, f)
	exported, err := privacy.AssembleSAR(f.Admin(), InstallationDB(f.Pool), f.Subject)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range exported.ConversationDerivations {
		if fmt.Sprint(row["activity_id"]) != f.Erased.String() {
			continue
		}
		found = true
		if row["pending_cleanup"] != true {
			t.Fatalf("erasure cleanup status=%v", row)
		}
		if row["topic_digest"] != nil {
			t.Fatal("invalidated topic was exported as a published decision")
		}
	}
	if !found {
		t.Fatal("export omitted pending derived-state cleanup")
	}
	finishThreadRepairs(t, f.Env)
}

func TestPhysicalDeletionCannotDiscardPendingThreadErasure(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	backed := seedThreadPrivacyGround(t, f, &f.Erased, "Source-only identifying note")
	revoked := seedThreadPrivacyGround(t, f, &f.Erased, "Previously revoked identifying note")
	f.WsExec(t, `UPDATE communication_basis SET revoked_at='2026-01-02' WHERE id=$1`, revoked)
	stageThreadPrivacyRepair(t, f, false)
	eraseThreadPrivacySource(t, f)
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(f.Admin(), `DELETE FROM activity WHERE id=@id`, pgx.NamedArgs{"id": f.Erased})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation WHERE activity_id=$1`, f.Erased); count != 1 {
		t.Fatalf("physical deletion discarded %d durable invalidations", 1-count)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE id=$1 AND (revoked_at IS NULL OR source_activity_id IS NOT NULL OR thread_source_activity_id IS NOT NULL OR note IS NOT NULL)`, backed); got != 0 {
		t.Fatal("physical source deletion retained live or identifying ground")
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE id=$1 AND revoked_at='2026-01-02' AND note IS NULL AND source_activity_id IS NULL AND thread_source_activity_id IS NULL`, revoked); got != 1 {
		t.Fatal("physical deletion retained a previously revoked source note")
	}
	finishThreadRepairs(t, f.Env)
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
		t.Fatalf("deleted source still grouped survivor under %s", got)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_invalidation`); count != 0 {
		t.Fatalf("physical erasure left %d invalidations", count)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE own_topic=$1 OR effective_topic=$1`, f.Digest); count != 0 {
		t.Fatalf("physical erasure retained %d comparisons", count)
	}
}

func seedThreadPrivacyGround(t *testing.T, f threadPrivacyFixture, source *ids.ActivityID, note string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(f.Admin(), `INSERT INTO communication_basis(contact_id,kind,thread_key,source_activity_id,valid_from,note,captured_by)
 VALUES(@contact,'subject_initiated_correspondence',@key,@source,'2026-01-01',@note,'human:fixture') RETURNING id`, pgx.NamedArgs{"contact": f.SurvivorSubject, "key": f.Root, "source": source, "note": note}).Scan(&id)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestErasureRetractsCommittedInheritanceBeforeAnOldPlanCanPublish(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	subject := "Logo"
	if _, err := f.Activities.UpdateActivity(partySeatContext(t, f.Env, f.Rep1), f.Survivor, activities.UpdateActivityInput{Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	backed := seedThreadPrivacyGround(t, f, &f.Erased, "Source-only identifying note")
	independent := seedThreadPrivacyGround(t, f, nil, "Independent controller note")
	revoked := seedThreadPrivacyGround(t, f, &f.Erased, "Previously revoked identifying note")
	f.WsExec(t, `UPDATE communication_basis SET revoked_at='2026-01-02' WHERE id=$1`, revoked)
	run := beginUnavailableThread(t, f.Env, f.Root)
	reached := false
	for range 2000 {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, threadRepairApplier(f.Pool)); err != nil {
				return err
			}
			return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE phase='apply')`).Scan(&reached)
		}); err != nil {
			t.Fatal(err)
		}
		if reached {
			break
		}
	}
	if !reached {
		t.Fatal("repair did not complete inheritance")
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE thread_key=$1 AND thread_derived_from=ANY($2::uuid[]) AND revoked_at IS NULL`, f.Reply, []ids.UUID{backed, independent}); got != 2 {
		t.Fatalf("committed inherited grounds=%d", got)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE thread_derived_from=ANY($1::uuid[]) AND note IS NOT NULL`, []ids.UUID{backed, independent}); got != 0 {
		t.Fatalf("inherited freeform notes=%d", got)
	}
	eraseThreadPrivacySource(t, f)
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Root {
		t.Fatalf("old plan moved the survivor before invalidation drained: %s", got)
	}
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return consent.ThreadBasisCopyTx(f.Admin(), tx, backed, f.Root, "human-copy-after-erasure")
	}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("human proof-copy authority: %v", err)
	}
	system := principal.SystemActing(f.Admin(), "system:thread_repair")
	if err := database.WithWorkspaceTx(system, f.Pool, func(tx pgx.Tx) error {
		return consent.ThreadBasisCopyTx(system, tx, backed, f.Root, "new-copy-after-erasure")
	}); err != nil {
		t.Fatal(err)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE thread_key='new-copy-after-erasure'`); got != 0 {
		t.Fatal("erased source renewed its ground before invalidation drained")
	}
	finishThreadRepairs(t, f.Env)
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE (id=$1 OR thread_derived_from=$1) AND (revoked_at IS NULL OR source_activity_id IS NOT NULL OR thread_source_activity_id IS NOT NULL OR note IS NOT NULL)`, backed); got != 0 {
		t.Fatalf("erased inherited proof remains live or identifying: %d", got)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE (id=$1 OR thread_derived_from=$1) AND revoked_at IS NULL`, independent); got != 2 {
		t.Fatalf("independent grounds remaining=%d", got)
	}
	if note := f.WsScalar(t, `SELECT note FROM communication_basis WHERE id=$1`, independent); note != "Independent controller note" {
		t.Fatalf("independent note=%q", note)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM communication_basis WHERE id=$1 AND revoked_at='2026-01-02' AND note IS NULL AND source_activity_id IS NULL AND thread_source_activity_id IS NULL`, revoked); got != 1 {
		t.Fatal("previously revoked source note survived or its revocation changed")
	}
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
		t.Fatalf("survivor root=%s", got)
	}
}

func TestAStoredRedactionCannotBecomeThreadEvidenceWhenItsArchiveFlagChanges(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	stageThreadPrivacyRepair(t, f, false)
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(f.Admin(), `UPDATE activity SET subject=NULL,body=NULL,redacted_fields=ARRAY['subject','body']::text[],archived_at=NULL WHERE id=@id`, pgx.NamedArgs{"id": f.Erased}); err != nil {
			return err
		}
		return capture.NewPendingStore(InstallationDB(f.Pool)).PurgeActivityProjectionsTx(f.Admin(), tx, []ids.UUID{f.Erased.UUID})
	}); err != nil {
		t.Fatal(err)
	}
	finishThreadRepairs(t, f.Env)
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
		t.Fatalf("redacted source still supplied a thread ancestor: %s", got)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_topic WHERE activity_id=$1`, f.Erased); count != 0 {
		t.Fatalf("redacted input rebuilt %d topic caches", count)
	}
}

func TestErasureCanInvalidatePublishedWorkWhileAnotherConversationIsActive(t *testing.T) {
	f := newThreadPrivacyFixture(t, false)
	run := beginUnavailableThread(t, f.Env, f.Root)
	published := false
	for range 2000 {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, run, threadRepairApplier(f.Pool)); err != nil {
				return err
			}
			return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE phase='published')`).Scan(&published)
		}); err != nil {
			t.Fatal(err)
		}
		if published {
			break
		}
	}
	if !published {
		t.Fatal("first repair did not reach publication")
	}
	const other = "unrelated-active@other.example"
	raw := subjectThreadMail(seatAddress(t, f.Env, f.Rep1), other, "Another discussion", "")
	raw = []byte(strings.Replace(string(raw), "From: Pat Counterparty <pat@counterparty.example>", "From: Sam Correspondent <sam@other.example>", 1))
	captureThreadMail(t, f.Env, f.Rep1, raw)
	active := false
	for range 1000 {
		if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
			if err := capture.QueueThreadRepairTx(f.Admin(), tx, other); err != nil {
				return err
			}
			if _, _, _, err := capture.ThreadRepairBatchTx(f.Admin(), tx, 1, ids.NewV7(), threadRepairApplier(f.Pool), other); err != nil {
				return err
			}
			return tx.QueryRow(f.Admin(), `SELECT EXISTS(SELECT 1 FROM capture_thread_repair r JOIN capture_thread_barrier b ON b.repair_id=r.id WHERE r.phase<>'published' AND r.state='ready' AND b.blocked_key=@key)`, pgx.NamedArgs{"key": other}).Scan(&active)
		}); err != nil {
			t.Fatal(err)
		}
		if active {
			break
		}
	}
	if !active {
		t.Fatal("second conversation never became active")
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_repair r JOIN capture_thread_barrier b ON b.repair_id=r.id WHERE r.phase='published' AND b.blocked_key=$1`, f.Root); count == 0 {
		t.Fatalf("published components=%d", count)
	}
	if count := f.WsCount(t, `SELECT count(*) FROM capture_thread_repair WHERE phase<>'published' AND state='ready'`); count != 1 {
		t.Fatalf("active components=%d", count)
	}
	eraseThreadPrivacySource(t, f)
	finishThreadRepairs(t, f.Env)
	if got := threadKeyOfMessage(t, f.Env, f.Reply); got != f.Reply {
		t.Fatalf("erased ancestor remained %s", got)
	}
	if got := threadKeyOfMessage(t, f.Env, other); got != other {
		t.Fatalf("unrelated conversation moved to %s", got)
	}
}
