// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a deal room invitation says about the mail it sent.
//
// It says `queued`: a relay accepted the message for sending. A mailbox
// receiving it is a later and separate fact, and an address that hard-bounces a
// second afterwards was queued all the same. The distinction decides what a
// seller does next — told the invitation went, they wait; told it did not, they
// copy the link and pass it on themselves — so a field that overstated this
// would leave a buyer with no way into the room and a seller unaware of it.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

var errRelayRefused = errors.New("the relay refused the envelope")

// refusingMailer is a relay that takes nothing. It stands for the ordinary
// outage — configured, attempted, and the message still does not go — which is
// the case that separates "queued" from "we have somewhere to send".
type refusingMailer struct{}

func (refusingMailer) Send(context.Context, string, string, string) error {
	return errRelayRefused
}

// A relay that accepts the message reads as queued.
func TestAnAcceptedInvitationReadsAsQueued(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(discardingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	if !openRoomWithABuyer(t, e).queued {
		t.Error("a relay that accepted the invitation reads as not queued, so the seller is told " +
			"to pass on a link that is already on its way")
	}
}

// An installation with no relay reads as not queued, and the credential is
// still issued — the seller passes it on themselves.
func TestAnInvitationWithNoRelayReadsAsNotQueued(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	// openRoomWithABuyer fatals on an empty credential, which is the assertion
	// this case most needs: sending is best-effort and must never cost the
	// seller the link they are being asked to pass on by hand.
	if openRoomWithABuyer(t, e).queued {
		t.Error("an installation with no relay reads as queued, so the seller waits for mail that " +
			"was never attempted")
	}
}

// Resend answers the same way, because it is the same question.
//
// Both endpoints render through issuedBody today, which is a fact about one
// function rather than a promise. A resend that grew its own body — the obvious
// place for it, since it supersedes rather than creates — would ship the old
// claim on the endpoint a seller reaches precisely when the first mail did not
// arrive. This asks resend directly, so that change fails here.
func TestAResentInvitationReportsQueuedTheSameWay(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(refusingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)

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
	if queued {
		t.Error("a resent invitation the relay refused reads as queued, so a seller retrying a " +
			"failed send is told the second one went when it did not either")
	}
}

// participantOf reads the one buyer's id off the room's roster.
func participantOf(t *testing.T, e *apptest.AppEnv, room buyerRoom) string {
	t.Helper()
	var roster struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID+"/participants",
		nil, nil, &roster); status != http.StatusOK {
		t.Fatalf("read roster = %d", status)
	}
	if len(roster.Data) != 1 {
		t.Fatalf("the room holds %d participant(s), want the one this suite invited", len(roster.Data))
	}
	id := roster.Data[0].ID
	if id == "" {
		t.Fatal("the roster entry carries no id")
	}
	return id
}

// A relay that REFUSES the message reads as not queued.
//
// The case that discriminates: the installation can send, the send was
// attempted, and it did not happen. A field reporting the ATTEMPT rather than
// the outcome passes both cases above and fails only this one.
func TestARefusedInvitationReadsAsNotQueued(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithDealRoomInviteMail(refusingMailer{}),
		compose.WithPublicBaseURL("https://rooms.example.test"))
	e.BootstrapWorkspace(t)

	if openRoomWithABuyer(t, e).queued {
		t.Error("an invitation the relay refused reads as queued, so the seller is told it is on " +
			"its way when nothing left")
	}
}
