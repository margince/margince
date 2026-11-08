// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const recruitingRelay = "group-11111111-2222-4333-8444-555555555555@hire.lever.co"

func TestRecruitingRelayMailKeepsItsThreadParticipantsAndFiles(t *testing.T) {
	e := integration.Setup(t)
	mailbox := seatAddress(t, e, e.Rep1)
	sent := strings.ReplaceAll(string(threadMail(recruitingRelay, "relay-sent@example.test", "Mon, 06 Jul 2026 12:00:00 +0000", "")),
		"Pat Counterparty <pat@counterparty.example>", "Rep <"+mailbox+">")
	captureThreadMail(t, e, e.Rep1, []byte(sent))
	raw := strings.ReplaceAll(string(threadMail(mailbox, "relay-reply@example.test", "Tue, 07 Jul 2026 12:00:00 +0000", "relay-sent@example.test", "relay-sent@example.test")),
		"pat@counterparty.example", recruitingRelay)
	raw = strings.Replace(raw, "Subject:", "Cc: Alice <alice@lever.co>\r\nSubject:", 1)
	parsed, err := mailmap.Parse([]byte(raw), mailbox)
	if err != nil {
		t.Fatal(err)
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithFileKeeper(fileKeeper(e.Pool, blobstore.NewMemory()))
	if _, err := sink.Upsert(threadConnectorCtx(e, e.Rep1), withFiles(parsed.ToRecord("gmail", []byte(raw)), onePDF())); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE email = $1`, recruitingRelay); got != 0 {
		t.Fatalf("relay created %d contacts after correspondence", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id IN ('relay-sent@example.test', 'relay-reply@example.test') AND archived_at IS NULL`); got != 2 {
		t.Errorf("kept %d messages, want 2", got)
	}
	if threadKeyOfMessage(t, e, "relay-sent@example.test") != threadKeyOfMessage(t, e, "relay-reply@example.test") {
		t.Error("relay reply lost its thread")
	}
	if got := filesFor(threadConnectorCtx(e, e.Rep1), t, e.DB(), "relay-reply@example.test"); len(got) != 1 {
		t.Errorf("stored %d attachments, want 1", len(got))
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE address = $1`, "alice@lever.co"); got == 0 {
		t.Error("legitimate CC participant was lost")
	}
}

func TestRelayRepairRevisitsFinishedTasksAndPreservesHumanRecords(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Recruiting Provider", Source: "manual",
		Domains: []contacts.CompanyDomainInput{{Domain: "hire.lever.co", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	finished := machineAddressDataTask()
	finished.Version = 2
	finished.Steps = finished.Steps[:1]
	runMachineAddressTask(ctx, t, e, finished)
	relay, inferred := capturedEmployee(t, e, "legacy@hire.lever.co")
	human, humanEdge := capturedEmployee(t, e, "edited@hire.lever.co")
	if _, err := e.Contacts.UpdateRelationship(e.Admin(), humanEdge, contacts.UpdateRelationshipInput{Role: new("Recruiter")}); err != nil {
		t.Fatal(err)
	}
	conn := integration.OwnerConn(t)
	for i, contact := range []ids.ContactID{relay, human} {
		address := recruitingRelay
		if i == 1 {
			address = strings.Replace(recruitingRelay, "11111111", "aaaaaaaa", 1)
		}
		if _, err := conn.Exec(ctx, `UPDATE contact_email SET email = $2 WHERE contact_id = $1`, contact, address); err != nil {
			t.Fatal(err)
		}
	}
	task := machineAddressDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit = 1
	}
	rollback := errors.New("roll back employment repair")
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := task.Steps[1].Batch(ctx, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) || !edgeLive(t, e, inferred) {
		t.Fatal("employment repair escaped rollback", err)
	}
	runMachineAddressTask(ctx, t, e, task)
	if edgeLive(t, e, inferred) {
		t.Error("relay kept its inferred provider employment")
	}
	if !edgeLive(t, e, humanEdge) {
		t.Error("human-edited employment was retired")
	}
	for _, contact := range []ids.ContactID{relay, human} {
		if contactArchived(t, e, contact.UUID) || machineTagged(t, e, contact.UUID) != 1 {
			t.Error("relay contact must stay available and be tagged for review")
		}
	}
	runMachineAddressTask(ctx, t, e, technicalAddressDataTask(e.Pool))
	for _, contact := range []ids.ContactID{relay, human} {
		if contactArchived(t, e, contact.UUID) {
			t.Fatal("technical repair archived a relay awaiting review")
		}
	}
	tags := collections.NewStore(e.DB())
	tagUUID, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM tag WHERE name='machine-address'`))
	if err != nil {
		t.Fatal(err)
	}
	tagID := ids.From[ids.TagKind](tagUUID)
	if _, err := tags.RemoveTag(e.Admin(), tagID, "contact", human.UUID); err != nil {
		t.Fatal(err)
	}
	replay := machineAddressDataTask()
	replay.Version++
	runMachineAddressTask(ctx, t, e, replay)
	if machineTagged(t, e, human.UUID) != 0 {
		t.Fatal("repair reversed a human tag removal")
	}
	if _, err := tags.ArchiveTag(e.Admin(), tagID); err != nil {
		t.Fatal(err)
	}
	replay.Version++
	runMachineAddressTask(ctx, t, e, replay)
	if got := e.WsCount(t, `SELECT count(*) FROM tag WHERE id=$1 AND archived_at IS NULL`, tagID); got != 0 {
		t.Fatal("repair restored an archived review tag")
	}
	runMachineAddressTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship' AND entity_id = $1 AND action = 'archive'`, inferred); got != 1 {
		t.Errorf("repair wrote %d archive audits, want 1", got)
	}
}

func TestRelaySenderRepairRevisitsTheFinishedClassifier(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	finished := machineSenderDataTask()
	finished.Version = 2
	runMachineAddressTask(ctx, t, e, finished)
	mailbox := seatAddress(t, e, e.Rep1)
	raw := strings.ReplaceAll(string(threadMail(mailbox, "relay-legacy@example.test", "Tue, 07 Jul 2026 12:00:00 +0000", "")),
		"pat@counterparty.example", recruitingRelay)
	captureThreadMail(t, e, e.Rep1, []byte(raw))
	e.WsExec(t, `UPDATE activity SET sender_is_machine = false WHERE source_id = 'relay-legacy@example.test'`)
	runMachineAddressTask(ctx, t, e, machineSenderDataTask())
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = 'relay-legacy@example.test' AND sender_is_machine AND archived_at IS NULL`); got != 1 {
		t.Error("legacy relay message must be marked and remain visible")
	}
}
