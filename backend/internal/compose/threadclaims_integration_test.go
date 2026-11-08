// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func threadReadableBy(t *testing.T, e *integration.Env, seat ids.UUID, message string) bool {
	t.Helper()
	ctx := e.As(seat, nil, integration.RepPerms)
	var visible bool
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(value any) int { args = append(args, value); return len(args) }
		clause, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.source_id = $%d AND %s)`, arg(message), clause)
		return tx.QueryRow(ctx, query, args...).Scan(&visible)
	})
	if err != nil {
		t.Fatal(err)
	}
	return visible
}

func TestForgedReferenceChainsCannotClaimAnotherSeatsConversation(t *testing.T) {
	for _, variant := range []string{"first arrival", "legacy neighbour", "legacy replay", "archived foreign message"} {
		t.Run(variant, func(t *testing.T) {
			e := integration.Setup(t)
			victim, mine := seatAddress(t, e, e.Rep2), seatAddress(t, e, e.Rep1)
			captureThreadMail(t, e, e.Rep2, threadMail(victim, "victim@example.test", "Mon, 01 Jun 2026 08:00:00 +0000", ""))
			captureThreadMail(t, e, e.Rep1, threadMail(mine, "mine@example.test", "Tue, 02 Jun 2026 08:00:00 +0000", ""))
			middle := threadMail(mine, "middle@example.test", "Wed, 03 Jun 2026 08:00:00 +0000", "victim@example.test", "victim@example.test")
			captureThreadMail(t, e, e.Rep1, middle)
			if got := threadKeyOfMessage(t, e, "middle@example.test"); got == "victim@example.test" {
				t.Fatal("a first arrival joined a foreign conversation")
			}
			beforeMine := threadReadableBy(t, e, e.Rep1, "victim@example.test")
			beforeVictim := threadReadableBy(t, e, e.Rep2, "victim@example.test")
			if !threadReadableBy(t, e, e.Rep1, "mine@example.test") {
				t.Fatal("the seat must read its own message")
			}
			if beforeMine || !beforeVictim {
				t.Fatal("fixture must keep the victim's content private")
			}
			if variant != "first arrival" {
				// Retained rows may carry a foreign key from capture before membership checks.
				e.WsExec(t, `UPDATE activity SET thread_key = 'victim@example.test' WHERE source_id = 'middle@example.test'`)
			}
			if variant == "archived foreign message" {
				e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE source_id = 'victim@example.test'`)
			}
			if variant == "legacy replay" {
				replay := threadMail(mine, "middle@example.test", "Wed, 03 Jun 2026 08:00:00 +0000", "mine@example.test", "victim@example.test", "mine@example.test")
				captureThreadMail(t, e, e.Rep1, replay)
			} else {
				captureThreadMail(t, e, e.Rep1, threadMail(mine, "bridge@example.test", "Thu, 04 Jun 2026 08:00:00 +0000", "middle@example.test", "middle@example.test", "mine@example.test"))
			}
			for _, message := range []string{"victim@example.test", "mine@example.test"} {
				if got := threadKeyOfMessage(t, e, message); got != message {
					t.Errorf("%s moved into %s", message, got)
				}
			}
			if !threadReadableBy(t, e, e.Rep1, "mine@example.test") {
				t.Error("the seat lost its own message")
			}
			if threadReadableBy(t, e, e.Rep1, "victim@example.test") != beforeMine || threadReadableBy(t, e, e.Rep2, "victim@example.test") != beforeVictim {
				t.Error("refusing the merge changed message audiences")
			}
		})
	}
}

func TestSharedConversationKeysStillJoinWhenBothSeatsHoldEveryMessage(t *testing.T) {
	e := integration.Setup(t)
	both := seatAddress(t, e, e.Rep1) + ", " + seatAddress(t, e, e.Rep2)
	for i, root := range []string{"first@example.test", "second@example.test"} {
		at := fmt.Sprintf("%s, 0%d Jun 2026 08:00:00 +0000", []string{"Mon", "Tue"}[i], i+1)
		raw := threadMail(both, root, at, "")
		for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
			captureThreadMail(t, e, seat, raw)
		}
	}
	captureThreadMail(t, e, e.Rep1, threadMail(both, "bridge@example.test", "Wed, 03 Jun 2026 08:00:00 +0000", "second@example.test", "first@example.test", "second@example.test"))
	if first, second := threadKeyOfMessage(t, e, "first@example.test"), threadKeyOfMessage(t, e, "second@example.test"); first != second {
		t.Errorf("shared messages remained in %s and %s", first, second)
	}
}

func TestAChosenMessageIDCannotReuseAnotherSeatsHeaderRoot(t *testing.T) {
	e := integration.Setup(t)
	captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), "foreign@example.test", "Mon, 01 Jun 2026 08:00:00 +0000", "future@example.test", "future@example.test"))
	captureThreadMail(t, e, e.Rep1, threadMail(seatAddress(t, e, e.Rep1), "future@example.test", "Tue, 02 Jun 2026 08:00:00 +0000", ""))
	if threadKeyOfMessage(t, e, "foreign@example.test") == threadKeyOfMessage(t, e, "future@example.test") {
		t.Error("a chosen Message-ID joined another seat's existing header root")
	}
}

func TestAReplyToAnIsolatedMessageKeepsItsConversation(t *testing.T) {
	e := integration.Setup(t)
	captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), "foreign@example.test", "Mon, 01 Jun 2026 08:00:00 +0000", "future@example.test", "future@example.test"))
	captureThreadMail(t, e, e.Rep1, threadMail(seatAddress(t, e, e.Rep1), "future@example.test", "Tue, 02 Jun 2026 08:00:00 +0000", ""))
	key := threadKeyOfMessage(t, e, "future@example.test")
	anchor, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM activity WHERE source_id='future@example.test'`))
	if err != nil {
		t.Fatal(err)
	}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: &recordingStager{}})
	sent, err := adapter.SendEmail(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms), anchor, agents.SendEmailArgs{To: []string{"pat@counterparty.example"}, Subject: "Re: Angebot", Body: "I will send the draft.", ConsentPurpose: "transactional"})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT thread_key FROM activity WHERE id=$1`, sent.ActivityID); got != key {
		t.Fatalf("reply moved from %s to %s", key, got)
	}
}

// A colleague's private reply does not split the seat off a conversation it
// shares. Its later mail joins, and the private reply stays private.
func TestAConversationWithAnUnheldPrivateReplyStaysOneConversation(t *testing.T) {
	e := integration.Setup(t)
	both := seatAddress(t, e, e.Rep1) + ", " + seatAddress(t, e, e.Rep2)
	raw := threadMail(both, "shared@example.test", "Mon, 01 Jun 2026 08:00:00 +0000", "")
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		captureThreadMail(t, e, seat, raw)
	}
	captureThreadMail(t, e, e.Rep2, threadMail(seatAddress(t, e, e.Rep2), "private@example.test", "Tue, 02 Jun 2026 08:00:00 +0000", "shared@example.test", "shared@example.test"))
	captureThreadMail(t, e, e.Rep1, threadMail(seatAddress(t, e, e.Rep1), "later@example.test", "Wed, 03 Jun 2026 08:00:00 +0000", "shared@example.test", "shared@example.test"))
	shared := threadKeyOfMessage(t, e, "shared@example.test")
	if got := threadKeyOfMessage(t, e, "later@example.test"); got != shared {
		t.Errorf("the seat's later mail is filed under %q, want the shared conversation %q", got, shared)
	}
	if got := threadKeyOfMessage(t, e, "private@example.test"); got != shared {
		t.Errorf("the private reply moved to %q", got)
	}
	if threadReadableBy(t, e, e.Rep1, "private@example.test") {
		t.Fatal("joining the conversation exposed the private reply")
	}
}
