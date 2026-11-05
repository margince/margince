// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func stagePartyReply(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	e.GrantRole(t, e.Rep1, "admin")
	activatePartySeat(t, e, e.Rep1)
	anchor, recipient := seedTransactionalReply(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	delivery := realDeliveryStager(t, e)
	path := SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: delivery}
	adapter := newCommsAdapter(e.Pool, nil, path)
	consents := consent.NewStore(e.DB())
	var purpose ids.PurposeID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id FROM consent_purpose WHERE key='transactional' AND archived_at IS NULL`).Scan(&purpose)
	}); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"bob@buyer.test", "casey@buyer.test"} {
		lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Email: &address, FullName: new(address), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := consents.Record(e.Admin(), consent.RecordInput{LeadID: ids.From[ids.LeadKind](ids.UUID(lead.Id)), PurposeID: purpose, NewState: "granted", PolicyText: &grantedWording}); err != nil {
			t.Fatal(err)
		}
	}
	sent, err := adapter.SendEmail(ctx, anchor, agents.SendEmailArgs{
		To: []string{recipient}, Cc: []string{"bob@buyer.test", "casey@buyer.test"},
		Subject: "Product requirements", Body: "Could we discuss the product requirements?", ConsentPurpose: "transactional",
	})
	if err != nil {
		t.Fatal(err)
	}
	return sent.ActivityID
}

func TestCapturedPartiesWaitsForTheGovernedSendReceiptAndRecoversDerivation(t *testing.T) {
	e := integration.Setup(t)
	sent := stagePartyReply(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	var delivery ids.UUID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT id FROM comms_outbound WHERE activity_id=$1`, sent).Scan(&delivery)
	}); err != nil {
		t.Fatal(err)
	}
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	system := principal.SystemActing(e.Admin(), "party-recovery-test")
	if err := sink.RecoverPartyAdmissions(system, 200); err != nil {
		t.Fatal(err)
	}
	assertCopiedContacts := func(want int) {
		t.Helper()
		if got := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email) IN ('bob@buyer.test','casey@buyer.test') AND archived_at IS NULL`); got != want {
			t.Fatalf("copied contacts=%d, want %d", got, want)
		}
	}
	assertCopiedContacts(0)
	store := comms.NewStore(e.DB(), func() time.Time { return clocktest.Now(t) }, activities.NewStore(e.DB()), comms.WithPartyAdmission(func(context.Context, ids.ActivityID) error {
		return errors.New("derivation unavailable")
	}))
	if err := store.RecordSent(ctx, delivery, connector.SendReceipt{ProviderMessageID: "accepted"}); err != nil {
		t.Fatalf("derivation fault changed a committed receipt: %v", err)
	}
	assertCopiedContacts(0)
	if e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE id=$1 AND status='sent' AND party_admission_pending`, delivery) != 1 {
		t.Fatal("a derivation fault retired the receipt's pending work")
	}
	if err := sink.RecoverPartyAdmissions(system, 200); err != nil {
		t.Fatal(err)
	}
	assertCopiedContacts(0)
	if e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE id=$1 AND party_admission_pending`, delivery) != 0 {
		t.Fatal("successful recovery did not settle the accepted receipt")
	}
	if e.WsCount(t, `SELECT count(*) FROM lead WHERE lower(email) IN ('bob@buyer.test','casey@buyer.test') AND archived_at IS NULL`) != 2 {
		t.Fatal("sending to existing leads converted them")
	}
	if err := store.RecordSent(ctx, delivery, connector.SendReceipt{ProviderMessageID: "duplicate"}); !errors.Is(err, comms.ErrTerminal) {
		t.Fatalf("a duplicate receipt could retransmit: %v", err)
	}
	if e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE activity_id=$1 AND status='sent' AND provider_message_id='accepted'`, sent) != 1 {
		t.Fatal("recovery replaced or duplicated the accepted delivery")
	}
}
