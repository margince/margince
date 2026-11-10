// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

type recoveryRecipients struct{ selfFirst, copyColleague bool }

func setupRecoveryRecipients(t *testing.T, recipients recoveryRecipients) (recoveryEnv, string) {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.inWorkspace(t, `UPDATE capture_connection SET account_label=@mailbox WHERE user_id=@user`, pgx.StrictNamedArgs{"user": p.user, "mailbox": sendingMailbox})
	p.stakeADeal(t)
	to := []string{"buyer@preflight.test"}
	if recipients.selfFirst {
		establishRecoveryRecipient(t, p, sendingMailbox)
		to = append([]string{sendingMailbox}, to...)
	}
	cc := []string{}
	colleague := ""
	if recipients.copyColleague {
		p.DescribeCompany(t)
		colleague = createdID(t, p.AppEnv, "/v1/users", AnyMap{"email": "copy@recovery.test", "display_name": "Copy recipient", "role": "rep"})
		p.inWorkspace(t, `INSERT INTO capture_connection (provider,user_id,scopes,status,auth,provider_scopes,account_label)
 VALUES ('gmail',@user,'{}','connected','{"refresh_token":"r","granted":[]}',@scopes,'copy@recovery.test')`, pgx.StrictNamedArgs{"user": colleague, "scopes": []string{apptest.GmailReadonlyScope}})
		establishRecoveryRecipient(t, p, "copy@recovery.test")
		cc = append(cc, "copy@recovery.test")
	}
	var sent struct {
		ID     string `json:"id"`
		Detail string `json:"detail"`
	}
	if status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "As discussed.", "to": to, "cc": cc, "consent_purpose": "transactional",
	}, nil, &sent); status != http.StatusAccepted {
		t.Fatalf("send-email: %d %s", status, sent.Detail)
	}
	activity := uuidOf(t, sent.ID)
	delivery, minted := p.deliveryFor(t, activity)
	return recoveryEnv{preflightEnv: p, activity: activity, delivery: delivery, minted: minted}, colleague
}

func TestExactProviderEvidenceWinsOverTheFirstToAddress(t *testing.T) {
	r, _ := setupRecoveryRecipients(t, recoveryRecipients{selfFirst: true})
	r.captureOwnEcho(r.ownMailbox(t), t, echoAlreadyCaptured("Re: Inbound question", gmailStamped))
	gmail := r.sendRefusingReadBack(t, http.StatusServiceUnavailable, "backendError")
	r.convergedOnTheSend(t)
	if gmail.sends.Load() != 1 {
		t.Fatal("message sent more than once")
	}
}

func TestTheSendAbsorbsTheColleaguesCopyWhenItsOwnEchoArrives(t *testing.T) {
	r, colleague := setupRecoveryRecipients(t, recoveryRecipients{copyColleague: true})
	gmail := r.sendRefusingReadBack(t, http.StatusServiceUnavailable, "backendError")
	stored := gmail.transmitted(t)
	r.captureColleagueCopy(t, colleague, stored)
	r.captureOwnEcho(r.ownMailbox(t), t, stored)
	r.convergedOnTheSend(t)
	if gmail.sends.Load() != 1 {
		t.Fatal("message sent more than once")
	}
}

func TestAReceiptAndCaptureWaitingOnTheSameProviderKeyConverge(t *testing.T) {
	r, _ := setupRecoveryRecipients(t, recoveryRecipients{selfFirst: true})
	observer := OwnerConn(t)
	holder, err := pgx.ConnectConfig(context.Background(), observer.Config().Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := holder.Close(context.Background()); err != nil {
			t.Errorf("closing holder: %v", err)
		}
	}()
	tx, err := holder.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err != pgx.ErrTxClosed {
			t.Errorf("rollback: %v", err)
		}
	}()
	if err := storekit.LockSentEchoKey(context.Background(), tx, uuidOf(t, r.user), "gmail", gmailSentID); err != nil {
		t.Fatal(err)
	}
	gmail := &refusingReadBackGmail{}
	stub := gmail.serve(t, http.StatusServiceUnavailable, "backendError")
	mailbox := r.ownMailbox(t)
	stored := echoAlreadyCaptured("Re: Inbound question", gmailStamped)
	var racers sync.WaitGroup
	racers.Add(2)
	returned := make(chan struct{})
	go func() {
		defer racers.Done()
		if outcome, _ := r.dispatchThrough(t, r.delivery, stub, nil); outcome != comms.OutcomeSent {
			t.Errorf("dispatch outcome: %v", outcome)
		}
	}()
	go func() { defer racers.Done(); r.captureOwnEcho(mailbox, t, stored) }()
	go func() { racers.Wait(); close(returned) }()
	testdb.AwaitQueuedBehind(t, observer, int32(holder.PgConn().PID()), 2, returned, "receipt and capture")
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-returned
	r.convergedOnTheSend(t)
	if gmail.sends.Load() != 1 {
		t.Fatal("racing recovery resent the message")
	}
}

func establishRecoveryRecipient(t *testing.T, p *preflightEnv, address string) {
	t.Helper()
	contact := createdID(t, p.AppEnv, "/v1/contacts", AnyMap{"full_name": "Recovery recipient", "source": "manual", "emails": []AnyMap{{"email": address}}})
	if status := p.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "deal_stakeholder", "deal_id": p.dealID, "contact_id": contact, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("recipient stakeholder: %d", status)
	}
	apptest.AskedAboutTheDeal(t, p.AppEnv, contact, address, p.dealID)
}

func (r recoveryEnv) captureColleagueCopy(t *testing.T, colleague string, stored []byte) {
	t.Helper()
	parsed, err := mailmap.Parse(stored, "copy@recovery.test")
	if err != nil {
		t.Fatal(err)
	}
	record := parsed.ToRecord("gmail", stored)
	record.ProviderMessageID = "colleague-copy"
	if _, err := compose.NewCaptureSink(r.Pool, compose.CaptureConfig{}).Upsert(seatMailboxCtx(t, r.workspaceID(t), uuidOf(t, colleague)), record); err != nil {
		t.Fatal(err)
	}
	if r.activityOnTheStampedIdentity(t) == r.activity {
		t.Fatal("colleague fixture did not create a separate copy")
	}
}

func TestAPartialProviderCopyCannotAbsorbTheColleaguesInboundCopy(t *testing.T) {
	r, colleague := setupRecoveryRecipients(t, recoveryRecipients{copyColleague: true})
	gmail := r.sendRefusingReadBack(t, http.StatusServiceUnavailable, "backendError")
	stored := gmail.transmitted(t)
	r.captureColleagueCopy(t, colleague, stored)
	parsed, err := mailmap.Parse(stored, sendingMailbox)
	if err != nil {
		t.Fatal(err)
	}
	record := parsed.AttestSentByOwner(true).ToRecord("gmail", stored)
	fields, ok := record.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("mail parse did not yield activity fields")
	}
	fields.BodyCompleteness = capture.BodyPartial
	record.Fields = fields
	record.ProviderMessageID = gmailSentID
	if _, err := compose.NewCaptureSink(r.Pool, compose.CaptureConfig{}).Upsert(r.ownMailbox(t), record); err != nil {
		t.Fatal(err)
	}
	r.sendIsUntouched(t)
}
