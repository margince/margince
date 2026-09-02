// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Re-opening the mail a counterparty hold caught — the only widening path in a
// derivation that is otherwise tighten-only.
//
// It exists because a hold placed by mistake was permanent: lifting it
// deliberately widens nothing, and no other path reaches the mail it already
// caught. What makes it safe is not the widening itself but what it REFUSES,
// so most of this file is refusals: a colleague's hold on the same message, a
// message that also matched the confidential marker, and a message captured
// before the product recorded more than one reason.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSharingHistoryReleasesWhatOnlyThisSeatsHoldCaught(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-1@studiolegal.example", ""))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Fatalf("the hold gave %q / %q, want participants / counterparty — nothing to re-open", got, reason)
	}

	if released := shareHistory(t, e, e.Rep1); released != 1 {
		t.Fatalf("released %d imports, want 1", released)
	}
	if got, reason := audienceOf(t, e, activityID); got != "workspace" || reason != "" {
		t.Errorf("after sharing history the message is %q / %q, want workspace with no reason: "+
			"a hold lifted by its own author left the mail it caught permanently held",
			got, reason)
	}
}

func TestSharingHistoryLeavesAMessageAColleagueAlsoHolds(t *testing.T) {
	// A hold is a seat's own decision, and re-opening it is that seat's to make
	// — but only over their own imports. The derivation still takes the
	// strictest answer across every import row, so a colleague's hold on the
	// same message has to survive one seat changing their mind.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	holdCounterparty(t, e, e.Rep3, capturemod.HoldKindDomain, "studiolegal.example")
	msg := email("anwalt@studiolegal.example", "Dr. Legal",
		captureOwner+", "+secondSeatAddress, "widen-2@studiolegal.example", "")
	sync(t, msg)
	activityID := oneActivityID(t, e)
	syncSecond(t, msg)

	shareHistory(t, e, e.Rep1)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Errorf("after one seat shared history the message is %q / %q, want it still held: "+
			"a colleague's hold was lifted by somebody else's decision", got, reason)
	}
}

func TestSharingHistoryLeavesAMessageTheSenderMarkedConfidential(t *testing.T) {
	// The case the whole provenance change was built for. The birth ladder
	// checks the counterparty hold BEFORE the subject marker, so before
	// verdict_reasons existed this message recorded 'counterparty' alone and a
	// widening keyed on that reason would have published a message the sender
	// explicitly asked us not to share.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, markedEmail("anwalt@studiolegal.example", captureOwner,
		"widen-3@studiolegal.example", "[Vertraulich] Aufhebungsvertrag"))
	activityID := oneActivityID(t, e)

	// Both reasons are recorded, and the FIRST still decided the audience.
	reasons := importReasons(t, e, activityID, e.Rep1)
	if len(reasons) != 2 || reasons[0] != "counterparty" || reasons[1] != "explicitly_confidential" {
		t.Fatalf("import reasons = %v, want [counterparty explicitly_confidential]: the ladder "+
			"stopped at the first match, so the marker is unrecorded and unprovable", reasons)
	}

	if released := shareHistory(t, e, e.Rep1); released != 0 {
		t.Errorf("released %d imports, want 0", released)
	}
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Errorf("a message the sender marked confidential became %q when its counterparty "+
			"hold was lifted", got)
	}
}

func TestSharingHistoryLeavesAMessageCapturedBeforeReasonsWereRecorded(t *testing.T) {
	// A pre-migration row records the first rule that matched and says nothing
	// about the rest, so it is not PROVABLY counterparty-only. Refusing it costs
	// a bulk re-open of old mail; admitting it would guess, in the direction
	// that discloses.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-4@studiolegal.example", ""))
	activityID := oneActivityID(t, e)
	forgetImportReasons(t, e, activityID)

	if released := shareHistory(t, e, e.Rep1); released != 0 {
		t.Errorf("released %d imports, want 0: a row whose reasons are unknown was treated "+
			"as counterparty-only", released)
	}
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Errorf("a message with no recorded reasons became %q", got)
	}
}

func TestSharingHistoryLeavesAnotherSeatsOwnImports(t *testing.T) {
	// Whose mail a person keeps private is itself private, so this operation
	// has no id and no admin arm. The seat scoping is what enforces that: one
	// seat sharing their history must not touch a message only a colleague
	// imported.
	env := newCaptureEnv(t)
	e := env.e
	syncSecond := secondMailbox(t, e, e.Rep3)
	allowSharedPosture(t, e)

	holdCounterparty(t, e, e.Rep3, capturemod.HoldKindDomain, "studiolegal.example")
	syncSecond(t, email("anwalt@studiolegal.example", "Dr. Legal", secondSeatAddress,
		"widen-5@studiolegal.example", ""))
	activityID := oneActivityID(t, e)

	// Rep1 has no imports at all, so their pass must claim nothing.
	if released := shareHistory(t, e, e.Rep1); released != 0 {
		t.Errorf("a seat released %d imports they never made, want 0", released)
	}
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Errorf("another seat's held import became %q / %q", got, reason)
	}
}

func TestSharingHistoryLeavesAMessageAHeldMailboxTookIn(t *testing.T) {
	// The mailbox's own posture is a standing instruction that outlives any one
	// counterparty hold. Before the ladder recorded it, decideBirthTx returned
	// at the counterparty rung and never asked mailboxPostureTx — so a `held`
	// mailbox's mail recorded 'counterparty' alone, and lifting the hold
	// published mail from a mailbox that never asked for its mail to be shared.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	setPosture(t, env, e.Rep1, capturemod.PostureHeld)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-6@studiolegal.example", ""))
	activityID := oneActivityID(t, e)

	reasons := importReasons(t, e, activityID, e.Rep1)
	if len(reasons) != 2 || reasons[0] != "counterparty" || reasons[1] != "posture" {
		t.Fatalf("import reasons = %v, want [counterparty posture]: the ladder returned at the "+
			"counterparty rung, so the mailbox's own answer is unrecorded and unprovable", reasons)
	}

	if released := shareHistory(t, e, e.Rep1); released != 0 {
		t.Errorf("released %d imports, want 0", released)
	}
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Errorf("mail from a held mailbox became %q when a counterparty hold was lifted", got)
	}
}

func TestSharingHistoryLeavesAMessageOnAThreadAlreadyHeld(t *testing.T) {
	// An inherited verdict is NOT opening-only: inheritedVerdictTx returns held,
	// unsure and held_by_owner for any sender at all. Before the ladder recorded
	// it, a message on an already-held thread that also matched a counterparty
	// hold recorded 'counterparty' alone — and lifting the hold discarded the
	// only record that the conversation was held anyway.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// The owner holds the thread themselves, which is the verdict a later
	// message on it inherits whoever sent it.
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-7a@studiolegal.example", ""))
	first := oneActivityID(t, e)
	holdThreadByOwner(t, e, e.Rep1, first)

	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-7b@studiolegal.example", "widen-7a@studiolegal.example"))

	second := newestActivityID(t, e)
	reasons := importReasons(t, e, second, e.Rep1)
	if len(reasons) < 2 || reasons[len(reasons)-1] != "inherited_verdict" {
		t.Fatalf("import reasons = %v, want the inherited verdict recorded after the "+
			"counterparty hold", reasons)
	}

	if released := shareHistory(t, e, e.Rep1); released != 0 {
		t.Errorf("released %d imports, want 0: a message on a thread the owner already held "+
			"was published by lifting a counterparty hold", released)
	}
}

func TestACounterpartyHoldSurvivesAnOpeningVerdictOnTheSameMessage(t *testing.T) {
	// An opening verdict opens only a message nothing else held. The ladder
	// evaluates every rung rather than returning at the first, so a message a
	// counterparty hold already caught now reaches inheritedVerdictTx too — and
	// a thread cleared for this sender answers `cleared`. Reading that verdict
	// before the posture would publish the message the hold was placed to stop.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-8a@studiolegal.example", ""))
	first := oneActivityID(t, e)
	clearThreadForSender(t, e, e.Rep1, first, "anwalt@studiolegal.example")

	// The hold arrives after the thread was cleared, which is the ordinary case:
	// a seat decides mid-conversation that this correspondent is private.
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")
	sync(t, email("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"widen-8b@studiolegal.example", "widen-8a@studiolegal.example"))

	second := newestActivityID(t, e)
	if got, _ := audienceOf(t, e, second); got != "participants" {
		t.Fatalf("a message under a counterparty hold was born %q because its thread "+
			"carried an opening verdict", got)
	}
}

// shareHistory runs the widening as one seat and returns how many messages it
// re-opened.
//
// Through the real store method, with the real seams compose injects: the
// clearing of the row-level hold is the half a test doing its own UPDATE would
// silently skip, and skipping it leaves every row re-pinned by the recompute.
func shareHistory(t *testing.T, e *integration.SearchEnv, seat ids.UUID) int {
	t.Helper()
	store := capturemod.NewCounterpartyHoldStore(e.DB())
	reopened, err := store.ShareHistory(seatContext(e, seat),
		activities.RecomputeAudienceTx, activities.ClearCounterpartyHoldTx)
	if err != nil {
		t.Fatalf("sharing a hold's history: %v", err)
	}
	return reopened
}

// importReasons reads the reasons one seat's import row recorded.
func importReasons(t *testing.T, e *integration.SearchEnv, activityID, seat ids.UUID) []string {
	t.Helper()
	var reasons []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT verdict_reasons FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
			activityID, seat).Scan(&reasons)
	}); err != nil {
		t.Fatalf("reading the import reasons: %v", err)
	}
	return reasons
}

// forgetImportReasons puts an import row back into the shape it had before
// verdict_reasons existed: the deciding reason, and nothing about the rest.
func forgetImportReasons(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_import SET verdict_reasons = NULL WHERE activity_id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("forgetting the import reasons: %v", err)
	}
}

// holdThreadByOwner records the owner's own hold on a thread, which is the
// verdict a later message on it inherits whoever sent it. Written directly
// because the product path for it is the Senders surface, and what this
// scenario needs is the ledger state, not that surface's own gate.
func holdThreadByOwner(t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, seen_addresses)
			SELECT a.thread_key, $2, 'held_by_owner', ARRAY[]::text[]
			  FROM activity a WHERE a.id = $1
			ON CONFLICT (thread_key, user_id) DO UPDATE SET status = 'held_by_owner'`,
			activityID, seat)
		return err
	}); err != nil {
		t.Fatalf("holding the thread as its owner: %v", err)
	}
}

// clearThreadForSender settles a thread as ordinary with the sender on record,
// so a later message from that address inherits `cleared` rather than re-opening
// the ledger — the state under which an opening verdict actually applies.
func clearThreadForSender(t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID, from string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, seen_addresses)
			SELECT a.thread_key, $2, 'cleared', ARRAY[$3]::text[]
			  FROM activity a WHERE a.id = $1
			ON CONFLICT (thread_key, user_id) DO UPDATE
			  SET status = 'cleared', seen_addresses = ARRAY[$3]::text[]`,
			activityID, seat, from)
		return err
	}); err != nil {
		t.Fatalf("clearing the thread for its sender: %v", err)
	}
}

// newestActivityID is oneActivityID's sibling for a scenario that captures more
// than one message: the most recent by occurrence.
func newestActivityID(t *testing.T, e *integration.SearchEnv) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the newest activity: %v", err)
	}
	return id
}

// markedEmail is email() with a subject the scenario chooses, which is what the
// confidential marker reads.
func markedEmail(from, to, msgID, subject string) []byte {
	return []byte(strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain",
		"",
		"hello",
		"",
	}, "\r\n"))
}
