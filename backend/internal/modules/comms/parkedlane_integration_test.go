// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// The undelivered-lane read against rows the real writers produced: staged
// with StageTx, abandoned with Park, transmitted-then-parked with
// ParkTransmitted — never hand-inserted. The predicates under test are SQL
// (the stamp, the window, ownership, the contact join), which a unit double
// proves nothing about.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestParkedSendsForCarriesTheCallersAbandonedSendsOnly(t *testing.T) {
	e := setupStore(t)

	contact := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Anna Weber', 'test', 'human:x')`, contact); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`,
		ids.NewV7(), e.activity, contact); err != nil {
		t.Fatal(err)
	}

	// The send that was given up on.
	abandoned := e.stage(t, e.baseInput(e.activity, "abandoned@myco.test"))
	if err := e.store.Park(e.ctx, abandoned, "the mailbox is no longer send-capable"); err != nil {
		t.Fatalf("parking the delivery: %v", err)
	}

	// A send whose MESSAGE went out and whose receipt could not be written
	// wears the same status and must never reach the lane: the recipient has
	// it, and telling the sender it was never sent would be a lie that
	// invites a second copy.
	transmitted := e.stage(t, e.baseInput(e.activity, "transmitted@myco.test"))
	if err := e.store.ParkTransmitted(e.ctx, transmitted, "receipt lost", "prov-transmitted"); err != nil {
		t.Fatalf("parking the transmitted delivery: %v", err)
	}

	// A send that actually left is not on it either.
	if err := e.store.RecordSent(e.ctx, e.stage(t, e.baseInput(e.activity, "sent@myco.test")),
		connector.SendReceipt{ProviderMessageID: "prov-sent"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}

	// A CHANNEL send given up on wears the same stamp and reaches this lane
	// too. It has no subject line, so the card is headed by the channel and
	// the counterparty instead.
	channel := e.stageChannel(t, StageChannelInput{
		ActivityID: e.telegramActivity(t),
		Provider:   "telegram",
		Recipient: connector.ChannelIdentity{
			Provider: "telegram", ChannelUserID: "778899", Username: "buyer",
		},
		Body:           "On its way today.",
		ConsentPurpose: "transactional",
	})
	if err := e.store.Park(e.ctx, channel, "the bot was removed from the chat"); err != nil {
		t.Fatalf("parking the channel delivery: %v", err)
	}
	// The counterparty as Telegram knows them. The card's recipient half is
	// read from here. Never from the CRM record's own name, which a reader of
	// this lane may not be allowed to see.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES ($1, 'telegram', '778899', 'buyer', 'manual', 'human:x')`, contact); err != nil {
		t.Fatalf("seeding the channel identity: %v", err)
	}

	since := e.clockValue.Add(-7 * 24 * time.Hour)
	reader := readerCtx(e.ws, e.user)
	parked, err := e.store.ParkedSendsFor(reader, since, 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor: %v", err)
	}
	if len(parked) != 2 {
		t.Fatalf("ParkedSendsFor = %+v, want both sends that were given up on, mail and channel", parked)
	}
	byID := map[ids.UUID]ParkedSend{}
	for _, send := range parked {
		byID[send.ID] = send
	}
	channelCard, onLane := byID[channel]
	if !onLane {
		t.Fatalf("the parked channel send is not on the lane: %+v", parked)
	}
	// Headed by the channel and the counterparty the provider knows, and by
	// nothing out of the message. The text is unsent.
	if channelCard.Subject != "Telegram to buyer" {
		t.Errorf("channel card title = %q, want %q", channelCard.Subject, "Telegram to buyer")
	}
	if strings.Contains(channelCard.Subject, "On its way today") {
		t.Error("the unsent message text reached a shared queue through the card title")
	}
	if channelCard.Reason != "the bot was removed from the chat" {
		t.Errorf("channel reason = %q, want the dispatcher's own words", channelCard.Reason)
	}
	got, onLane := byID[abandoned]
	if !onLane {
		t.Fatalf("the parked mail send is not on the lane: %+v", parked)
	}
	if got.Reason != "the mailbox is no longer send-capable" {
		t.Errorf("reason = %q, want the dispatcher's own words", got.Reason)
	}
	if got.ContactID != contact {
		t.Errorf("contact = %s, want the one the activity is filed under (%s)", got.ContactID, contact)
	}
	if got.Subject != "Re: pricing" {
		t.Errorf("mail card title = %q, want the send's own subject line unchanged", got.Subject)
	}
	if got.ParkedAt.IsZero() {
		t.Error("the send carries no time, so the lane has nothing to window or order on")
	}

	// A window opening after the park drops it: the lane ages out.
	none, err := e.store.ParkedSendsFor(reader, e.clockValue.Add(time.Hour), 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor with a later window: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("aged window = %+v, want empty", none)
	}

	// Another contact's context reads nothing of this caller's.
	stranger := ids.New[ids.UserKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other')`,
		stranger, "other-"+stranger.String()+"@comms.test"); err != nil {
		t.Fatal(err)
	}
	othersView, err := e.store.ParkedSendsFor(readerCtx(e.ws, stranger), since, 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor as another contact: %v", err)
	}
	if len(othersView) != 0 {
		t.Fatalf("another reader reads %+v, want nothing of this caller's", othersView)
	}
}

// A caller whose role lacks the activity read grant is refused like any other
// timeline read: the subject lines of their sends are activity content.
func TestParkedSendsForRefusesAReaderWithoutTheActivityGrant(t *testing.T) {
	e := setupStore(t)
	if _, err := e.store.ParkedSendsFor(e.ctx, e.clockValue.Add(-time.Hour), 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("ParkedSendsFor without the activity grant = %v, want the permission sentinel", err)
	}
}
