// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// importedFrom is the importer's create for one message, From the given sender.
func importedFrom(messageID, from string) crmcontracts.CreateActivityRequest {
	req := importedFromAnotherCRM(messageID, "Shared with you", "A presentation was shared with you.")
	req.Participants.From = &from
	sourceID := "engagement-" + messageID
	req.SourceId = &sourceID
	return req
}

// automaticKindOf reads the row through the activities module's own read, so
// the wire field comes from the projection every reader shares.
func automaticKindOf(t *testing.T, e *integration.Env, seat ids.UUID, messageID string) *crmcontracts.ActivityAutomaticKind {
	t.Helper()
	id := columnOfMessage[ids.UUID](t, e, "id", messageID)
	got, err := activities.NewStore(e.DB()).GetActivity(seatCtx(t, e, seat), ids.ActivityID{UUID: id}, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading %s: %v", messageID, err)
	}
	return got.AutomaticKind
}

// Every writer of mail stamps a sending system's message by capture's rule.
// That covers the importer's create door and a mailbox taking over an import.
// A delivery report is not a case here: the importer's door refuses it.
func TestEveryMailWriterMarksASendingSystemsMessage(t *testing.T) {
	e := integration.Setup(t)
	owner := e.Rep1
	for _, tc := range []struct {
		name, messageID, from string
		want                  bool
	}{
		{"a file-share notice", "notice@writers.test", "drive-shares-dm-noreply@google.com", true},
		{"a human's mail", "human@writers.test", "pat@counterparty.example", false},
		{"an export that named the relay as its sender", "relayed@writers.test", "slides-relay@counterparty.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			importThrough(t, e, owner, importedFrom(tc.messageID, tc.from))
			if got := columnOfMessage[bool](t, e, "sender_is_machine", tc.messageID); got != tc.want {
				t.Fatalf("sender_is_machine=%v, want %v", got, tc.want)
			}
			kind := automaticKindOf(t, e, owner, tc.messageID)
			if tc.want != (kind != nil && *kind == crmcontracts.ActivityAutomaticKindSendingSystem) {
				t.Fatalf("automatic_kind=%v, want sending_system=%v", kind, tc.want)
			}
		})
	}

	// A log with no From is a human's own account, whatever it is filed on.
	inbound, subject := "inbound", "Notes from the call"
	logged, _, err := activities.NewStore(e.DB()).LogActivity(seatCtx(t, e, owner), activities.LogActivityInput{
		Kind: "email", Direction: &inbound, Subject: &subject, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if logged.AutomaticKind != nil {
		t.Fatalf("a log with no From reads automatic_kind=%s", *logged.AutomaticKind)
	}

	// The mailbox then reads the relayed import as the notice it was.
	const messageID = "relayed@writers.test"
	captureWithTakeOver(t, e, owner, theSameMessage(messageID, "drive-shares-dm-noreply@google.com", ownerAddr))
	if got := activitiesDescribing(t, e, messageID); got != 1 {
		t.Fatalf("the take-over made %d activities, want 1", got)
	}
	if !columnOfMessage[bool](t, e, "sender_is_machine", messageID) {
		t.Fatal("the mailbox read the message from a sending system and the row still counts as a human's")
	}
	// A human's import keeps counting when the same mailbox reads it.
	captureWithTakeOver(t, e, owner, theSameMessage("human@writers.test", "pat@counterparty.example", ownerAddr))
	if columnOfMessage[bool](t, e, "sender_is_machine", "human@writers.test") {
		t.Fatal("a human's mail read by the mailbox was marked as a sending system's")
	}
}
