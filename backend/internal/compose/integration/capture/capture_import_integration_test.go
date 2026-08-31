// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Per-mailbox provenance, through the production sink.
//
// An email is stored once — activity identity is (source_system, source_id),
// and source_id is the Message-ID — so when a message reaches two seats'
// mailboxes there is one row, and captured_by names the first sync alone.
// These prove the second seat is recorded anyway, and that a message ends at
// the strictest thing any importing seat asked for whichever order the two
// syncs ran in.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// secondMailbox connects a SECOND seat's mailbox to the same workspace and
// answers a sync function for it, so a test can deliver one message to two
// mailboxes and choose which one syncs first.
//
// It builds its own registry and connector rather than reusing the env's: the
// env's connection belongs to Rep1, and the whole point here is a message
// arriving through somebody else's grant.
// secondSeatAddress is the second connected mailbox's own address. A real
// second mailbox has one; the shared fake claims the workspace owner's, which
// would make every seat look like a recipient of every message.
const secondSeatAddress = "colleague@myco.example"

func secondMailbox(t *testing.T, e *integration.SearchEnv, user ids.UUID) func(t *testing.T, raws ...[]byte) {
	t.Helper()
	seedCaptureRoleFor(t, e, user)
	conn := &mailBatchConnector{accountLabel: secondSeatAddress}
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{})
	registry.Register(conn)
	grantCtx := humanWithScopes(e, user, []principal.Scope{principal.ScopeRead})
	connID, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the second mailbox: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	return func(t *testing.T, raws ...[]byte) {
		t.Helper()
		conn.raws, conn.sent, conn.deals, conn.kinds = raws, nil, nil, nil
		if err := registry.SyncOnce(wsCtx, connID); err != nil {
			t.Fatalf("second mailbox SyncOnce: %v", err)
		}
	}
}

// seedCaptureRoleFor assigns the capture role seedCaptureRole created to a
// second seat. The role is per workspace and already exists by the time any
// test calls this, so assigning is the whole job.
func seedCaptureRoleFor(t *testing.T, e *integration.SearchEnv, user ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO role_assignment (role_id, user_id)
			SELECT id, $1 FROM role WHERE key = 'capture_rep'`, user)
		return err
	})
	if err != nil {
		t.Fatalf("assigning the capture role to the second seat: %v", err)
	}
}

// importRowsOf answers every seat that imported one activity.
func importRowsOf(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) []ids.UUID {
	t.Helper()
	var users []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT user_id FROM capture_import WHERE activity_id = $1 ORDER BY user_id`, activityID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u ids.UUID
			if err := rows.Scan(&u); err != nil {
				return err
			}
			users = append(users, u)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the import rows: %v", err)
	}
	return users
}

// audienceOf answers one activity's audience and the reason recorded with it.
func audienceOf(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) (string, string) {
	t.Helper()
	var audience string
	var reason *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT audience, audience_reason FROM activity WHERE id = $1`, activityID).
			Scan(&audience, &reason)
	})
	if err != nil {
		t.Fatalf("reading the audience: %v", err)
	}
	if reason == nil {
		return audience, ""
	}
	return audience, *reason
}

// setImportDecision writes what one seat's mailbox concluded about a message.
// The sink writes these columns in a later part of this feature; here they are
// set directly, because what is under test is the derivation over them.
func setImportDecision(t *testing.T, e *integration.SearchEnv, activityID, user ids.UUID, posture, status string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(context.Background(), `
			UPDATE capture_import
			   SET posture_at_import = NULLIF($3, ''),
			       verdict_status = NULLIF($4, '')
			 WHERE activity_id = $1 AND user_id = $2`,
			activityID, user, posture, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			t.Fatalf("no import row for seat %s on activity %s", user, activityID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("recording the import decision: %v", err)
	}
}

// oneActivityID answers the id of the single captured activity, failing when
// there is not exactly one — a message stored twice is the defect this whole
// table exists because of, so the assertion is worth making explicitly.
func oneActivityID(t *testing.T, e *integration.SearchEnv) ids.UUID {
	t.Helper()
	var found []ids.UUID
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT id FROM activity WHERE kind = 'email' ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			found = append(found, id)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("listing activities: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one captured email, got %d", len(found))
	}
	return found[0]
}

// customerMail is one ordinary message from a customer, addressed to both
// seats — the shape that reaches two mailboxes and is stored once.
func customerMail(msgID string) []byte {
	return email("anna@acme.example", "Anna Acme", captureOwner, msgID, "")
}

// customerMailToBoth is one message a customer addressed to BOTH seats — the
// shape that genuinely reaches two mailboxes and is stored once. Each seat's
// own address on it is what proves their provider delivered it to them.
func customerMailToBoth(msgID string) []byte {
	return email("anna@acme.example", "Anna Acme", captureOwner+", "+secondSeatAddress, msgID, "")
}

func TestASecondImportingMailboxIsRecordedOnAMessageStoredOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	// The same Message-ID through both mailboxes. The second sync hits the
	// incumbent and writes no activity — which is exactly when the second seat
	// used to vanish from the record entirely.
	sync(t, customerMailToBoth("shared-1@acme.example"))
	syncSecond(t, customerMailToBoth("shared-1@acme.example"))

	activityID := oneActivityID(t, e)
	importers := importRowsOf(t, e, activityID)
	if len(importers) != 2 {
		t.Fatalf("want an import row for each of the two mailboxes, got %d", len(importers))
	}
	seen := map[ids.UUID]bool{importers[0]: true, importers[1]: true}
	if !seen[e.Rep1] || !seen[e.Rep3] {
		t.Fatalf("want import rows for %s and %s, got %v", e.Rep1, e.Rep3, importers)
	}

	// And the second seat is a participant, which is how they read the message
	// once anybody's decision holds it.
	var participants int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND user_id = $2`,
			activityID, e.Rep3).Scan(&participants)
	}); err != nil {
		t.Fatalf("counting the second seat's participant rows: %v", err)
	}
	if participants == 0 {
		t.Fatal("the second importing seat is not a participant on the message their own mailbox delivered")
	}
}

func TestTheStrictestImportingMailboxDecidesTheAudience(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncSecond := secondMailbox(t, e, e.Rep3)

	sync(t, customerMailToBoth("strict-1@acme.example"))
	syncSecond(t, customerMailToBoth("strict-1@acme.example"))
	activityID := oneActivityID(t, e)

	// Both mailboxes are `classified` by default, so the message starts held
	// and says which rule holds it. Opening it takes a verdict on each seat's
	// own import row, which is what the rest of this test walks through.
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "posture" {
		t.Fatalf("two classified mailboxes should hold the message, got %q / %q", got, reason)
	}

	// One seat's mailbox holds it. One held contributor is enough.
	setImportDecision(t, e, activityID, e.Rep1, "classified", "")
	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "posture" {
		t.Fatalf("one classified mailbox should hold the message: got %q / %q", got, reason)
	}

	// The OTHER seat's mailbox saying `shared` does not widen it. This is the
	// case that has no correct answer without per-seat rows: with one column
	// and one captured_by, whichever sync ran last would win.
	setImportDecision(t, e, activityID, e.Rep3, "shared", "cleared")
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("a colleague's shared mailbox must not widen a held message: got %q", got)
	}

	// It opens only when the holding seat's own mailbox stops holding it.
	setImportDecision(t, e, activityID, e.Rep1, "classified", "cleared")
	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "workspace" || reason != "" {
		t.Fatalf("with every contributor cleared want workspace: got %q / %q", got, reason)
	}
}

func TestTheAudienceIsTheSameWhicheverMailboxSyncsFirst(t *testing.T) {
	// The ordering claim, proven by running both orders and comparing. A test
	// that ran one order would pass against a sink that simply let the last
	// sync win.
	for _, order := range []struct {
		name        string
		holderFirst bool
	}{
		{"the holding mailbox syncs first", true},
		{"the open mailbox syncs first", false},
	} {
		t.Run(order.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			e, sync := env.e, env.sync
			syncSecond := secondMailbox(t, e, e.Rep3)

			msg := customerMailToBoth("order-1@acme.example")
			if order.holderFirst {
				sync(t, msg)
				syncSecond(t, msg)
			} else {
				syncSecond(t, msg)
				sync(t, msg)
			}
			activityID := oneActivityID(t, e)

			setImportDecision(t, e, activityID, e.Rep1, "classified", "")
			setImportDecision(t, e, activityID, e.Rep3, "shared", "cleared")
			recompute(t, e, activityID)

			if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "posture" {
				t.Fatalf("want participants/posture whichever mailbox synced first, got %q / %q", got, reason)
			}
		})
	}
}

func TestARecomputeThatChangesNothingWritesNoAuditRowAndEmitsNoEvent(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, customerMail("quiet-1@acme.example"))
	activityID := oneActivityID(t, e)

	// The recompute runs on every import of every message. If it wrote
	// unconditionally, a mailbox re-syncing its backlog would put one audit row
	// and one activity.updated on the bus per message per pass.
	before := auditAndOutboxCounts(t, e, activityID)
	recompute(t, e, activityID)
	if after := auditAndOutboxCounts(t, e, activityID); after != before {
		t.Fatalf("a no-change recompute wrote %v, want %v", after, before)
	}

	// A real change writes exactly one of each. The message is already held by
	// the mailbox's own posture, so the change that MOVES it is a verdict
	// clearing the thread — setting the posture again would write nothing, which
	// is the case above rather than this one.
	setImportDecision(t, e, activityID, e.Rep1, "classified", "cleared")
	recompute(t, e, activityID)
	after := auditAndOutboxCounts(t, e, activityID)
	if after.audit != before.audit+1 || after.outbox != before.outbox+1 {
		t.Fatalf("a change should write one audit row and one event: got %v from %v", after, before)
	}
}

func TestAHumanSelectedAudienceIsNotMovedByADerivation(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, customerMail("selected-1@acme.example"))
	activityID := oneActivityID(t, e)

	// A person named a specific set of readers. No contribution below knows
	// how to rebuild that set, so a derivation that moved the row would either
	// publish what they narrowed or discard the names they chose.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'selected' WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("setting the selected audience: %v", err)
	}

	setImportDecision(t, e, activityID, e.Rep1, "shared", "cleared")
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "selected" {
		t.Fatalf("a derivation moved a human's selected audience to %q", got)
	}
}

// recompute runs the production derivation over one activity, in its own
// transaction, the way the sink runs it.
func recompute(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) {
	t.Helper()
	// A correlation id, because the recompute writes through the same audit and
	// outbox path every mutation here takes, and that path requires one — the
	// sink's own context carries it, and a fixture without one would be testing
	// a shape production never runs.
	ctx := principal.WithCorrelationID(e.Admin(), ids.NewV7())
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](activityID))
	})
	if err != nil {
		t.Fatalf("recomputing the audience: %v", err)
	}
}

// writeCounts is what one activity has recorded about itself in the two tables
// the write shape requires a mutation to touch.
type writeCounts struct{ audit, outbox int }

func auditAndOutboxCounts(t *testing.T, e *integration.SearchEnv, activityID ids.UUID) writeCounts {
	t.Helper()
	var c writeCounts
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1`,
			activityID).Scan(&c.audit); err != nil {
			return err
		}
		// The outbox stores its envelope as JSON, so the activity is named
		// inside it rather than in a column.
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM event_outbox WHERE envelope #>> '{entity,id}' = $1::text`,
			activityID).Scan(&c.outbox)
	})
	if err != nil {
		t.Fatalf("counting the audit and outbox rows: %v", err)
	}
	return c
}

func TestTheLinkLessHoldSurvivesARecompute(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	// An infrastructure notice: the ladder files it under no record and holds
	// it to its participants. No import row records that hold — it is a fact
	// about what the message is, not about whose mailbox delivered it — so a
	// derivation reading import rows alone would widen it back on the next
	// sync of the same mailbox.
	sync(t, email("dse@eu.docusign.net", "DocuSign EU", captureOwner, "linkless-1@docusign.net", ""))
	activityID := oneActivityID(t, e)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "no_record" {
		t.Fatalf("the ladder should hold a link-less message and say why: got %q / %q", got, reason)
	}

	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "participants" || reason != "no_record" {
		t.Fatalf("a recompute widened a message the ladder held: got %q / %q", got, reason)
	}
}

// TestTheTwoModulesSpellTheRowCarriedReasonsTheSameWay holds the two spellings
// of each row-carried reason together — capture's, by driving the sink and
// reading what it stamped, and activities', by comparing against its constant.
//
// capture writes audience_reason when its ladder holds a link-less message, and
// activities reads it to know that hold is not one of its own to widen. Neither
// module can import the other, so the word is written twice, and a drift on
// either side silently un-holds every suppressed newsletter: the sink stamps a
// reason the recompute does not recognise, the recompute finds no contributor
// asking for a hold, and it widens the row to the whole workspace.
//
// Asserted against the literal so the test fails from EITHER side.
func TestTheTwoModulesSpellTheRowCarriedReasonsTheSameWay(t *testing.T) {
	// capture's constants are unexported, so its side is read through what the
	// sink WRITES. Comparing that against the activities constant is the whole
	// check: either side moving alone fails it, which is the drift that matters.
	// A literal in this file would only be a third copy of the same word.
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	setMailSharing(t, e, false)
	sync(t, customerMail("mirror-floor@acme.example"))
	if _, reason := audienceOf(t, e, oneActivityID(t, e)); reason != activities.ReasonWorkspaceFloor {
		t.Fatalf("the sink stamps the workspace floor %q; activities reads %q",
			reason, activities.ReasonWorkspaceFloor)
	}

	setMailSharing(t, e, true)

	// The two reasons a per-message decision writes. Both are stamped by the
	// sink and read by the derivation, and a drift on either side un-holds the
	// mail they exist to keep back.
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	holdCounterparty(t, e, e.Rep1, capturemod.HoldKindDomain, "mirrorlegal.example")
	sync(t, customerMailFrom("partner@mirrorlegal.example", "mirror-hold@mirrorlegal.example"))
	if _, reason := audienceOf(t, e, activityIDOf(t, e, "mirror-hold@mirrorlegal.example")); reason != activities.ReasonCounterparty {
		t.Fatalf("the sink stamps a counterparty hold %q; activities reads %q", reason, activities.ReasonCounterparty)
	}

	sync(t, emailWithSubject("anwalt@other.example", "Dr. Legal", captureOwner,
		"mirror-marker@other.example", "[Vertraulich] Vertrag"))
	if _, reason := audienceOf(t, e, activityIDOf(t, e, "mirror-marker@other.example")); reason != activities.ReasonConfidentialMarker {
		t.Fatalf("the sink stamps a confidential marker %q; activities reads %q", reason, activities.ReasonConfidentialMarker)
	}

	sync(t, email("dse@eu.docusign.net", "DocuSign EU", captureOwner, "mirror-linkless@docusign.net", ""))
	var linkLess ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_id = $1`, "mirror-linkless@docusign.net").Scan(&linkLess)
	}); err != nil {
		t.Fatalf("reading the link-less activity: %v", err)
	}
	if _, reason := audienceOf(t, e, linkLess); reason != activities.ReasonNoRecord {
		t.Fatalf("the sink stamps the link-less hold %q; activities reads %q",
			reason, activities.ReasonNoRecord)
	}
}

func TestAForgedMessageIDBuysNoAccessToAColleaguesHeldMail(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncOther := secondMailbox(t, e, e.Rep3)

	// A message identity is (source_system, source_id) and source_id is the
	// RFC822 Message-ID — a header the SENDER types. So any seat can mint a mail
	// carrying a colleague's Message-ID and sync it, and the capture will hit
	// the incumbent row rather than storing anything.
	//
	// What must not follow is an import row: the audience arm and the write
	// authority both read one as a grant, so a forged header would otherwise buy
	// read and write on correspondence the forger was never on.
	sync(t, customerMail("forge-target@acme.example"))
	activityID := oneActivityID(t, e)
	setImportDecision(t, e, activityID, e.Rep1, "classified", "")
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("the fixture needs a held message to attack, got audience %q", got)
	}

	// The second seat syncs their OWN mail — a different message, a different
	// sender, nothing to do with the first seat — but stamped with the held
	// message's Message-ID.
	syncOther(t, email("mallory@elsewhere.example", "Mallory", captureOwner, "forge-target@acme.example", ""))

	for _, u := range importRowsOf(t, e, activityID) {
		if u == e.Rep3 {
			t.Fatal("a forged Message-ID bought an import row on a colleague's held message")
		}
	}

	// A DECLARED DOMAIN buys nothing either. A seat declares one with no proof
	// of control, so if a domain counted as delivery evidence, claiming a
	// colleague's domain and then forging a Message-ID would walk straight
	// through this gate.
	declareIdentity(t, e, e.Rep3, capturemod.IdentityKindDomain, "claimed.example")
	syncOther(t, email("stranger@elsewhere.example", "Stranger", "victim@claimed.example",
		"forge-target@acme.example", ""))
	for _, u := range importRowsOf(t, e, activityID) {
		if u == e.Rep3 {
			t.Fatal("a self-declared domain bought an import row on a colleague's held message")
		}
	}

	// And the admit case, which is what proves the refusal above is a rule
	// rather than a gate that refuses everyone: a seat who IS on the message
	// gets their import row from the same code path.
	sync(t, customerMailToBoth("forge-admit@acme.example"))
	var admitted ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_id = $1`, "forge-admit@acme.example").Scan(&admitted)
	}); err != nil {
		t.Fatalf("reading the admit-case activity: %v", err)
	}
	syncOther(t, customerMailToBoth("forge-admit@acme.example"))
	var second bool
	for _, u := range importRowsOf(t, e, admitted) {
		if u == e.Rep3 {
			second = true
		}
	}
	if !second {
		t.Fatal("a seat legitimately on an open message got no import row — the gate refuses everyone")
	}
}

func TestTheWorkspaceMailSharingFloorSurvivesTheDerivation(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	// A SHARED mailbox, so the floor is the only thing that can hold this
	// message. Without that the mailbox's own default (classified) holds it
	// anyway, and the test passes whether the floor works or not — which it did
	// until postures existed.
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)

	// Mail sharing OFF: the sink births every new email held to its
	// participants. That is a workspace decision, and no import row records it
	// — so a derivation reading import rows alone sees nothing asking for a
	// hold and widens the message back to the whole workspace on the very
	// capture that just held it.
	setMailSharing(t, e, false)
	sync(t, customerMail("floor-1@acme.example"))
	activityID := oneActivityID(t, e)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("with mail sharing off a captured email must be held, got audience %q", got)
	}

	// And it stays held across a later recompute, which is the path a second
	// mailbox's sync takes.
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("a recompute widened a message the workspace floor held, got %q", got)
	}
}

// setMailSharing flips the workspace posture through the real settings store,
// so the fixture exercises the value the sink actually reads.
func setMailSharing(t *testing.T, e *integration.SearchEnv, on bool) {
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
	store := capturemod.NewSettings(compose.NewSettingsStore(e.Pool))
	if _, err := store.Update(ctx, capturemod.SettingsPatch{MailSharing: &on}); err != nil {
		t.Fatalf("setting mail sharing to %v: %v", on, err)
	}
}

func TestAMessageWithNoRecoverableImporterIsLeftAsItIs(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, customerMail("orphan-1@acme.example"))
	activityID := oneActivityID(t, e)

	// A captured row whose importing seat cannot be recovered: the migration's
	// backfill reads the trailing uuid out of captured_by, and a connection with
	// no human behind it stamps the bare connector id. Rows captured before this
	// feature can also predate that spelling.
	//
	// The derivation must leave such a row exactly as it is. It has no
	// contributor to derive from, and deriving "no contributor asks for a hold"
	// into "workspace" would publish a message on the strength of not knowing
	// anything about it.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`DELETE FROM capture_import WHERE activity_id = $1`, activityID); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET captured_by = 'connector:gmail', audience = 'participants',
			        audience_reason = NULL WHERE id = $1`, activityID)
		return err
	}); err != nil {
		t.Fatalf("shaping the orphan row: %v", err)
	}

	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("a derivation widened a message with no recoverable importer, got %q", got)
	}
}

func TestARowCarriedHoldOutlivesAMailboxPostureOnTheSameMessage(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync

	// A message held by the WORKSPACE floor, which a mailbox posture then also
	// holds. Both reasons cannot fit in one column, and the floor is the one
	// that must survive: a verdict clears a mailbox posture, and nothing clears
	// an admin's decision that colleagues do not read captured mail.
	setMailSharing(t, e, false)
	sync(t, customerMail("survive-1@acme.example"))
	activityID := oneActivityID(t, e)

	setImportDecision(t, e, activityID, e.Rep1, "classified", "")
	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("both holds agree the message is held, got %q", got)
	}

	// The mailbox's verdict clears. The floor has not moved, so the message
	// must not open.
	setImportDecision(t, e, activityID, e.Rep1, "classified", "cleared")
	recompute(t, e, activityID)
	if got, reason := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("clearing a mailbox verdict published mail the workspace floor held: %q / %q", got, reason)
	}
}

func TestARowNarrowedBeforeThisFeatureIsNotPublishedByTheFirstRecompute(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	sync(t, customerMail("legacy-1@acme.example"))
	activityID := oneActivityID(t, e)

	// The shape a DEPLOYED database is in the moment this migration runs: a row
	// three older writers could narrow — the ladder's link-less hold, the
	// workspace floor, a human's own PATCH — none of which recorded why, plus
	// the import row the migration backfills for it.
	//
	// The integration lane cannot see this by itself: it starts from a fresh
	// schema, so every row it holds was written by the new code and carries a
	// reason. This test builds the old shape deliberately, because the
	// derivation reading it as "no contributor asks for a hold" would publish
	// every already-private message in the installation on the next sync.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET audience = 'participants', audience_reason = NULL WHERE id = $1`,
			activityID)
		return err
	}); err != nil {
		t.Fatalf("shaping the pre-migration row: %v", err)
	}
	backfillReasons(t, e)

	recompute(t, e, activityID)
	if got, _ := audienceOf(t, e, activityID); got != "participants" {
		t.Fatalf("the first recompute published a message narrowed before this feature, got %q", got)
	}
}

// backfillReasons runs the migration's own reason backfill, so the fixture
// exercises the statement that ships rather than a paraphrase of it.
func backfillReasons(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity SET audience_reason = 'manual'
			 WHERE audience <> 'workspace' AND audience_reason IS NULL`)
		return err
	}); err != nil {
		t.Fatalf("running the reason backfill: %v", err)
	}
}

// activityIDOf answers the activity a given Message-ID landed as.
func activityIDOf(t *testing.T, e *integration.SearchEnv, sourceID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_id = $1`, sourceID).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the activity for %s: %v", sourceID, err)
	}
	return id
}
