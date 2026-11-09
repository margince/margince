// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func captureProviderMail(t *testing.T, e *integration.Env, from, subject string, user ids.UUID, otherSeats ...ids.UUID) ids.UUID {
	t.Helper()
	rec := providerMailRecord(t, e, from, subject, user, otherSeats...)
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(threadConnectorCtx(e, user), rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, otherSeat := range otherSeats {
		other := rec
		other.ProviderMessageID = "other-" + rec.ProviderMessageID
		other.DeliveredTo = seatAddress(t, e, otherSeat)
		if _, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(threadConnectorCtx(e, otherSeat), other); err != nil {
			t.Fatal(err)
		}
	}
	return ref.ID
}

func providerMailRecord(t *testing.T, e *integration.Env, from, subject string, user ids.UUID, otherSeats ...ids.UUID) connector.NormalizedRecord {
	t.Helper()
	mailbox := seatAddress(t, e, user)
	recipients := []string{mailbox}
	for _, seat := range otherSeats {
		recipients = append(recipients, seatAddress(t, e, seat))
	}
	providerID := "provider-" + ids.NewV7().String()
	raw := strings.Join([]string{
		"From: Sender <" + from + ">", "To: " + strings.Join(recipients, ", "), "Subject: " + subject,
		"Date: Mon, 01 Jun 2026 08:00:00 +0000", "Message-ID: <" + providerID + "@capture.test>",
		"Content-Type: text/plain; charset=utf-8", "", "the message body", "",
	}, "\r\n")
	parsed, err := mailmap.Parse([]byte(raw), mailbox)
	if err != nil {
		t.Fatal(err)
	}
	rec := parsed.ToRecord("gmail", []byte(raw))
	rec.ProviderMessageID = providerID
	return rec
}

func TestProviderRemovalsRequireCurrentMappingsAndKeepOtherCopies(t *testing.T) {
	for _, variant := range []string{"legacy", "replay fills legacy", "rebound account", "reconnected account", "another provider copy"} {
		t.Run(variant, func(t *testing.T) {
			e := integration.Setup(t)
			rec := providerMailRecord(t, e, "friend@example.test", "Private note", e.Rep1)
			sink := newCaptureSink(e.Pool, CaptureConfig{}).WithMessagePurger(purgerFor(t, e).PurgeRemovedAdmitted)
			ctx := threadConnectorCtx(e, e.Rep1)
			ref, err := sink.Upsert(ctx, rec)
			if err != nil {
				t.Fatal(err)
			}
			switch variant {
			case "legacy", "replay fills legacy":
				e.WsExec(t, `UPDATE capture_import SET provider_message_ids = '{}' WHERE activity_id = $1`, ref.ID)
				if variant == "replay fills legacy" {
					if _, err := sink.Upsert(ctx, rec); err != nil {
						t.Fatal(err)
					}
				}
			case "rebound account":
				e.WsExec(t, `UPDATE capture_connection SET generation = generation + 1 WHERE user_id = $1 AND provider = 'gmail'`, e.Rep1)
			case "reconnected account":
				e.WsExec(t, `UPDATE capture_connection SET generation = generation + 1 WHERE user_id = $1 AND provider = 'gmail'`, e.Rep1)
				if _, err := sink.Upsert(ctx, rec); err != nil {
					t.Fatal(err)
				}
			case "another provider copy":
				other := rec
				other.ProviderMessageID = "other-id"
				if _, err := sink.Upsert(ctx, other); err != nil {
					t.Fatal(err)
				}
			}
			if err := sink.RemoveMessage(ctx, connector.NaturalKey{SourceSystem: "gmail", SourceID: rec.ProviderMessageID}); err != nil {
				t.Fatal(err)
			}
			if gone := activityBody(t, e, ref.ID) == ""; gone != (variant == "replay fills legacy" || variant == "reconnected account") {
				t.Errorf("message destroyed=%v for %s", gone, variant)
			}
			if variant == "another provider copy" {
				if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1 AND cardinality(provider_message_ids) = 1 AND provider_message_ids[1] LIKE '%:other-id'`, ref.ID); got != 1 {
					t.Error("removal did not consume just the deleted provider alias")
				}
			}
		})
	}
}

func TestMailboxDeletionRequiresItsAdmittedSeatAndProvider(t *testing.T) {
	e := integration.Setup(t)
	rec := providerMailRecord(t, e, "friend@example.test", "Private note", e.Rep1)
	ctx := threadConnectorCtx(e, e.Rep1)
	ref, err := newCaptureSink(e.Pool, CaptureConfig{}).Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"no admission", "different seat", "different provider", "admission refused"} {
		t.Run(variant, func(t *testing.T) {
			seat, provider := e.Rep1, "gmail"
			admit := func(context.Context, pgx.Tx) error { return nil }
			switch variant {
			case "no admission":
				admit = nil
			case "different seat":
				seat = e.Rep2
			case "different provider":
				provider = "graph"
			case "admission refused":
				admit = func(context.Context, pgx.Tx) error { return apperrors.ErrPermissionDenied }
			}
			err := purgerFor(t, e).PurgeRemovedAdmitted(ctx, admit, seat, provider, rec.ProviderMessageID)
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("unbound removal accepted: %v", err)
			}
			if activityBody(t, e, ref.ID) == "" {
				t.Fatal("refused deletion destroyed mail")
			}
		})
	}
}

func TestMailboxPurgeRechecksAColleagueClaimInsideTheEraseTransaction(t *testing.T) {
	e := integration.Setup(t)
	rec := providerMailRecord(t, e, "friend@example.test", "Private note", e.Rep1, e.Rep2)
	sourceCtx := threadConnectorCtx(e, e.Rep1)
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	ref, err := sink.Upsert(sourceCtx, rec)
	if err != nil {
		t.Fatal(err)
	}
	var selected capture.PurgeSubject
	err = database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		floor, _, err := purgeFloorTx(e.Admin(), tx)
		if err != nil {
			return err
		}
		selected, err = capture.SelectRemovedPurgeTx(e.Admin(), tx, e.Rep1, "gmail", rec.ProviderMessageID, floor)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.SoleImports) != 1 {
		t.Fatal("initial selection did not find the sole copy")
	}
	other := rec
	other.DeliveredTo = seatAddress(t, e, e.Rep2)
	if _, err := sink.Upsert(threadConnectorCtx(e, e.Rep2), other); err != nil {
		t.Fatal(err)
	}
	purger := purgerFor(t, e)
	check := purger.removedStillPurgeable(sourceCtx, nil, e.Rep1, "gmail", rec.ProviderMessageID)
	count, err := purger.retention.PurgeActivitiesIf(principal.SystemActing(e.Admin(), "system:capture"), selected.SoleImports, privacy.PurgeMailboxDeletion, check)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || activityBody(t, e, ref.ID) == "" {
		t.Fatal("the stale selection erased a colleague's copy")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1`, ref.ID); got != 2 {
		t.Fatalf("kept %d claims, want 2", got)
	}
}

func TestMailboxDeletionPreservesMailWrittenByTheCRM(t *testing.T) {
	e := integration.Setup(t)
	address := seatAddress(t, e, e.Rep1)
	root := "crm-mail-root@counterparty.test"
	captureThreadMail(t, e, e.Rep1, threadMail(address, root, "Tue, 01 Sep 2026 10:00:00 +0000", ""))
	anchor, err := ids.Parse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, root))
	if err != nil {
		t.Fatal(err)
	}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: &recordingStager{}})
	sent, err := adapter.SendEmail(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), anchor, agents.SendEmailArgs{To: []string{"pat@counterparty.example"}, Subject: "Re: Angebot", Body: "I will send the draft.", ConsentPurpose: "transactional"})
	if err != nil {
		t.Fatal(err)
	}
	messageID := e.WsScalar(t, `SELECT source_id FROM activity WHERE id=$1`, sent.ActivityID)
	raw := strings.Join([]string{"From: " + address, "To: pat@counterparty.example", "Subject: Re: Angebot", "Date: Sun, 18 Oct 2026 10:00:00 +0000", "Message-ID: <" + messageID + ">", "References: <" + root + ">", "Content-Type: text/plain; charset=utf-8", "", "I will send the draft.", ""}, "\r\n")
	mail, err := mailmap.Parse([]byte(raw), address)
	if err != nil {
		t.Fatal(err)
	}
	rec := mail.AttestSentByOwner(true).ToRecord("gmail", []byte(raw))
	rec.ProviderMessageID = "crm-sent-provider-id"
	ctx := threadConnectorCtx(e, e.Rep1)
	sink := newCaptureSink(e.Pool, CaptureConfig{}).WithMessagePurger(purgerFor(t, e).PurgeRemovedAdmitted)
	ref, err := sink.Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID != sent.ActivityID {
		t.Fatal("provider echo did not map the CRM send")
	}
	if count := e.WsCount(t, `SELECT count(*) FROM capture_import i JOIN capture_connection c ON c.user_id=i.user_id WHERE i.activity_id=$1 AND c.provider='gmail' AND i.provider_message_ids @> ARRAY[c.provider || ':' || c.generation || ':crm-sent-provider-id']`, sent.ActivityID); count != 1 {
		t.Fatal("provider echo did not record its mailbox mapping")
	}

	if err := sink.RemoveMessage(ctx, connector.NaturalKey{SourceSystem: "gmail", SourceID: rec.ProviderMessageID}); err != nil {
		t.Fatal(err)
	}
	if activityBody(t, e, sent.ActivityID) == "" {
		t.Fatal("mailbox cleanup erased a CRM-authored send")
	}
}
