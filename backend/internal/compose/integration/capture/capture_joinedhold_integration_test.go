// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// threadedReply is a reply whose References name SEVERAL messages: the first is
// the root its own thread key comes from, and the rest are what the join can
// reach. A shortened chain — iPhone Mail sends one — is how a real reply ends up
// rooted somewhere other than the conversation's opener.
func threadedReply(msgID string, references ...string) []byte {
	return []byte(strings.Join([]string{
		"From: partner@acme.example",
		"To: " + captureOwner,
		"Subject: project",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"References: <" + strings.Join(references, "> <") + ">",
		"Content-Type: text/plain", "", "hello", "",
	}, "\r\n"))
}

// clearThreadOf is holdThreadOf's mirror: the classifier judged this conversation
// shareable, and SAW the address the reply comes from — without that the inherit
// path reopens the ledger instead of inheriting, and the reply's birth would decide
// the case rather than the join.
func clearThreadOf(t *testing.T, e *integration.SearchEnv, user ids.UUID, sourceID, seen string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			SELECT a.thread_key, $2, $3, $4, ARRAY[$5], now() FROM activity a WHERE a.source_id = $1
			ON CONFLICT (thread_key, user_id) DO UPDATE
			   SET status = EXCLUDED.status, seen_addresses = EXCLUDED.seen_addresses,
			       resolved_at = EXCLUDED.resolved_at`,
			sourceID, user, capturemod.VerdictCleared, "business", seen)
		return err
	}); err != nil {
		t.Fatalf("clearing the thread of %s: %v", sourceID, err)
	}
}

// holdThreadOf records the verdict a classifier leaves on the conversation a
// message is filed under, modelled the way seedPersonalThread models it: the
// capture pass opens the row and the classifier answers it, and the answer is
// what a fixture can state without an LLM in the lane.
func holdThreadOf(t *testing.T, e *integration.SearchEnv, user ids.UUID, sourceID string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			SELECT a.thread_key, $2, $3, $4, '{}', now() FROM activity a WHERE a.source_id = $1
			ON CONFLICT (thread_key, user_id) DO UPDATE
			   SET status = EXCLUDED.status, resolved_at = EXCLUDED.resolved_at`,
			sourceID, user, capturemod.VerdictHeld, "business")
		return err
	}); err != nil {
		t.Fatalf("holding the thread of %s: %v", sourceID, err)
	}
}

func threadKeyOf(t *testing.T, e *integration.SearchEnv, sourceID string) string {
	t.Helper()
	var key string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(thread_key, '') FROM activity WHERE source_id = $1`, sourceID).Scan(&key)
	}); err != nil {
		t.Fatalf("reading the thread key of %s: %v", sourceID, err)
	}
	return key
}

// A reply the join moves into a held conversation is held with it.
//
// Under a shared posture a message is born open, and the birth decision reads the
// verdict of the key the message's OWN References root gives it. A shortened chain
// roots this reply on a key carrying no verdict, so nothing it reads at birth
// holds it; the join then files it under the opener's thread, which the classifier
// is holding. The thread is held and one message in it is not.
func TestAReplyJoiningAHeldThreadIsHeldWithIt(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setMailSharing(t, e, true)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// Born open, and held by the classifier afterwards.
	sync(t, email("partner@acme.example", "", captureOwner, "jh-opener@acme.example", ""))
	if audience, _ := audienceOf(t, e, activityIDOf(t, e, "jh-opener@acme.example")); audience != "workspace" {
		t.Fatalf("the opener is %q under a shared posture, want workspace — this case says nothing "+
			"about a join unless messages are born open", audience)
	}
	holdThreadOf(t, e, e.Rep1, "jh-opener@acme.example")

	// Rooted on a thread of its own, so the reply's birth reads no held verdict.
	sync(t, email("partner@acme.example", "", captureOwner, "jh-stray@acme.example", ""))
	sync(t, threadedReply("jh-reply@acme.example", "jh-stray@acme.example", "jh-opener@acme.example"))

	opener := threadKeyOf(t, e, "jh-opener@acme.example")
	if got := threadKeyOf(t, e, "jh-reply@acme.example"); got != opener {
		t.Fatalf("the reply is filed under %q, want the held opener's %q — the audience assertion "+
			"below means nothing unless the join moved it", got, opener)
	}
	audience, reason := audienceOf(t, e, activityIDOf(t, e, "jh-reply@acme.example"))
	if audience != "participants" {
		t.Errorf("the reply reads %q inside a held conversation, want participants — the whole "+
			"workspace can read a message the classifier held this thread for", audience)
	}
	if reason == "" {
		t.Error("the reply is held with no reason recorded, so nothing can widen it back")
	}
}

// And it only ever closes. A reply born held keeps its hold when the thread it
// joins says nothing: that hold was placed for something other than the filing.
func TestAHeldReplyJoiningAnOpenThreadStaysHeld(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setMailSharing(t, e, true)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, email("partner@acme.example", "", captureOwner, "oh-opener@acme.example", ""))
	// This one's OWN thread is held, so it is born held; the thread it joins is not.
	sync(t, email("partner@acme.example", "", captureOwner, "oh-stray@acme.example", ""))
	holdThreadOf(t, e, e.Rep1, "oh-stray@acme.example")
	sync(t, threadedReply("oh-reply@acme.example", "oh-stray@acme.example", "oh-opener@acme.example"))

	if audience, _ := audienceOf(t, e, activityIDOf(t, e, "oh-reply@acme.example")); audience != "participants" {
		t.Errorf("a reply born held reads %q after joining an open thread, want participants — "+
			"the join widened a hold placed for another reason", audience)
	}
}

// A reply joining a CLEARED conversation keeps the audience it was born with. The
// re-derivation only ever closes, so a thread the classifier judged shareable
// changes nothing about a message that lands in it — and the branch that decides
// this is the one a fix that closed on any verdict would get wrong.
func TestAReplyJoiningAClearedThreadStaysOpen(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setMailSharing(t, e, true)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, email("partner@acme.example", "", captureOwner, "cl-opener@acme.example", ""))
	clearThreadOf(t, e, e.Rep1, "cl-opener@acme.example", "partner@acme.example")

	sync(t, email("partner@acme.example", "", captureOwner, "cl-stray@acme.example", ""))
	sync(t, threadedReply("cl-reply@acme.example", "cl-stray@acme.example", "cl-opener@acme.example"))

	opener := threadKeyOf(t, e, "cl-opener@acme.example")
	if got := threadKeyOf(t, e, "cl-reply@acme.example"); got != opener {
		t.Fatalf("the reply is filed under %q, want the cleared opener's %q — the assertion below "+
			"says nothing unless the join moved it", got, opener)
	}
	if audience, _ := audienceOf(t, e, activityIDOf(t, e, "cl-reply@acme.example")); audience != "workspace" {
		t.Errorf("the reply reads %q after joining a CLEARED conversation, want workspace — the "+
			"re-derivation closed a message no verdict asked to close", audience)
	}
}

// The hold SURVIVES a recompute, which is the half that makes it a fix rather than
// a moment. activities.RecomputeAudienceTx derives an audience from capture_import,
// so a narrowing recorded only on the activity is one the next recompute undoes —
// and recomputes are routine: alias adoption runs one for every message it adopts.
func TestAJoinedHoldSurvivesAnAudienceRecompute(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setMailSharing(t, e, true)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, email("partner@acme.example", "", captureOwner, "sv-opener@acme.example", ""))
	holdThreadOf(t, e, e.Rep1, "sv-opener@acme.example")
	sync(t, email("partner@acme.example", "", captureOwner, "sv-stray@acme.example", ""))
	sync(t, threadedReply("sv-reply@acme.example", "sv-stray@acme.example", "sv-opener@acme.example"))

	reply := activityIDOf(t, e, "sv-reply@acme.example")
	before, beforeReason := audienceOf(t, e, reply)
	if before != "participants" {
		t.Fatalf("the reply reads %q before any recompute, want participants — the case below says "+
			"nothing unless the join held it first", before)
	}

	// The package's own helper, which runs the production derivation in its own
	// transaction the way the sink runs it — correlation id included.
	recompute(t, e, reply)

	after, afterReason := audienceOf(t, e, reply)
	if after != "participants" {
		t.Errorf("the reply reads %q after a recompute, want participants — the hold was recorded "+
			"where the derivation does not look, so routine work reopens a held conversation", after)
	}
	// The reason too: a recompute that agrees on the audience and rewrites the word
	// churns the row and shows a reader a different explanation each time.
	if afterReason != beforeReason {
		t.Errorf("the recompute rewrote the reason from %q to %q — the import row and the activity "+
			"disagree about why this message is held", beforeReason, afterReason)
	}
}
