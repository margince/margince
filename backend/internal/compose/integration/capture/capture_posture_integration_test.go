// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Born held, through the production sink.
//
// The decision order is the subject: every rule that can only tighten runs
// before the one that can loosen, so a message that should be held was never
// anything else. These drive the real registry, so what they prove is the
// audience a row is BORN with, not one a later pass corrected it to.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	activitiesmod "github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// setPosture puts the acting seat's mailbox in one posture, through the real
// registry, so the fixture exercises the gate the product ships.
func setPosture(t *testing.T, env captureEnv, user ids.UUID, posture string) {
	t.Helper()
	if _, err := env.registry.SetMailPosture(seatContext(env.e, user), "gmail", posture, false); err != nil {
		t.Fatalf("setting the mailbox posture to %s: %v", posture, err)
	}
}

// allowSharedPosture is the admin turning on the workspace opt-in.
func allowSharedPosture(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	on := true
	store := capturemod.NewSettings(composeSettingsStore(e))
	if _, err := store.Update(ctx, capturemod.SettingsPatch{SharedPostureAllowed: &on}); err != nil {
		t.Fatalf("turning on the shared-posture opt-in: %v", err)
	}
}

// postureOf reads what one seat's mailbox currently asks for.
func postureOf(t *testing.T, e *integration.SearchEnv, user ids.UUID) string {
	t.Helper()
	var posture string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT mail_posture FROM capture_connection WHERE user_id = $1 AND archived_at IS NULL`,
			user).Scan(&posture)
	}); err != nil {
		t.Fatalf("reading the mailbox posture: %v", err)
	}
	return posture
}

func TestAConnectionIsBornClassified(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e

	// Nothing asked for this. The column's DEFAULT is the whole safety
	// property: an older binary, a missed insert path or a future caller that
	// forgets the column all produce a HELD mailbox, never a shared one.
	if got := postureOf(t, e, e.Rep1); got != capturemod.PostureClassified {
		t.Fatalf("a fresh connection is %q, want classified — a default that shares is a leak nobody chose", got)
	}
}

func TestAClassifiedMailboxBirthsEveryMessageHeld(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	sync(t, customerMail("classified-1@acme.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "posture" {
		t.Fatalf("a classified mailbox births %q / %q, want participants / posture", got, reason)
	}
}

func TestASharedMailboxNeedsTheWorkspaceOptIn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	// Refused while the workspace has not opted in. Not a 403: the seat's
	// authority over their own mailbox is not in question, the workspace's
	// posture is.
	_, err := env.registry.SetMailPosture(seatContext(e, e.Rep1), "gmail", capturemod.PostureShared, false)
	var refusal *capturemod.SharedPostureNotAllowedError
	if !errors.As(err, &refusal) {
		t.Fatalf("asking for shared without the opt-in gave %v, want the opt-in refusal", err)
	}
	if got := postureOf(t, e, e.Rep1); got != capturemod.PostureClassified {
		t.Fatalf("a refused request moved the posture to %q", got)
	}

	// And admitted once an admin turns it on.
	allowSharedPosture(t, e)
	if _, err := env.registry.SetMailPosture(seatContext(e, e.Rep1), "gmail", capturemod.PostureShared, false); err != nil {
		t.Fatalf("asking for shared WITH the opt-in: %v", err)
	}
	if got := postureOf(t, e, e.Rep1); got != capturemod.PostureShared {
		t.Fatalf("the opt-in was on and the posture is %q", got)
	}
}

func TestNarrowingAMailboxNeverNeedsTheOptIn(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	// A seat may always ask for MORE privacy than the workspace requires.
	// Making them ask an admin first would be a product that argues with
	// somebody protecting their own mail.
	for _, posture := range []string{capturemod.PostureHeld, capturemod.PostureClassified} {
		if _, err := env.registry.SetMailPosture(seatContext(e, e.Rep1), "gmail", posture, false); err != nil {
			t.Fatalf("narrowing to %s without the opt-in: %v", posture, err)
		}
	}
}

func TestASharedMailboxBirthsAMessageOpen(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, customerMail("shared-open-1@acme.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "workspace" || reason != "" {
		t.Fatalf("a shared mailbox births %q / %q, want workspace with no reason", got, reason)
	}
}

// TestAnOwnerHoldOnAMessageBornOpenStillShowsItsEvidence is margince#4183: a
// thread a SHARED mailbox births open never runs EnsureTx, so its ledger row
// (if one exists at all) starts with no first_activity_id — and DecideAsOwner
// used to leave it that way. heldthreadslist's existence check joins on that
// column, so an owner who held such a thread was told their own intact
// message had been erased.
func TestAnOwnerHoldOnAMessageBornOpenStillShowsItsEvidence(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	sync(t, customerMail("owner-hold-open-1@acme.example"))
	activityID := oneActivityID(t, e)
	var threadKey string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, activityID).Scan(&threadKey)
	}); err != nil {
		t.Fatalf("reading the thread to hold: %v", err)
	}

	// The hold path, not the share one: this thread never went through
	// EnsureTx (a shared mailbox births its messages open, with no verdict
	// row at all), so DecideAsOwner is the row's very first writer.
	if _, err := compose.NewThreadAudienceSetter(e.Pool).Decide(sharingContext(e, e.Rep1), threadKey, false); err != nil {
		t.Fatalf("holding the thread as its owner: %v", err)
	}

	held, err := capturemod.HeldThreadsFor(seatContext(e, e.Rep1), e.DB())
	if err != nil {
		t.Fatalf("listing held threads: %v", err)
	}
	if len(held) != 1 {
		t.Fatalf("want exactly one held thread, got %d", len(held))
	}
	if !held[0].HasActivity {
		t.Fatalf("an owner's hold on a message born open reads HasActivity=false, " +
			"want true: the message is intact, only its ledger row was")
	}
	if held[0].ActivityID == nil || *held[0].ActivityID != activityID {
		t.Fatalf("held thread activity id = %v, want %s", held[0].ActivityID, activityID)
	}
}

func TestTheWorkspaceFloorOutranksASharedMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	setMailSharing(t, e, false)

	// The floor runs FIRST, before anything a mailbox asks. A workspace that
	// turned sharing off has said something about every mailbox in it.
	sync(t, customerMail("floor-over-shared@acme.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "workspace_floor" {
		t.Fatalf("the floor let a shared mailbox through: %q / %q", got, reason)
	}
}

func TestAConfidentialMarkerHoldsTheMessageInsideTheCaptureTransaction(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// A shared mailbox, so nothing else here holds anything. The sender's own
	// subject line is the only thing standing between this message and every
	// colleague — and it holds it at INSERT, not in a later pass, so there is no
	// window in which the row was workspace-readable.
	sync(t, emailWithSubject("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"marker-1@studiolegal.example", "[Vertraulich] Aufhebungsvertrag"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("a marked message is %q / %q, want participants / explicitly_confidential", got, reason)
	}

	// And the audit trail of the capture never says it was anything else: one
	// create row, and no update moving the audience.
	var moves int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'`,
			activityID).Scan(&moves)
	}); err != nil {
		t.Fatalf("counting audience moves: %v", err)
	}
	if moves != 0 {
		t.Fatalf("%d audience updates on a message that should have been born held — it was open first", moves)
	}
}

func TestACounterpartyHoldBirthsTheMessageHeld(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "studiolegal.example")

	// The hold is about the correspondent, not the message: whatever this one
	// says, this seat has decided their mail with that party is nobody else's.
	sync(t, customerMailFrom("partner@studiolegal.example", "hold-1@studiolegal.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Fatalf("a held counterparty's mail is %q / %q, want participants / counterparty", got, reason)
	}
}

func TestAnAddressHoldCoversThatAddressAndNotItsDomain(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// An ADDRESS hold, not a domain one. Holding one person at a firm says
	// nothing about their colleagues — a seat holding their own lawyer has not
	// asked to hide every message from that firm's billing desk.
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindAddress, "anwalt@bigfirm.example")

	sync(t, customerMailFrom("anwalt@bigfirm.example", "addr-held@bigfirm.example"))
	if got, reason := audienceOf(t, e, activityIDOf(t, e, "addr-held@bigfirm.example")); got != "participants" || reason != "counterparty" {
		t.Fatalf("the held address gave %q / %q, want participants / counterparty", got, reason)
	}

	sync(t, customerMailFrom("billing@bigfirm.example", "addr-open@bigfirm.example"))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "addr-open@bigfirm.example")); got != "workspace" {
		t.Fatalf("an address hold reached the whole domain: a colleague at the same firm is %q", got)
	}
}

func TestACounterpartyHoldIsOneSeatsAlone(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// The OTHER seat holds that domain. That says nothing about this seat's
	// mail — a workspace-wide hold would let anyone keep a colleague's customer
	// out of the shared CRM by naming their domain.
	holdCounterparty(t, e, e.Rep3, capturemod.HoldKindDomain, "studiolegal.example")

	sync(t, customerMailFrom("partner@studiolegal.example", "otherhold-1@studiolegal.example"))
	activityID := oneActivityID(t, e)
	if got, _ := audienceOf(t, e, activityID); got != "workspace" {
		t.Fatalf("a colleague's hold held this seat's mail: %q", got)
	}
}

func TestACaptureWithNoIdentifiableConnectionIsBornHeld(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// Archive the connection out from under the sync. The seat can no longer be
	// asked what they want, and a message whose provenance the product cannot
	// establish is the last one to publish on a guess.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET archived_at = now() WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("archiving the connection: %v", err)
	}

	sync(t, customerMail("noconn-1@acme.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "posture" {
		t.Fatalf("a capture with no identifiable mailbox is %q / %q, want participants", got, reason)
	}
}

func TestACalendarEventFollowsTheWorkspaceDefaultNotTheMailPosture(t *testing.T) {
	env := newCaptureEnv(t)
	e, syncAsKind := env.e, env.syncAsKind
	setPosture(t, env, e.Rep1, capturemod.PostureHeld)

	// A held MAILBOX says what the seat asks of their MAIL. A meeting is not
	// correspondence a mailbox posture was ever asked about, and holding it on
	// the strength of one would empty the shared calendar for a reason nobody
	// stated.
	syncAsKind(t, map[string]string{"cal-1@acme.example": "meeting"},
		customerMail("cal-1@acme.example"))
	var activityID ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE kind = 'meeting'`).Scan(&activityID)
	}); err != nil {
		t.Fatalf("reading the captured meeting: %v", err)
	}
	if got, _ := audienceOf(t, e, activityID); got != "workspace" {
		t.Fatalf("a calendar event under a held mailbox is %q, want workspace", got)
	}
}

// emailWithSubject is email() with the subject line the scenario is about. The
// shared builder hard-codes one, and the subject is exactly what the
// confidential marker reads.
func emailWithSubject(from, fromName, to, msgID, subject string) []byte {
	fromHeader := from
	if fromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, from)
	}
	return []byte(strings.Join([]string{
		"From: " + fromHeader,
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

// customerMailFrom is customerMail with the sender the scenario needs.
func customerMailFrom(from, msgID string) []byte {
	return email(from, "", captureOwner, msgID, "")
}

// holdCounterparty records one seat's decision that a party's mail is nobody
// else's, through the real store.
func holdCounterparty(t *testing.T, e *integration.SearchEnv, user ids.UUID, kind, value string) {
	t.Helper()
	store := capturemod.NewCounterpartyHoldStore(e.DB())
	if _, err := store.Add(seatContext(e, user), kind, value); err != nil {
		t.Fatalf("holding %s %q: %v", kind, value, err)
	}
}

// composeSettingsStore is the settings store the capture settings sit on.
func composeSettingsStore(e *integration.SearchEnv) *settings.Store {
	return compose.NewSettingsStore(e.Pool)
}

func TestApplyingAPostureToHistoryNarrowsOnlyBackwards(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// Three messages captured while the mailbox was open.
	for _, id := range []string{"hist-1@acme.example", "hist-2@acme.example", "hist-3@acme.example"} {
		sync(t, customerMail(id))
	}
	if n := openActivityCount(t, e); n != 3 {
		t.Fatalf("%d open messages before the change, want 3", n)
	}

	// Narrowing WITH apply_to_history reaches back over them.
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), "gmail", capturemod.PostureHeld, true); err != nil {
		t.Fatalf("narrowing with apply_to_history: %v", err)
	}
	if n := openActivityCount(t, e); n != 0 {
		t.Fatalf("%d messages are still open after applying the posture to history", n)
	}

	// And widening back does NOT reach: the messages were held for reasons that
	// were true when they landed, and a posture change is not a review of them.
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), "gmail", capturemod.PostureShared, true); err != nil {
		t.Fatalf("widening with apply_to_history: %v", err)
	}
	if n := openActivityCount(t, e); n != 0 {
		t.Fatalf("widening re-opened %d messages of history; only narrowing reaches back", n)
	}
}

// openActivityCount is how many captured emails colleagues can currently read.
func openActivityCount(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity WHERE kind = 'email' AND audience = 'workspace'`).Scan(&n)
	}); err != nil {
		t.Fatalf("counting open messages: %v", err)
	}
	return n
}

func TestNarrowingToClassifiedNeverLoosensAHeldMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	sync(t, customerMail("ratchet-1@acme.example"))
	activityID := oneActivityID(t, e)

	// Held over the whole history.
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), "gmail", capturemod.PostureHeld, true); err != nil {
		t.Fatalf("holding the history: %v", err)
	}
	if got := importPostureOf(t, e, activityID, e.Rep1); got != capturemod.PostureHeld {
		t.Fatalf("the import row is %q after holding the history, want held", got)
	}

	// Now `classified`, also with apply_to_history. Classified is LOOSER than
	// held — a verdict can clear it — so a row already held must be left alone.
	// Reaching it would be a widening wearing a narrowing's name, and the batch
	// predicate is the only thing standing in the way.
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), "gmail", capturemod.PostureClassified, true); err != nil {
		t.Fatalf("applying classified to history: %v", err)
	}
	if got := importPostureOf(t, e, activityID, e.Rep1); got != capturemod.PostureHeld {
		t.Fatalf("applying classified to history loosened a held row to %q", got)
	}
}

// importPostureOf reads what one seat's import row records for a message.
func importPostureOf(t *testing.T, e *integration.SearchEnv, activityID, user ids.UUID) string {
	t.Helper()
	var posture *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT posture_at_import FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
			activityID, user).Scan(&posture)
	}); err != nil {
		t.Fatalf("reading the import row's posture: %v", err)
	}
	if posture == nil {
		return ""
	}
	return *posture
}

func TestASecondMailboxDoesNotLendItsPostureToTheFirst(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)

	// The seat's gmail — the one the syncs below go through — is HELD.
	setPosture(t, env, e.Rep1, capturemod.PostureHeld)

	// The same seat then connects a second, SHARED mailbox on another provider.
	// It is newer, and it delivers none of this seat's gmail. A posture read
	// that asked for "this seat's newest connection" would answer `shared` and
	// publish mail the held mailbox brought in.
	second := &mailBatchConnector{accountLabel: "team@myco.example", name: "graph"}
	secondRegistry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	secondRegistry.Register(second)
	if _, err := secondRegistry.Connect(
		humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}),
		"graph", connector.Auth("refresh")); err != nil {
		t.Fatalf("connecting the second mailbox: %v", err)
	}
	if _, err := secondRegistry.SetMailPosture(
		seatContext(e, e.Rep1), "graph", capturemod.PostureShared, false); err != nil {
		t.Fatalf("sharing the second mailbox: %v", err)
	}

	sync(t, customerMail("twobox-1@acme.example"))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("a second, shared mailbox lent its posture to the one that delivered: %q / %q", got, reason)
	}
}

func TestARebindResetsThePostureToHeld(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// The same provider, reconnected against a DIFFERENT account. The seat
	// chose `shared` for a mailbox that is now gone — a role inbox, say — and
	// the account behind this row is somebody's personal mail. Inheriting the
	// answer would publish that account's mail on arrival, with nobody asked.
	env.conn.accountLabel = "personal@myco.example"
	if _, err := env.registry.Connect(
		humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}),
		"gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("rebinding the mailbox: %v", err)
	}

	if got := postureOf(t, e, e.Rep1); got != capturemod.PostureHeld {
		t.Fatalf("a rebind left the posture at %q; the previous mailbox's answer is not this one's to inherit", got)
	}
}

func TestACounterpartyHoldSurvivesAnotherSeatsClearedVerdict(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// The OPEN mailbox syncs first, so the row is born workspace with no reason
	// on it. The holding seat's sync then hits the incumbent, and their hold
	// lives only on their own import row — the activity row never carried it.
	msg := email("anwalt@studiolegal.example", "Dr. Legal",
		captureOwner+", "+secondSeatAddress, "hold-survives-1@studiolegal.example", "")
	sync(t, msg)
	activityID := oneActivityID(t, e)

	holdCounterparty(t, e, e.Rep3, capturemod.HoldKindDomain, "studiolegal.example")
	syncSecond(t, msg)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Fatalf("the second seat's hold gave %q / %q, want participants / counterparty", got, reason)
	}

	// Now a verdict clears the thread for BOTH seats, and the row's own reason
	// is overwritten by something weaker on the way — which is what happens
	// whenever another contributor's answer wins the derivation. The hold then
	// survives only if contributionOf reads it off the holding seat's import
	// row; reading the activity row alone is not enough, because that row is
	// exactly what a later derivation rewrites.
	clearRowReason(t, e, activityID)
	setImportDecision(t, e, activityID, e.Rep1, "shared", "cleared")
	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "counterparty" {
		t.Fatalf("a colleague's cleared verdict left %q / %q, want participants / counterparty", got, reason)
	}

	// The reason matters as much as the audience. `posture` is documented as
	// clearable by a verdict and `counterparty` as not, so a row that ends up
	// saying `posture` opens the moment the HOLDING seat's own thread is
	// judged ordinary — the hold would be gone with nothing to show it ever
	// existed.
	// The damaging case: a classifier judges the HOLDING seat's own thread
	// ordinary. Their import row then carries both the hold that made it held
	// and a verdict that would open it, and only one of them can win. A
	// verdict speaks to the conversation; the hold speaks to the correspondent,
	// and nothing about a thread being ordinary makes a seat want their
	// lawyer's mail in a shared CRM.
	setImportVerdictKeepingPosture(t, e, activityID, e.Rep3, "cleared")
	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("a cleared verdict published mail the holding seat's own hold kept back: %q / %q", got, reason)
	}
}

// clearRowReason wipes the activity's own audience_reason, leaving the truth
// only on the import rows. That is the state a derivation reaches whenever a
// different contributor's answer wins, and the state a hold has to survive.
func clearRowReason(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience_reason = NULL WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("clearing the row reason: %v", err)
	}
}

// setImportVerdictKeepingPosture records a verdict on a seat's import row
// WITHOUT disturbing the posture and reason the capture wrote. That is the state
// the classifier reaches: it judges a thread and writes its answer beside what
// the capture already decided, rather than replacing it.
func setImportVerdictKeepingPosture(t *testing.T, e *integration.SearchEnv, activityID, user ids.UUID, status string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_import SET verdict_status = $3 WHERE activity_id = $1 AND user_id = $2`,
			activityID, user, status)
		return err
	}); err != nil {
		t.Fatalf("recording the verdict: %v", err)
	}
}

func TestAClearedThreadIsInheritedOnlyBySendersTheVerdictSaw(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)

	// A CLASSIFIED mailbox, so nothing opens a message except a verdict on its
	// thread. That is what makes this test about inheritance rather than about
	// the posture.
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)

	// A thread the classifier cleared, having seen exactly one address on it.
	// (Seeded directly: the engine that writes verdicts is a later change, and
	// this is the rule it will have to satisfy.)
	seedThreadVerdict(t, e, e.Rep1, "thread-root@acme.example", "cleared", []string{"buyer@acme.example"})

	// A reply from the address the verdict saw takes its answer.
	sync(t, email("buyer@acme.example", "Buyer", captureOwner,
		"seen-reply@acme.example", "thread-root@acme.example"))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "seen-reply@acme.example")); got != "workspace" {
		t.Fatalf("a reply from an address the verdict SAW is %q, want workspace", got)
	}

	// A message from an address it never saw does not — same thread key, same
	// subject line, entirely different correspondent. Inheriting by domain here
	// is how a settled customer thread would carry their lawyer's first message
	// into a shared timeline.
	sync(t, email("anwalt@acme.example", "Their Counsel", captureOwner,
		"unseen-reply@acme.example", "thread-root@acme.example"))
	if got, _ := audienceOf(t, e, activityIDOf(t, e, "unseen-reply@acme.example")); got != "participants" {
		t.Fatalf("a message from an address the verdict never saw is %q, want participants", got)
	}

	// And the thread is re-opened for the classifier to look at again, rather
	// than left carrying an answer this message just contradicted.
	if got := threadVerdictStatus(t, e, e.Rep1, "thread-root@acme.example"); got != "pending" {
		t.Fatalf("the ledger is %q after an unseen sender, want pending", got)
	}
}

// seedThreadVerdict writes what a classifier concluded about one thread for one
// seat. The engine that writes these lands with the classifier; this is the
// shape it must produce, asserted now so that change inherits a spec.
func seedThreadVerdict(t *testing.T, e *integration.SearchEnv, user ids.UUID, threadKey, status string, seen []string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, seen_addresses, resolved_at)
			VALUES ($1, $2, $3, $4, now())`, threadKey, user, status, seen)
		return err
	}); err != nil {
		t.Fatalf("seeding the thread verdict: %v", err)
	}
}

// threadVerdictStatus reads one seat's ledger row for a thread.
func threadVerdictStatus(t *testing.T, e *integration.SearchEnv, user ids.UUID, threadKey string) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM capture_thread_verdict WHERE thread_key = $1 AND user_id = $2`,
			threadKey, user).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the thread verdict: %v", err)
	}
	return status
}

// TestAClassifierVerdictIsNotASendersMarking holds apart the two things that
// once shared the word `explicitly_confidential`.
//
// A SENDER's subject-line marking is row-carried and survives an opening
// verdict: a person marked that message, and no model and no later pass
// overrules a person. A CLASSIFIER's answer of the same name is a judgement,
// and the seat whose mail it is may disagree with it.
//
// Sharing the word made the second behave like the first. The reported symptom:
// an ordinary deal thread was judged confidential, and the rep pressed Share.
// The ledger recorded shared_by_owner, the derivation ran, read
// `explicitly_confidential` off the row, treated it as the sender's own marking
// and left the message held. The share worked and the hold ignored it, with
// nothing on screen saying why.
//
// Both directions are asserted, because fixing one alone is the regression: a
// change that made every hold clearable would publish the messages a sender
// explicitly marked.
func TestAClassifierVerdictIsNotASendersMarking(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)

	// A classifier's hold, on a subject carrying no marker at all. The kind is
	// the model's own `explicitly_confidential`, which is what the shipped
	// classifier returned on the reported thread.
	sync(t, emailWithSubject("john@mii.example", "John Khoury", captureOwner,
		"verdict-share-1@mii.example", "Re: Follow up"))
	judged := oneActivityID(t, e)
	recordThreadVerdict(t, e, e.Rep1, judged, capturemod.VerdictHeld, "explicitly_confidential")
	if got, reason := audienceOf(t, e, judged); got != "participants" {
		t.Fatalf("a held message is %q / %q, want it held before the share", got, reason)
	} else if reason == "explicitly_confidential" {
		t.Fatalf("a CLASSIFIER's answer reached the row as %q, the word reserved for a sender's "+
			"own subject-line marking: that word is row-carried, so the verdict became a hold "+
			"its owner cannot clear", reason)
	}

	// The owner shares it, which is the whole point of a judgement being a
	// judgement: it is theirs to disagree with.
	shareThreadAsOwner(t, e, e.Rep1, judged)
	if got, reason := audienceOf(t, e, judged); got != "workspace" {
		t.Fatalf("after the owner shared it the message is %q / %q, want workspace: "+
			"the share reached the ledger and the derivation ignored it", got, reason)
	}

	// The other direction. A sender's marking is NOT the owner's to overrule,
	// and it still is not.
	sync(t, emailWithSubject("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"verdict-share-2@studiolegal.example", "[Vertraulich] Aufhebungsvertrag"))
	marked := newestActivityID(t, e)
	if got, reason := audienceOf(t, e, marked); got != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("a marked message is %q / %q, want participants / explicitly_confidential", got, reason)
	}
	shareThreadAsOwner(t, e, e.Rep1, marked)
	if got, reason := audienceOf(t, e, marked); got != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("a message its SENDER marked is %q / %q after an owner share, want it still "+
			"held: the marking is not the recipient's to lift", got, reason)
	}
}

// recordThreadVerdict drives the real writer — the store method the
// confidentiality engine calls — rather than an INSERT of the row it produces.
// The translation under test lives inside that method, so a seeded row would
// bypass exactly the code this scenario is about.
func recordThreadVerdict(
	t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID, status, kind string,
) {
	t.Helper()
	ctx := seatContext(e, seat)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var threadKey string
		if err := tx.QueryRow(ctx,
			`SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, activityID).Scan(&threadKey); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, kind, seen_addresses, resolved_at)
			VALUES ($1, $2, $3, $4, ARRAY[]::text[], now())
			ON CONFLICT (thread_key, user_id) DO UPDATE SET status = $3, kind = $4`,
			threadKey, seat, status, kind); err != nil {
			return err
		}
		return capturemod.NewThreadVerdictStore(nil).RecordOutcomeTx(ctx, tx,
			capturemod.PendingThread{ThreadKey: threadKey, UserID: seat, ActivityID: activityID},
			status, kind)
	}); err != nil {
		t.Fatalf("recording the thread verdict: %v", err)
	}
	recomputeAudience(t, e, seat, activityID)
}

// shareThreadAsOwner presses Share the way the endpoint does: through
// ThreadAudienceSetter.Decide, which is the whole shipped path.
//
// It used to call DecideAsOwner and then the derivation by hand, and that is
// precisely what a test must not do — the assembled version left out the step
// between them, so a defect living in the seam was invisible to it while the
// product had it. The setter runs the ledger write, the hold clear and the
// recompute in one transaction, and this asks for all three.
func shareThreadAsOwner(t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID) {
	t.Helper()
	ctx := sharingContext(e, seat)
	var threadKey string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(thread_key, '') FROM activity WHERE id = $1`, activityID).Scan(&threadKey)
	}); err != nil {
		t.Fatalf("reading the thread to share: %v", err)
	}
	if _, err := compose.NewThreadAudienceSetter(e.Pool).Decide(ctx, threadKey, true); err != nil {
		t.Fatalf("sharing the thread as its owner: %v", err)
	}
}

// sharingContext is one seat with the grant the endpoint checks. Decide asks
// for activity:update before it writes anything, so a context without it would
// prove only that the gate is there.
func sharingContext(e *integration.SearchEnv, user ids.UUID) context.Context {
	ctx := seatContext(e, user)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// recomputeAudience runs the derivation the way every product writer does after
// changing a contribution.
//
// The SEAT's context, not the admin's: the derivation writes an audit row, and a
// row with no actor behind it is a change nobody made. That context is threaded
// into the call rather than a fresh Background one, which is where the actor
// lives.
func recomputeAudience(t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID) {
	t.Helper()
	ctx := seatContext(e, seat)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return activitiesmod.RecomputeAudienceTx(
			ctx, tx, ids.From[ids.ActivityKind](activityID))
	}); err != nil {
		t.Fatalf("recomputing the audience: %v", err)
	}
}

// TestAnOwnerShareLiftsAVerdictHoldAndNothingElse is the reported bug and its
// three boundaries.
//
// A row-carried hold outranks an opening contribution, which is right for the
// holds a recipient may not lift and wrong for the one they may. The owner
// pressed Share; the ledger recorded shared_by_owner, the derivation ran, read
// the verdict's own reason off the row and left the message held. The share
// worked and the hold ignored it, however many times it was pressed.
//
// The three that must NOT move are asserted beside it, because a clear that was
// one word too wide would publish mail nobody decided to publish.
func TestAnOwnerShareLiftsAVerdictHoldAndNothingElse(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureClassified)

	// A classifier held it, answering `explicitly_confidential` — the kind that
	// collides with a sender's own marking and lands on the row as the marking's
	// word. That collision is what made the share inert: the row could not say
	// which of the two held it, so it refused to let either be lifted. The
	// subject carries no marker at all, which is the whole point.
	sync(t, emailWithSubject("john@mii.example", "John Khoury", captureOwner,
		"ownershare-1@mii.example", "Re: Follow up"))
	judged := oneActivityID(t, e)
	recordThreadVerdict(t, e, e.Rep1, judged, capturemod.VerdictHeld, "explicitly_confidential")
	// The row as a binary BEFORE rowReasonForKind wrote it: the kind reached the
	// column verbatim. Written here rather than left to the current writer,
	// which translates it away — these are the rows already in the field, and
	// they are what a rep cannot share. Set through the import row and the
	// derivation, so the activity's own reason is derived rather than dictated.
	setImportVerdictReason(t, e, e.Rep1, judged, "explicitly_confidential")
	recomputeAudience(t, e, e.Rep1, judged)
	if got, _ := audienceOf(t, e, judged); got != "participants" {
		t.Fatalf("a message a verdict held is %q, want it held before the share", got)
	}
	shareThreadAsOwner(t, e, e.Rep1, judged)
	if got, reason := audienceOf(t, e, judged); got != "workspace" {
		t.Fatalf("after its owner shared it the message is %q / %q, want workspace: the share "+
			"reached the ledger and the row's own reason outranked it", got, reason)
	}

	// A SENDER's marking is not the recipient's to lift.
	sync(t, emailWithSubject("anwalt@studiolegal.example", "Dr. Legal", captureOwner,
		"ownershare-2@studiolegal.example", "[Vertraulich] Aufhebungsvertrag"))
	marked := newestActivityID(t, e)
	shareThreadAsOwner(t, e, e.Rep1, marked)
	if got, reason := audienceOf(t, e, marked); got != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("a message its SENDER marked is %q / %q after an owner share, want it still held",
			got, reason)
	}

	// A COUNTERPARTY hold is this seat's standing decision about a person.
	// Sharing one thread says nothing about whether they want that party's mail
	// in a shared CRM.
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "ownerhold.example")
	sync(t, email("partner@ownerhold.example", "Partner", captureOwner,
		"ownershare-3@ownerhold.example", ""))
	heldParty := newestActivityID(t, e)
	shareThreadAsOwner(t, e, e.Rep1, heldParty)
	if got, reason := audienceOf(t, e, heldParty); got != "participants" || reason != "counterparty" {
		t.Fatalf("a counterparty hold is %q / %q after an owner share, want it still held", got, reason)
	}

	// And the WORKSPACE floor, which one seat cannot overrule.
	setMailSharing(t, e, false)
	sync(t, customerMail("ownershare-4@acme.example"))
	floored := newestActivityID(t, e)
	shareThreadAsOwner(t, e, e.Rep1, floored)
	if got, reason := audienceOf(t, e, floored); got != "participants" || reason != "workspace_floor" {
		t.Fatalf("the workspace floor is %q / %q after an owner share, want it still held", got, reason)
	}
}

// setImportVerdictReason puts an import row back into the shape a verdict wrote
// before rowReasonForKind translated the classifier's kind away.
//
// The COLUMN is set and the derivation is left to decide what the activity row
// says about it, so the scenario still turns on production's own reading. There
// is no product path to this state any more — that is the fix — and the rows it
// reproduces are the ones already captured, which is exactly why the share has
// to work on them.
func setImportVerdictReason(
	t *testing.T, e *integration.SearchEnv, seat, activityID ids.UUID, reason string,
) {
	t.Helper()
	ctx := seatContext(e, seat)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE capture_import SET verdict_reason = $3
			 WHERE activity_id = $1 AND user_id = $2`, activityID, seat, reason)
		return err
	}); err != nil {
		t.Fatalf("restoring the pre-translation verdict reason: %v", err)
	}
}
