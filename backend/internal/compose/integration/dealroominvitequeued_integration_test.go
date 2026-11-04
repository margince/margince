// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a deal room invitation says about the mail it sent.
//
// The invite answers without waiting for the relay. `queued` says the mail was
// handed to the sender. The roster reads `pending` until the relay answers,
// then `sent` or `failed`, so a seller learns of a refusal on the row.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var errRelayRefused = errors.New("the relay refused the envelope")

// refusingMailer is a relay that takes nothing. It stands for the ordinary
// outage — configured, attempted, and the message still does not go — which is
// the case that separates "queued" from "we have somewhere to send".
type refusingMailer struct{}

func (refusingMailer) Send(context.Context, string, string, string) error {
	return errRelayRefused
}

// heldMailer holds every send until the test releases it.
type heldMailer struct{ release chan struct{} }

func (m heldMailer) Send(ctx context.Context, _, _, _ string) error {
	select {
	case <-m.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// The relay holds its answer, so the invite can answer only if the send runs
// off the request. The roster reads pending while the relay holds the mail and
// sent once it takes it.
func TestAnInviteAnswersBeforeASlowRelayAndTheRosterCatchesUp(t *testing.T) {
	relay := heldMailer{release: make(chan struct{})}
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(relay),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)
	defer func() {
		select {
		case <-relay.release:
		default:
			close(relay.release)
		}
	}()

	room := openRoomWithABuyer(t, e)
	if !room.queued {
		t.Error("an invitation handed to the sender reads as not queued")
	}
	if state := deliveryStateOf(t, e, room); state != "pending" {
		t.Errorf("while the relay holds the mail the roster reads %q, want pending", state)
	}
	close(relay.release)
	waitForDeliveryState(t, e, room, "sent")
}

// A relay that accepts the message reads as queued, and the roster as sent.
func TestAnAcceptedInvitationReadsAsQueued(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(discardingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	room := openRoomWithABuyer(t, e)
	if !room.queued {
		t.Error("a relay that accepted the invitation reads as not queued, so the seller is told " +
			"to pass on a link that is already on its way")
	}
	waitForDeliveryState(t, e, room, "sent")
}

// An installation with no relay reads as not queued, and the credential is
// still issued, so the seller passes it on themselves.
func TestAnInvitationWithNoRelayReadsAsNotQueued(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	// openRoomWithABuyer fatals on an empty credential, which is the assertion
	// this case most needs: sending is best-effort and must never cost the
	// seller the link they are being asked to pass on by hand.
	room := openRoomWithABuyer(t, e)
	if room.queued {
		t.Error("an installation with no relay reads as queued, so the seller waits for mail that " +
			"was never attempted")
	}
	if state := deliveryStateOf(t, e, room); state != "pending" {
		t.Errorf("with no relay the roster reads %q, want pending", state)
	}
}

// Resend answers the same way, because it is the same question. Both endpoints
// render through issuedBody today; this asks resend directly, so a resend that
// grew its own body fails here.
func TestAResentInvitationReportsQueuedTheSameWay(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(refusingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	waitForDeliveryState(t, e, room, "failed")

	var resent AnyMap
	if status := e.Call(t, "POST",
		"/v1/deal-rooms/"+room.roomID+"/participants/"+participantOf(t, e, room)+"/resend",
		AnyMap{}, nil, &resent); status != http.StatusCreated {
		t.Fatalf("resend = %d %v", status, resent)
	}

	queued, present := resent["queued"].(bool)
	if !present {
		t.Fatalf("the resent invitation carries no boolean `queued`: %v", resent)
	}
	if !queued {
		t.Error("a resent invitation handed to the sender reads as not queued")
	}
	waitForDeliveryState(t, e, room, "failed")
}

// participantOf reads the one buyer's id off the room's roster.
func participantOf(t *testing.T, e *apptest.AppEnv, room buyerRoom) string {
	t.Helper()
	id, _ := soleParticipant(t, e, room)
	return id
}

// deliveryStateOf reads the one buyer's delivery state off the room's roster.
func deliveryStateOf(t *testing.T, e *apptest.AppEnv, room buyerRoom) string {
	t.Helper()
	_, state := soleParticipant(t, e, room)
	return state
}

func soleParticipant(t *testing.T, e *apptest.AppEnv, room buyerRoom) (string, string) {
	t.Helper()
	var roster struct {
		Data []struct {
			ID            string `json:"id"`
			DeliveryState string `json:"delivery_state"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID+"/participants",
		nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("read roster = %d", status)
	}
	if len(roster.Data) != 1 {
		t.Fatalf("the room holds %d participant(s), want the one this suite invited", len(roster.Data))
	}
	if roster.Data[0].ID == "" {
		t.Fatal("the roster entry carries no id")
	}
	return roster.Data[0].ID, roster.Data[0].DeliveryState
}

// waitForDeliveryState reads the roster until the relay's answer lands on it.
// The send runs off the request, so the answer arrives a moment after the
// invite does. Bounded by a number of reads rather than one fixed sleep.
func waitForDeliveryState(t *testing.T, e *apptest.AppEnv, room buyerRoom, want string) {
	t.Helper()
	const reads, pause = 500, 20 * time.Millisecond
	got := ""
	for range reads {
		if got = deliveryStateOf(t, e, room); got == want {
			return
		}
		//craft:ignore test-sleep the poll interval for a condition wait bounded by the read count above, not the fixed-duration sleep this check exists to catch
		time.Sleep(pause)
	}
	t.Fatalf("the roster reads %q after %d reads, want %q", got, reads, want)
}

// A relay that REFUSES the message was still handed over, so the invitation
// reads as queued, and the refusal lands on the roster as failed.
func TestARefusedInvitationShowsAsFailedOnTheRoster(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(refusingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	room := openRoomWithABuyer(t, e)
	if !room.queued {
		t.Error("an invitation handed to the sender reads as not queued")
	}
	waitForDeliveryState(t, e, room, "failed")
}

// roomSenderContext holds deal_room update on the installation's workspace,
// the authority the detached send reads and stamps the invitation under.
func roomSenderContext(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	user := ids.NewV7()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"deal_room": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// The detached send mails a link only while it still admits its bearer and
// the participant still has the address it was issued to.
func TestAnInvitationIsMailableOnlyWhileItStandsAtItsAddress(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)
	store := dealrooms.NewStore(e.DB())
	ctx := roomSenderContext(t, e)
	room := openRoomWithABuyer(t, e)

	mailable := func(credential, email string) bool {
		t.Helper()
		ok, err := store.InvitationMailable(ctx, credential, email)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !mailable(room.credential, room.email) {
		t.Fatal("a standing invitation at its own address reads as not mailable")
	}
	if mailable(room.credential, "someone.else@buyer.example") {
		t.Error("an invitation reads as mailable to an address it was not issued to")
	}

	var resent AnyMap
	participant := participantOf(t, e, room)
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/participants/"+participant+"/resend",
		AnyMap{}, nil, &resent); status != http.StatusCreated {
		t.Fatalf("resend = %d %v", status, resent)
	}
	if mailable(room.credential, room.email) {
		t.Error("a link a resend retired still reads as mailable")
	}
	fresh, _ := resent["credential"].(string)
	if !mailable(fresh, room.email) {
		t.Fatal("the resent link reads as not mailable")
	}

	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/participants/"+participant+"/revoke",
		AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("revoke = %d", status)
	}
	if mailable(fresh, room.email) {
		t.Error("a revoked participant's link still reads as mailable")
	}
}
