// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// How far a `noise` verdict reaches, and what it is allowed to destroy
// (ADR-0072/A118 §4). The verdict is evidence about ONE inbound message whose
// From header nobody authenticates, so its reach is drawn deliberately: wide
// enough to cover every message that sender already wrote, and no wider — never
// the workspace's own correspondence, never mail that arrives long afterwards.
// Destruction is narrower still than hiding: hiding is reversible and a reply
// releases it, so an uncorroborated verdict may hide forever but never redact.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A disposition covers the SENDER, so its effects cover every message that
// sender wrote — not just the one that happened to raise the question. The
// second and third mail from a stranger join the open question rather than
// raising their own, so an effect keyed on the ledger row's single activity_id
// would hide message #1 and leave the rest on the timeline with full bodies:
// "noise is not shown" defeated by sending two emails instead of one.
func TestANoiseVerdictHidesEveryMessageThatSenderWrote(t *testing.T) {
	e := integration.Setup(t)
	// All three carry List-Unsubscribe, so the redaction half below is reachable
	// too — that corroboration is per-message (migration 0137), while the
	// sender-wide reach this test exists to pin is per-address.
	first := seedBulkCapturedMail(t, e, "bulk@flood.example", "offer one")
	second := seedBulkCapturedMail(t, e, "bulk@flood.example", "offer two")
	third := seedBulkCapturedMail(t, e, "bulk@flood.example", "offer three")
	dispositionID := seedPendingDisposition(t, e, "bulk@flood.example", "flood.example", first)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	for _, id := range []ids.UUID{first, second, third} {
		if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, id); n != 1 {
			t.Fatalf("activity %s stayed visible — the verdict must cover the sender, not one message", id)
		}
	}

	for _, id := range []ids.UUID{first, second, third} {
		backdateArchive(t, e, id)
	}
	if err := engine.RedactNoiseWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), capture.NoiseUndoWindow, 0); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM activity
		 WHERE counterparty_email = 'bulk@flood.example'
		   AND (subject IS NOT NULL OR body IS NOT NULL OR raw IS NOT NULL)`); n != 0 {
		t.Fatalf("%d of the sender's messages kept their content past the undo window", n)
	}
}

// counterparty_email comes from the message's own From header, which nobody
// authenticates. So an outsider can mail the connected mailbox claiming to be
// anyone — and if a noise verdict acted on "every message bearing this address",
// one forged message would hide and then destroy the workspace's real
// correspondence with whoever was named.
//
// The scope is therefore narrower than the address: inbound only, never
// provider-attested, never linked to a person, and it stops applying entirely
// once the workspace has written to that address.
func TestAForgedSenderCannotReachTheWorkspacesOwnCorrespondence(t *testing.T) {
	e := integration.Setup(t)
	victim := "bigcustomer@corp.example"

	// The workspace's genuine relationship with the named party: mail it sent,
	// and inbound mail the provider attested as part of that correspondence.
	ownSent := seedOutboundMail(t, e, victim, "our proposal")
	// The attacker's single forged message, which is what actually gets judged.
	forged := seedCapturedMail(t, e, victim, "🚀 buy followers now")
	dispositionID := seedPendingDisposition(t, e, victim, "corp.example", forged)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if err := engine.HideNoiseStragglersWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}

	// The workspace's own sent mail is untouched — a stranger's forged header
	// has no authority over the record the workspace made itself.
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, ownSent); n != 1 {
		t.Fatal("a forged From header hid the workspace's OWN outbound mail")
	}
	// Writing to a wrongly-hidden sender is the recovery path: correspondence is
	// the T1 signal that they are a counterparty, so the sweep lets go. Both
	// messages are archived and aged past the window first — otherwise the
	// assertions below would hold whatever the scope rule did.
	backdateArchive(t, e, forged)
	backdateArchive(t, e, ownSent)
	if err := engine.RedactNoiseWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), capture.NoiseUndoWindow, 0); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, forged); n != 1 {
		t.Fatal("mail from an address the workspace corresponds with was redacted — replying must call the sweep off")
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND body IS NOT NULL`, ownSent); n != 1 {
		t.Fatal("the workspace's own outbound mail was redacted by a stranger's verdict")
	}
}

// seedOutboundMail inserts one message the workspace SENT, attested by the
// provider — the T1 correspondence evidence.
func seedOutboundMail(t *testing.T, e *integration.Env, to, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source, captured_by, counterparty_email, counterparty_outbound_attested)
			VALUES ($1, 'email', $2, 'our own words', 'outbound',
			        'gmail', $3, 'gmail:'||$3, 'connector:gmail', $4, true)`,
			id, subject, "out-"+id.String(), to)
		return err
	})
	if err != nil {
		t.Fatalf("seeding outbound mail: %v", err)
	}
	return id
}

// The destruction is one transaction, so a failure cannot leave the activity
// stripped while the provider original survives. This drives the case that
// worried a reviewer: redact the activity's content by itself — the state a
// crash between two separate transactions would leave — and assert the sweep
// still collects it. A message whose original outlives its text is unfinished
// work, not finished work, and nothing else in the system would ever collect it.
func TestRedactionCollectsMailWhoseOriginalOutlivedItsText(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedBulkCapturedMail(t, e, "loud@bulk.example", "offer")
	dispositionID := seedPendingDisposition(t, e, "loud@bulk.example", "bulk.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// Simulate the half-done state: content gone, original still held.
	stripActivityContent(t, e, activityID)
	if n := rawCaptureRows(t, e, activityID); n != 1 {
		t.Fatal("the fixture did not reproduce the half-done state")
	}

	backdateArchive(t, e, activityID)
	if err := engine.RedactNoiseWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), capture.NoiseUndoWindow, 0); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}
	if n := rawCaptureRows(t, e, activityID); n != 0 {
		t.Fatal("the sweep skipped mail whose original outlived its text — that original would be retained forever")
	}
}

// The forged-bulk attack, and the line that stops it costing somebody their
// mail: write a message as an address the workspace has never corresponded
// with, shape it to read as marketing, and the model's noise verdict lands on
// the ADDRESS. Hiding on that evidence is fine — it is reversible, and a reply
// releases it. Destroying on it is not, and the victim cannot object to a
// destruction they never saw coming, because the mail was hidden first.
//
// So an ordinary message — no List-Unsubscribe header, nothing corroborating
// that it is bulk — is hidden and stays hidden. It is never destroyed, however
// long the undo window runs out.
func TestANoiseVerdictWithoutBulkCorroborationHidesButNeverDestroys(t *testing.T) {
	e := integration.Setup(t)
	// seedCapturedMail, NOT seedBulkCapturedMail: an ordinary message.
	activityID := seedCapturedMail(t, e, "real.person@partner.example", "about the contract")
	dispositionID := seedPendingDisposition(t, e, "real.person@partner.example", "partner.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The reversible half still works — the verdict is acted on immediately.
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, activityID); n != 1 {
		t.Fatal("the noise verdict did not hide the message — the reversible half of the effect must be unchanged")
	}

	// Age it well past the undo window and sweep twice: no amount of waiting
	// turns an uncorroborated verdict into permission to destroy.
	backdateArchive(t, e, activityID)
	for range 2 {
		if err := engine.RedactNoiseWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), capture.NoiseUndoWindow, 0); err != nil {
			t.Fatalf("redaction sweep: %v", err)
		}
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND subject IS NOT NULL AND body IS NOT NULL`, activityID); n != 1 {
		t.Fatal("the sweep destroyed mail on the model's word alone — a forged bulk message would take a real correspondent's mail with it")
	}
	if n := rawCaptureRows(t, e, activityID); n != 1 {
		t.Fatalf("%d provider originals left, want 1 — the original must survive alongside the text", n)
	}
}

// stripActivityContent nulls only the activity's text, leaving the provider
// original — the state a non-atomic redaction would commit.
func stripActivityContent(t *testing.T, e *integration.Env, id ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET subject = NULL, body = NULL, raw = NULL WHERE id = $1`, id)
		return err
	})
	if err != nil {
		t.Fatalf("stripping the activity content: %v", err)
	}
}

// A noise verdict is evidence about the mail that was in front of it, so it
// cannot reach forward forever. Otherwise one forged message — sent as an
// address the workspace has never written to — would hide and destroy every mail
// the real owner of that address ever sends afterwards, unseen, with the
// "reply to recover" escape unreachable because the victim's mail is invisible.
func TestANoiseVerdictCannotReachMailSentLongAfterIt(t *testing.T) {
	e := integration.Setup(t)
	poisoned := seedCapturedMail(t, e, "cfo@bigcorp.example", "🚀 crypto deals")
	dispositionID := seedPendingDisposition(t, e, "cfo@bigcorp.example", "bigcorp.example", poisoned)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// Time passes — well past the window in which this verdict is evidence
	// about anything — and then the real owner writes.
	backdateResolution(t, e, dispositionID, 30*24*time.Hour)
	genuine := seedCapturedMail(t, e, "cfo@bigcorp.example", "re: our contract renewal")
	if err := engine.HideNoiseStragglersWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}

	// The forged message is theirs to hide. The genuine one is not.
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, poisoned); n != 1 {
		t.Fatal("the judged message was not hidden")
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, genuine); n != 1 {
		t.Fatal("mail sent long after the verdict was hidden by it — a forged message must not bar an address forever")
	}

	// A sender who stamps a far-future Date: header must not slip the reach
	// either. occurred_at is the message's own header, as forgeable as the From
	// this scope rule exists to distrust, so the bound reads the capture clock.
	dated := seedCapturedMail(t, e, "cfo@bigcorp.example", "posted from the future")
	stampOccurredAt(t, e, dated, 90*24*time.Hour)
	sync := seedPendingDisposition(t, e, "cfo@bigcorp.example", "bigcorp.example", dated)
	resolveAsNoise(t, e, sync)
	if err := engine.HideNoiseStragglersWorkspace(principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, dated); n != 1 {
		t.Fatal("a forged future Date header slipped the verdict's reach — the bound must read the capture clock")
	}

	// That the same mail also raises a FRESH question is the ladder's half of
	// this rule, proven where the real capture path runs
	// (capture_tiergate_integration_test.go) — this fixture inserts activities
	// directly and never consults the ladder.
}

// backdateResolution ages a resolved disposition so its undo window has passed,
// instead of sleeping out a seven-day wait.
func backdateResolution(t *testing.T, e *integration.Env, id ids.UUID, by time.Duration) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET resolved_at = now() - $2::interval WHERE id = $1`, id, by.String())
		return err
	})
	if err != nil {
		t.Fatalf("backdating the resolution: %v", err)
	}
}

// stampOccurredAt sets a message's self-reported arrival time — the Date header
// a sender chooses, as opposed to when capture actually saw it.
func stampOccurredAt(t *testing.T, e *integration.Env, id ids.UUID, ahead time.Duration) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET occurred_at = now() + $2::interval WHERE id = $1`, id, ahead.String())
		return err
	})
	if err != nil {
		t.Fatalf("stamping the arrival time: %v", err)
	}
}

// resolveAsNoise puts a disposition straight into the noise state, for cases
// where the verdict itself is not what is under test.
func resolveAsNoise(t *testing.T, e *integration.Env, id ids.UUID) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_pending_counterparty
			   SET status = 'noise', resolved_at = now(), next_attempt_at = NULL,
			       claimed_until = NULL, claimed_by = NULL
			 WHERE id = $1`, id)
		return err
	})
	if err != nil {
		t.Fatalf("resolving as noise: %v", err)
	}
}

// A contact merely COPIED on a newsletter does not exempt it from the sweep.
//
// The sweep's scope refuses mail "linked to a person" because a linked message
// belongs to somebody's record. That read the link as evidence of who the
// message is WITH, which held only while a message was filed under the party
// the ladder judged. Capture now files a message under every participant it
// resolves (capture/sinkmaillinks.go), so a blast naming one contact in Cc
// carries a person link — and without the role test the predicate is never true
// again for it: the message can never be hidden, and never redacted, however
// plainly its sender is judged noise.
//
// Mutation: drop the role arm from noiseMailScope and this fails at the first
// assertion, the blast still visible.
func TestABlastIsHiddenThoughItCopiesAContact(t *testing.T) {
	e := integration.Setup(t)

	blast := seedBulkCapturedMail(t, e, "bulk@flood.example", "offer one")
	// The contact the blast copied, filed under it exactly as capture files a
	// resolved participant — a link plus a participant row that is NOT the author.
	copied := seedPerson(t, e, "cc@partner.example")
	fileUnderAs(t, e, blast, copied, "cc")

	dispositionID := seedPendingDisposition(t, e, "bulk@flood.example", "flood.example", blast)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, blast); n != 1 {
		t.Fatal("a blast stayed visible because it copied a contact — being in the Cc line of a " +
			"newsletter is not a record the newsletter belongs to, and the sweep can never reach it again")
	}
}

// The sender's OWN link still calls the sweep off, which is the rule the arm
// above narrows and must not repeal. Without this the narrowing could refuse
// everybody's link and look correct.
func TestTheSendersOwnRecordStillCallsTheSweepOff(t *testing.T) {
	e := integration.Setup(t)

	mail := seedBulkCapturedMail(t, e, "bulk@flood.example", "offer one")
	sender := seedPerson(t, e, "bulk@flood.example")
	fileUnderAs(t, e, mail, sender, "from")

	dispositionID := seedPendingDisposition(t, e, "bulk@flood.example", "flood.example", mail)
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, mail); n != 1 {
		t.Fatal("a message filed under the person who WROTE it was hidden — that message belongs " +
			"to their record, and a stale disposition has no authority over it")
	}
}

// seedPerson mints a contact at one address, for the link fixtures above.
func seedPerson(t *testing.T, e *integration.Env, email string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO person (id, full_name, source, captured_by)
			VALUES ($1, 'Someone', 'manual', 'human:x')`, id); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO person_email (person_id, email, source, captured_by)
			VALUES ($1, $2, 'manual', 'human:x')`, id, email)
		return err
	}); err != nil {
		t.Fatalf("seeding the contact %s: %v", email, err)
	}
	return id
}

// fileUnderAs files a message under a person in the shape capture leaves: the
// activity_link row, and the activity_participant row naming the role they were
// on the message in. Both, because it is the PAIR the sweep now reads.
func fileUnderAs(t *testing.T, e *integration.Env, activity, person ids.UUID, role string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, activity, person); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity_participant (activity_id, person_id, role)
			VALUES ($1, $2, $3)`, activity, person, role)
		return err
	}); err != nil {
		t.Fatalf("filing the message under the person as %q: %v", role, err)
	}
}

// decidingBrain records the owner's decision WHILE it answers, which is exactly
// the window the defect lives in: judgeOne reads the override in a transaction
// of its own, then spends a model call, and only then does apply open the
// transaction that runs the effects.
//
// A brain rather than a goroutine and a sleep. The race is real but its window
// is milliseconds wide, so a timing test would be the flake this rulebook
// forbids; hooking the one call that HAPPENS in the window makes the ordering a
// fact of the test rather than a hope.
type decidingBrain struct {
	inner  *scriptedVerdictBrain
	record func()
	once   bool
}

func (d *decidingBrain) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if !d.once {
		d.once = true
		d.record()
	}
	return d.inner.Complete(ctx, req)
}

// The owner answers during the model call, and the answer counts.
//
// The contact half already re-checked under the person row lock. The MAIL half
// did not: the hide and the workspace-wide domain suppression both ran on what
// judgeOne read before the seat spoke — so a person who said "this is business"
// watched the sender's whole domain get suppressed for every colleague, on the
// strength of a read that was already stale when it was used.
func TestAnOwnerDecidingDuringTheModelCallIsNotOverruled(t *testing.T) {
	e := integration.Setup(t)
	first := seedBulkCapturedMail(t, e, "dana.olsen@partner.example", "quarterly note")
	dispositionID := seedPendingDisposition(t, e, "dana.olsen@partner.example", "partner.example", first)

	scripted := &scriptedVerdictBrain{
		verdicts: map[string]string{dispositionID.String(): capture.KindNewsletter},
	}
	brain := &decidingBrain{inner: scripted, record: func() {
		seedSenderOverride(t, e, e.Rep1, "dana.olsen@partner.example", capture.OverrideBusiness)
	}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// The model was actually asked, which is what puts the decision inside the
	// window. Without this the case passes vacuously on any address that short
	// -circuits before the model — a role mailbox does, and an earlier draft of
	// this test used `hello@` and proved nothing at all.
	if !brain.once {
		t.Fatal("no model call, so the owner never decided mid-flight and this case " +
			"asserts nothing about the window it exists for")
	}

	// The mail stays visible. This is the half that was broken.
	if n := countIn(t, e,
		`SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, first); n != 0 {
		t.Errorf("the sender's mail was hidden after their owner called them business — " +
			"the effects ran on a read taken before the decision existed")
	}
	// And no domain suppression, which is the workspace-wide one: a colleague
	// would otherwise stop receiving a company this seat just vouched for.
	if n := countIn(t, e,
		`SELECT count(*) FROM organization_domain_disposition
		  WHERE domain = 'partner.example' AND admission = 'suppressed'`); n != 0 {
		t.Errorf("partner.example was suppressed for the whole workspace on one seat's " +
			"stale read, after that seat said the sender is business")
	}
	// The ledger carries the OWNER's answer, in this pass. Leaving the row open
	// for a later one would have been a stall: it was already leased under a
	// backoff, so "the next tick will fix it" means minutes, and until then the
	// sender sits unjudged with a decision on file.
	if got := dispositionStatus(t, e, dispositionID); got != "real" {
		t.Errorf("the disposition settled as %q, want real — the seat called this "+
			"sender business, and that is the answer the ledger should carry", got)
	}
}

// The lock, proved by making a concurrent writer WAIT for it.
//
// The re-read narrowed the window; it did not close it. A Set() committing
// between the read and the effects is still a stale answer acted on, and under
// READ COMMITTED no amount of re-reading fixes that — the two paths have to
// serialize on one key.
//
// Deterministic rather than timed: the test holds the lock in a transaction of
// its own and asserts the second attempt cannot take it while the first is
// open, then that it succeeds the moment the first commits. No sleeps, no
// goroutine racing a goroutine, and it fails when the lock is absent rather
// than when a machine is slow.
func TestASenderDecisionAndItsReaderSerializeOnOneKey(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	// One transaction takes the decision's lock and holds it open.
	go func() {
		done <- database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := capture.OverrideForTx(ctx, tx, e.Rep1, "dana.olsen@partner.example"); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	// A second one cannot take it. try_advisory asks the same question the
	// blocking lock does without waiting for the answer, so the assertion is
	// "the key is held", not "this call was slow".
	var free bool
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))`,
			"capture_sender_override_write", e.Rep1.String()+":dana.olsen@partner.example").Scan(&free)
	}); err != nil {
		t.Fatalf("probing the decision's lock: %v", err)
	}
	if free {
		t.Error("a second transaction took the decision's key while a reader held it — " +
			"the read and the act it drives are not serialized against a seat " +
			"recording a decision between them")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the holding transaction: %v", err)
	}

	// And it is released on commit, so this serializes writers rather than
	// wedging them.
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1 || ':' || $2, 0))`,
			"capture_sender_override_write", e.Rep1.String()+":dana.olsen@partner.example").Scan(&free)
	}); err != nil {
		t.Fatalf("re-probing the decision's lock: %v", err)
	}
	if !free {
		t.Error("the key stayed held after the transaction committed — an xact lock " +
			"that outlives its transaction would stall every later booking of this seat")
	}
}
