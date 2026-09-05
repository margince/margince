// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An owner's own say over a thread they imported, and the limits on it.
//
// The classifier answers first and is usually right; this is what happens when
// it is not. What the tests below are mostly about is the limits: a share
// releases the caller's OWN hold and cannot publish what a colleague is still
// holding, because a message reaching two mailboxes ends at the strictest of
// what the two of them ask for.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnOwnerSharesTheirOwnHeldThread(t *testing.T) {
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-share", "kunde@example.test", e.Rep1)

	outcome := decideThread(t, e, e.Rep1, "thread-share", true)
	if outcome.Messages != 1 || !outcome.Shared {
		t.Fatalf("messages=%d shared=%v, want 1 and true", outcome.Messages, outcome.Shared)
	}
	if got := activityAudience(t, e, id); got != "workspace" {
		t.Fatalf("a shared thread's message is %q, want workspace", got)
	}
}

func TestAnOwnerKeepsAThreadPrivateAgainstTheClassifier(t *testing.T) {
	// The click has to mean something permanent. A rep whose ordinary-looking
	// thread turned personal keeps it private, and no later pass may re-open it
	// — a classifier that could overturn an owner would make the click
	// advisory.
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-hold", "privat@example.test", e.Rep1)
	setImportVerdict(t, e, id, e.Rep1, "cleared")
	recompute(t, e, id)
	if got := activityAudience(t, e, id); got != "workspace" {
		t.Fatalf("the fixture starts %q, want workspace — otherwise the hold below proves nothing", got)
	}

	outcome := decideThread(t, e, e.Rep1, "thread-hold", false)
	if outcome.Shared {
		t.Fatal("keeping a thread private reported it as shared")
	}
	if got := activityAudience(t, e, id); got != "participants" {
		t.Fatalf("a thread the owner kept private is %q, want participants", got)
	}
	if got := importVerdictFor(t, e, id, e.Rep1); got != "held_by_owner" {
		t.Fatalf("the owner's contribution says %q, want held_by_owner — a person's answer is not a classifier's", got)
	}
}

func TestOneOwnersShareCannotPublishAColleaguesHeldMessage(t *testing.T) {
	// The whole per-owner model, at the point where it is easiest to break.
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-both", "kunde@example.test", e.Rep1)
	addImportRowFor(t, e, id, e.Rep2)

	outcome := decideThread(t, e, e.Rep1, "thread-both", true)

	if outcome.HeldByOthers != 1 {
		t.Fatalf("held_by_others=%d, want 1 — the caller is owed the fact that somebody else has a say", outcome.HeldByOthers)
	}
	if outcome.Shared {
		t.Fatal("the share reported success while a colleague was still holding the message")
	}
	if got := activityAudience(t, e, id); got != "participants" {
		t.Fatalf("the message is %q, want participants — one owner cannot publish what another holds", got)
	}
	// And the colleague's own contribution is untouched.
	if got := importVerdictFor(t, e, id, e.Rep2); got != "pending" {
		t.Fatalf("the colleague's contribution says %q, want pending", got)
	}
}

func TestTheLastOwnersShareReportsTheMessageOpen(t *testing.T) {
	// One message, two mailboxes, both `classified` at import — the default
	// posture, so this is the ordinary two-importer case. The first owner's
	// share is refused honestly: the colleague still holds. The second owner's
	// share releases the LAST hold, and the answer must say so on both fields —
	// a posture is what the mailbox asked of mail in general, and a seat's
	// explicit shared_by_owner ends its say. The recompute already opens the
	// message here; an answer still counting the posture as a hold tells the
	// person who just published a conversation that it stayed private.
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-last-holder", "kunde@example.test", e.Rep1)
	addImportRowFor(t, e, id, e.Rep2)

	first := decideThread(t, e, e.Rep1, "thread-last-holder", true)
	if first.Shared || first.HeldByOthers != 1 {
		t.Fatalf("first share: shared=%v held_by_others=%d, want false and 1 — the colleague is still holding",
			first.Shared, first.HeldByOthers)
	}

	last := decideThread(t, e, e.Rep2, "thread-last-holder", true)
	if last.HeldByOthers != 0 {
		t.Fatalf("held_by_others=%d after both owners shared, want 0 — a shared_by_owner seat is not holding, "+
			"whatever posture the message arrived under", last.HeldByOthers)
	}
	if !last.Shared {
		t.Fatal("the last holder's share reported the message private while it opened to the workspace")
	}
	if got := activityAudience(t, e, id); got != "workspace" {
		t.Fatalf("the message is %q after both owners shared, want workspace", got)
	}
}

func TestAThreadYouDidNotImportIsNotFound(t *testing.T) {
	// A thread key is the sender-controlled References root in a namespace the
	// whole workspace shares, so it is guessable. Answering not-found rather
	// than "nothing to do" keeps it from confirming that somebody else's
	// correspondence exists.
	e := integration.Setup(t)
	seedHeldThreadOn(t, e, "thread-theirs", "kunde@example.test", e.Rep1)

	_, err := NewThreadAudienceSetter(e.Pool).Decide(purgeCtx(e, e.Rep2), "thread-theirs", true)
	if err == nil {
		t.Fatal("a seat decided about a thread they never imported")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not-found — a refusal that named the thread would confirm it exists", err)
	}
}

func decideThread(t *testing.T, e *integration.Env, user ids.UUID, threadKey string, share bool) ThreadAudienceOutcome {
	t.Helper()
	outcome, err := NewThreadAudienceSetter(e.Pool).Decide(purgeCtx(e, user), threadKey, share)
	if err != nil {
		t.Fatalf("deciding about the thread: %v", err)
	}
	return outcome
}

// seedHeldThreadOn lands one held message on a thread with the import row that
// makes the seeding seat its importer.
func seedHeldThreadOn(t *testing.T, e *integration.Env, threadKey, from string, user ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, thread_key,
			                      audience, audience_reason)
			VALUES ($1, 'email', 'Betreff', 'the message body', 'inbound', 'gmail', $2,
			        'gmail:'||$2, 'connector:gmail', $3, $4, 'participants', 'pending_verdict')`,
			id, "th-"+id.String(), from, threadKey); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_import (activity_id, user_id, posture_at_import, verdict_status)
			VALUES ($1, $2, 'classified', 'pending')`, id, user)
		return err
	}); err != nil {
		t.Fatalf("seeding thread mail: %v", err)
	}
	return id
}

func setImportVerdict(t *testing.T, e *integration.Env, activityID, user ids.UUID, status string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_import SET verdict_status = $3
			 WHERE activity_id = $1 AND user_id = $2`, activityID, user, status)
		return err
	}); err != nil {
		t.Fatalf("setting the import verdict: %v", err)
	}
}

// recompute re-derives one message's audience the way every writer does, so a
// fixture that sets a contribution directly still reaches the state production
// would have reached.
func recompute(t *testing.T, e *integration.Env, activityID ids.UUID) {
	t.Helper()
	ctx := purgeCtx(e, e.Rep1)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](activityID))
	}); err != nil {
		t.Fatalf("recomputing the audience: %v", err)
	}
}

func TestTheHoldCountReadsOnlyTheCallersOwnMessages(t *testing.T) {
	// A thread key is the RFC822 References root, taken verbatim from a
	// sender's header — guessable, forgeable, and shared across one workspace
	// namespace. Counting holders BY thread key would walk messages the caller
	// never received, and a capture_import row is itself an arm of the audience
	// gate: the count would read exactly the membership a held message hides.
	//
	// A seat could then map which colleagues are on which private
	// conversations, one thread key at a time, without reading a word of them.
	e := integration.Setup(t)
	const key = "thread-shared-key"
	// The caller's own message on the thread.
	mine := seedHeldThreadOn(t, e, key, "kunde@example.test", e.Rep1)
	// A LATER message on the same thread that only a colleague imported — the
	// conversation continued without the caller.
	theirs := seedHeldThreadOn(t, e, key, "kunde@example.test", e.Rep2)

	outcome := decideThread(t, e, e.Rep1, key, true)

	if outcome.HeldByOthers != 0 {
		t.Fatalf("held_by_others=%d, want 0 — the colleague holds a message this caller never "+
			"received, and counting it tells them who is on a conversation they are not", outcome.HeldByOthers)
	}
	if got := activityAudience(t, e, mine); got != "workspace" {
		t.Fatalf("the caller's own message is %q, want workspace — nobody else holds it", got)
	}
	// And the colleague's message is untouched by a decision that was never
	// about it.
	if got := activityAudience(t, e, theirs); got != "participants" {
		t.Fatalf("the colleague's message is %q, want participants", got)
	}
}

// The decision names the messages it reached, so a client can refresh exactly
// them rather than guessing from the record it happened to be looking at.
//
// It names only the ones the CALLER may read. An import row satisfies one arm
// of the audience gate and says nothing about discoverability, so a message
// filed only against records outside this seat's row scope is counted by
// Messages and left out of the list: the decision reached it, and saying which
// id it was would say that record exists.
func TestAThreadDecisionNamesTheMessagesTheCallerCanRead(t *testing.T) {
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-ids", "kunde@example.test", e.Rep1)

	outcome := decideThread(t, e, e.Rep1, "thread-ids", true)
	if outcome.Messages != 1 {
		t.Fatalf("messages=%d, want 1", outcome.Messages)
	}
	if len(outcome.ActivityIDs) != 1 || outcome.ActivityIDs[0] != id {
		t.Fatalf("activity_ids=%v, want the one message the decision reached (%v)", outcome.ActivityIDs, id)
	}
}

// The same decision through the real HTTP surface, because the handler builds
// its response field by field and a field it does not name reaches the client
// as null.
//
// That is exactly how activity_ids shipped broken for a day: the setter filled
// it, the response literal did not copy it, and the frontend test stubbed a
// correct body so nothing failed. A test that decodes what the handler
// actually wrote is the only one that can see it.
func TestTheThreadAudienceResponseCarriesEveryFieldItPromises(t *testing.T) {
	e := integration.Setup(t)
	id := seedHeldThreadOn(t, e, "thread-wire", "kunde@example.test", e.Rep1)

	srv := Server{threadAudience: NewThreadAudienceSetter(e.Pool)}
	req := httptest.NewRequest(http.MethodPost,
		"/v1/activities/threads/thread-wire/audience",
		strings.NewReader(`{"share":true}`)).WithContext(purgeCtx(e, e.Rep1))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.SetThreadAudience(rec, req, "thread-wire")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}

	var got crmcontracts.ThreadAudienceOutcome
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Messages != 1 || !got.Shared {
		t.Errorf("messages=%d shared=%v, want 1 and true", got.Messages, got.Shared)
	}
	if len(got.ActivityIds) != 1 || ids.UUID(got.ActivityIds[0]) != id {
		t.Errorf("activity_ids=%v on the wire, want the message the decision reached (%v)", got.ActivityIds, id)
	}
}
