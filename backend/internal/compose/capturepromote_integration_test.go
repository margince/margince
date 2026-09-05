// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whose record a captured contact is, and what makes it the workspace's.
//
// Capture mints every person owner-scoped, because the workspace writing to an
// address proves the address is a counterparty and not that the counterparty is
// the business's. The verdict is what settles that, and `person` is the answer
// that widens the row. The tests below drive the REAL sink — the one compose
// assembles for production — so what they prove about visibility is what a
// captured mail actually does.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seedAttestedOutbound records that the workspace WROTE to this address — the
// T1 correspondence evidence, and the precondition every test here starts from.
//
// Written as SQL rather than captured through the sink because the attestation
// may only be minted inside capture/mailmap, where a provider's own filing of
// the message is known (TestOnlyTheMailMapperMintsTheOutboundAttestation holds
// that). A test that minted it itself would be claiming an authority the
// product deliberately gives no caller.
// seedAttestedOutbound lands one message the workspace sent, on a named thread.
//
// The thread matters: an exchange is a reply to something we wrote, and the two
// halves are joined by the thread key rather than by the address. A seeded
// outbound with no thread cannot be answered.
func seedAttestedOutbound(t *testing.T, e *integration.Env, sourceID, counterparty, threadKey string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, thread_key, counterparty_email,
			                      counterparty_outbound_attested)
			VALUES ('email', 'Angebot', 'Anbei.', 'outbound', 'gmail', $1,
			        'gmail:'||$1, 'connector:gmail', $3, $2, true)`,
			sourceID, counterparty, threadKey)
		return err
	}); err != nil {
		t.Fatalf("seeding the workspace's own mail to %s: %v", counterparty, err)
	}
}

// captureInboundThroughRealSink lands one INBOUND mail through the sink compose
// builds for production — the ensurer attached, so the tier ladder actually runs
// and can create a person. captureMail in the participant suite deliberately
// uses a bare sink; this one is here because the ladder is the subject.
func captureInboundThroughRealSink(
	t *testing.T, e *integration.Env, owner ids.UUID, sourceID, counterparty, threadKey string,
) {
	t.Helper()
	// Domain is populated the way mailmap populates it — the address's own
	// host. Omitting it looks harmless and is not: the ledger row would carry
	// no domain, and every effect keyed on the domain (the company refusal
	// among them) returns early on a record no connector ever produces.
	domain := counterparty[strings.LastIndex(counterparty, "@")+1:]
	sink := newCaptureSink(e.Pool, CaptureConfig{})
	_, err := sink.Upsert(mailboxOwnerCtx(e, owner), connector.NormalizedRecord{
		EntityType: "activity",
		NaturalKey: connector.NaturalKey{SourceSystem: "gmail", SourceID: sourceID},
		Counterparty: connector.Counterparty{
			Email: counterparty, DisplayName: "Pat Counterparty",
			Domain: domain, Direction: connector.DirectionInbound,
		},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "Angebot", Body: "Anbei.", Direction: connector.DirectionInbound,
		},
		ThreadKey: threadKey,
		// One of the SEAT's own addresses on the message, which is the evidence
		// the import row is written on: without it the sink stores the activity
		// but records no per-seat contribution, and nothing opens a question.
		Addresses: []string{counterparty, "a@authz.test"},
		Source:    "gmail:" + sourceID, CapturedBy: "connector:gmail",
	})
	if err != nil {
		t.Fatalf("capturing %s: %v", sourceID, err)
	}
}

// personVisibility reads the row's visibility and reports whether a person for
// that address exists at all.
func personVisibility(t *testing.T, e *integration.Env, email string) (string, bool) {
	t.Helper()
	var visibility string
	found := true
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `
			SELECT p.visibility
			  FROM person p JOIN person_email pe ON pe.person_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&visibility)
		if err == pgx.ErrNoRows {
			found = false
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the visibility of %s: %v", email, err)
	}
	return visibility, found
}

// openDisposition returns the id of the ledger row still awaiting a verdict for
// this address, and whether there is one.
func openDisposition(t *testing.T, e *integration.Env, email string) (ids.UUID, bool) {
	t.Helper()
	var id ids.UUID
	found := true
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(), `
			SELECT id FROM capture_pending_counterparty
			 WHERE email = $1 AND status = 'pending'`, email).Scan(&id)
		if err == pgx.ErrNoRows {
			found = false
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the open disposition for %s: %v", email, err)
	}
	return id, found
}

func runVerdict(t *testing.T, e *integration.Env, brain *scriptedVerdictBrain) {
	t.Helper()
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
}

func TestACorrespondedSenderIsJudgedAndBecomesTheWorkspacesContact(t *testing.T) {
	// The defect this suite exists for. A sender the workspace writes to is
	// created on sight by the T1 rung, and before this the ladder asked no
	// question about them — so nothing ever promoted the row and the contact
	// stayed invisible to every colleague, permanently. The strongest evidence
	// a sender is a counterparty produced the most private record.
	e := integration.Setup(t)
	const sender = "timo@partner.example"

	// The workspace writes first: this is the T1 evidence.
	seedAttestedOutbound(t, e, "promote-out-1", sender, "promote-t1")
	// Then they reply, and the ladder creates the person on sight.
	captureInboundThroughRealSink(t, e, e.Rep1, "promote-in-1", sender, "promote-t1")

	visibility, found := personVisibility(t, e, sender)
	if !found {
		t.Fatalf("the T1 rung created no person for a corresponded sender")
	}
	if visibility != "owner" {
		t.Fatalf("a freshly captured person is %q, want owner — capture cannot yet know whose the contact is", visibility)
	}

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for a sender the ladder created a record for, " +
			"so nothing will ever promote the row and the contact stays the mailbox owner's forever")
	}

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)

	if visibility, _ = personVisibility(t, e, sender); visibility != "workspace" {
		t.Errorf("after a `person` verdict the contact is %q, want workspace — "+
			"a judged business counterparty belongs to the business", visibility)
	}
}

func TestAnAdvisorVerdictLeavesACorrespondedContactTheOwnersOwn(t *testing.T) {
	// The other half, and the reason the fix routes through the verdict rather
	// than simply creating workspace-scoped rows. A founder's lawyer is
	// corresponded with exactly like a customer; publishing them announces that
	// the founder has a lawyer.
	e := integration.Setup(t)
	const sender = "kanzlei@privat.example"

	seedAttestedOutbound(t, e, "advisor-out-1", sender, "advisor-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "advisor-in-1", sender, "advisor-t1")

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindAdvisor}}
	runVerdict(t, e, brain)

	if visibility, _ := personVisibility(t, e, sender); visibility != "owner" {
		t.Errorf("an advisor verdict left the contact %q, want owner — "+
			"publishing it announces that the mailbox owner has one", visibility)
	}
}

func TestASettledSenderIsNotAskedAgain(t *testing.T) {
	// The guard on the enqueue. Without it every later message from a judged
	// sender re-opens the question: a paid model call to re-derive a decision
	// that stands, and — for an advisor — a fresh chance to overturn an answer
	// whose whole point is that the record stays private.
	e := integration.Setup(t)
	const sender = "known@partner.example"

	seedAttestedOutbound(t, e, "settled-out-1", sender, "settled-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "settled-in-1", sender, "settled-t1")

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)

	// They write again, long after the answer.
	captureInboundThroughRealSink(t, e, e.Rep1, "settled-in-2", sender, "settled-t1")

	if _, reopened := openDisposition(t, e, sender); reopened {
		t.Error("a later message re-opened a settled sender's verdict — " +
			"the answer already stands, and asking again spends a model call to be told it twice")
	}
}

func TestADomainTheWorkspaceCorrespondsWithIsNeverRefusedACompany(t *testing.T) {
	// The blast radius of asking about corresponded senders at all. A supplier's
	// marketing mail is genuinely a newsletter, and the noise arm suppresses the
	// sender's whole DOMAIN — so without the correspondence guard, one blast
	// from a company the business works with every day would refuse that company
	// workspace-wide, standing.
	e := integration.Setup(t)
	const sender = "news@supplier.example"

	seedAttestedOutbound(t, e, "supplier-out-1", sender, "supplier-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "supplier-in-1", sender, "supplier-t1")

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindNewsletter}}
	runVerdict(t, e, brain)

	// The refusal is the `admission` column, not `status`: status tracks the
	// crawl, admission is the standing decision about whether the domain may
	// ever become a company. Asserting on status passes whatever the verdict
	// did, which is a test that cannot fail.
	if admission := domainAdmission(t, e, "supplier.example"); admission == people.DomainSuppressed {
		t.Error("a newsletter verdict refused a company the workspace corresponds with — " +
			"hiding the blast is right, refusing the supplier on the strength of it is not")
	}
}

func TestAFullQueueDoesNotStrandAContactAsTheOwnersForever(t *testing.T) {
	// The ceiling must delay the question, never cancel it. Once the person
	// exists, the ladder reads their address as already-known on every later
	// message — so a create whose question the cap refused has no second chance
	// unless the enqueue is retried for a record still owner-scoped. Without
	// that retry the contact is permanently invisible to colleagues, which is
	// the defect this whole suite exists to close, reachable by anyone who can
	// fill the queue with fresh addresses.
	e := integration.Setup(t)
	const sender = "capped@partner.example"

	// Fill this domain's share of the ceiling with other senders' open
	// questions, so the create below finds no room to ask.
	for i := 0; i < capture.PendingDeferralDomainCap; i++ {
		other := fmt.Sprintf("filler%d@partner.example", i)
		seedAttestedOutbound(t, e, fmt.Sprintf("filler-out-%d", i), other, fmt.Sprintf("filler-t%d", i))
		captureInboundThroughRealSink(t, e, e.Rep1, fmt.Sprintf("filler-in-%d", i), other, fmt.Sprintf("filler-t%d", i))
	}

	seedAttestedOutbound(t, e, "capped-out-1", sender, "capped-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "capped-in-1", sender, "capped-t1")
	if _, found := personVisibility(t, e, sender); !found {
		t.Fatal("the T1 rung created no person for a corresponded sender")
	}

	// The queue drains: the fillers are answered and their slots come back.
	runVerdict(t, e, &scriptedVerdictBrain{})

	// They write again, and the question that was refused must now be asked.
	captureInboundThroughRealSink(t, e, e.Rep1, "capped-in-2", sender, "capped-t1")
	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("a contact whose verdict the ceiling refused was never asked about again — " +
			"the cap cancelled the question instead of delaying it, and the record " +
			"stays the mailbox owner's for good")
	}

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)
	if visibility, _ := personVisibility(t, e, sender); visibility != "workspace" {
		t.Errorf("after the delayed verdict the contact is %q, want workspace", visibility)
	}
}

func TestASingleDecliningReplyDoesNotSpareASpammersDomain(t *testing.T) {
	// The correspondence test the domain guard uses has to be the ladder's own,
	// not a simpler "did we ever send here". A founder who answers unsolicited
	// mail with "not interested" produces exactly one attested outbound, and
	// reading that as correspondence would let the reply that told a spammer to
	// stop be the thing that protects their domain.
	e := integration.Setup(t)
	const sender = "angebote@spam.example"

	seedDecliningOutbound(t, e, "decline-out-1", sender)
	captureInboundThroughRealSink(t, e, e.Rep1, "decline-in-1", sender, "decline-t1")

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for this sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindSpam}}
	runVerdict(t, e, brain)

	if admission := domainAdmission(t, e, "spam.example"); admission != people.DomainSuppressed {
		t.Errorf("the domain admission is %q, want suppressed — a single declining reply "+
			"is not correspondence, and must not spare the sender's domain", admission)
	}
}

// seedDecliningOutbound is seedAttestedOutbound for the one shape the T1 gate
// excludes: a lone outbound whose own words end the conversation.
func seedDecliningOutbound(t *testing.T, e *integration.Env, sourceID, counterparty string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, counterparty_outbound_attested)
			VALUES ('email', 'Re: Angebot', 'Kein Interesse, bitte austragen.', 'outbound', 'gmail', $1,
			        'gmail:'||$1, 'connector:gmail', $2, true)`, sourceID, counterparty)
		return err
	}); err != nil {
		t.Fatalf("seeding the declining reply to %s: %v", counterparty, err)
	}
}

// domainAdmission reads the standing decision about a domain, "" when none.
func domainAdmission(t *testing.T, e *integration.Env, domain string) string {
	t.Helper()
	var admission string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(),
			`SELECT coalesce(admission, '') FROM organization_domain_disposition WHERE domain = $1`,
			domain).Scan(&admission)
		if err == pgx.ErrNoRows {
			admission = ""
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the admission of %s: %v", domain, err)
	}
	return admission
}

// TestAReopenedThreadKeepsSomethingToAskAbout covers the message that reopens a
// settled thread from a mailbox whose posture opens nothing.
//
// The reopen clears first_activity_id so the classifier is not shown the text a
// previous answer was about. Filling it again used to depend on the NEXT
// message arriving under a `classified` posture — and a shared mailbox never
// takes that branch, so the row sat pending with nothing to read: unclaimable
// once the claim requires a readable message, and before that, claimed and
// judged on a blank prompt.
func TestAReopenedThreadKeepsSomethingToAskAbout(t *testing.T) {
	e := integration.Setup(t)
	const first = "kunde@partner.example"
	const stranger = "anwalt@kanzlei.example"

	seedClassifiedGmail(t, e, e.Rep1)
	seedAttestedOutbound(t, e, "reopen-out-1", first, "reopen-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "reopen-in-1", first, "reopen-t1")

	// The thread is settled as ordinary, recording the sender it read.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE capture_thread_verdict
			   SET status = 'cleared', seen_addresses = ARRAY[$2::text],
			       resolved_at = now(), next_attempt_at = NULL
			 WHERE thread_key = $1`, "reopen-t1", first)
		return err
	}); err != nil {
		t.Fatalf("settling the thread: %v", err)
	}

	// A sender the verdict never read replies on the same thread. That reopens
	// the question, and this message is what the question is now about.
	captureInboundThroughRealSink(t, e, e.Rep1, "reopen-in-2", stranger, "reopen-t1")

	var status string
	var pointer *ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT status, first_activity_id FROM capture_thread_verdict WHERE thread_key = $1`,
			"reopen-t1").Scan(&status, &pointer)
	}); err != nil {
		t.Fatalf("reading the reopened thread: %v", err)
	}
	if status != capture.VerdictPending {
		t.Fatalf("thread status = %q, want pending: an unseen sender must re-open the question", status)
	}
	if pointer == nil {
		t.Fatal("the re-opened thread points at no message, so the classifier would be asked to " +
			"judge an empty prompt — or, once the claim requires a readable message, never asked at all")
	}
	var pointedAt string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT coalesce(counterparty_email, '') FROM activity WHERE id = $1`, *pointer).Scan(&pointedAt)
	}); err != nil {
		t.Fatalf("reading the message the re-opened question is about: %v", err)
	}
	if pointedAt != stranger {
		t.Fatalf("the re-opened question is about mail from %q, want %q: the classifier must read the "+
			"message that caused the re-open, not the one a previous answer already covered",
			pointedAt, stranger)
	}
}

// TestAHeldMailboxIsNotAskedToPublishItsMail is the refusal the posture owes.
//
// A `held` mailbox keeps its mail whatever a classifier concludes, so it must
// never have a confidentiality question opened for it: an `ordinary` answer on
// that question maps to a workspace audience, which is exactly the publication
// the posture exists to refuse.
func TestAHeldMailboxIsNotAskedToPublishItsMail(t *testing.T) {
	e := integration.Setup(t)
	const first = "kunde@partner.example"
	const stranger = "anwalt@kanzlei.example"

	seedClassifiedGmail(t, e, e.Rep1)
	seedAttestedOutbound(t, e, "held-out-1", first, "held-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "held-in-1", first, "held-t1")

	// The thread is settled, and THEN the seat asks for their mail to be kept.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			UPDATE capture_thread_verdict
			   SET status = 'cleared', seen_addresses = ARRAY[$2::text],
			       resolved_at = now(), next_attempt_at = NULL
			 WHERE thread_key = $1`, "held-t1", first); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET mail_posture = 'held' WHERE user_id = $1`, e.Rep1)
		return err
	}); err != nil {
		t.Fatalf("holding the mailbox: %v", err)
	}

	// A sender the verdict never read replies, which re-opens the question.
	captureInboundThroughRealSink(t, e, e.Rep1, "held-in-2", stranger, "held-t1")

	// The question, if one stands, must not be answerable: a claim is what
	// spends an attempt and reaches a model, and a `cleared` answer to this
	// thread would publish mail the seat asked to keep.
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming due threads: %v", err)
	}
	for _, c := range claimed {
		if c.ThreadKey == "held-t1" {
			t.Fatal("an `always held` mailbox's thread was claimed for classification, and an " +
				"`ordinary` answer publishes mail the seat asked to keep whatever a classifier concludes")
		}
	}
}

// seedClassifiedGmail connects a mailbox under the posture that holds its mail
// until a classifier judges the thread — the only one that opens a question.
//
// account_label is what puts the seat's own address in the identity set, which
// is the evidence an import row is written on: without it the sink stores the
// activity and records no per-seat contribution at all.
func seedClassifiedGmail(t *testing.T, e *integration.Env, user ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection
			       (user_id, provider, status, credential_ref, mail_posture, account_label)
			VALUES ($1, 'gmail', 'connected', 'vault:test', 'classified', 'a@authz.test')
			ON CONFLICT (user_id, provider)
			DO UPDATE SET mail_posture = 'classified', archived_at = NULL,
			              account_label = 'a@authz.test'`, user)
		return err
	}); err != nil {
		t.Fatalf("seeding a classified gmail connection: %v", err)
	}
}

// TestAContactTheCeilingRefusedIsAskedAboutByTheSweep closes the hole the cap
// leaves when the correspondence goes quiet.
//
// The ceiling delays a question rather than cancelling it, and the retry rides
// the NEXT message from that address. A sender who does not write again never
// triggers one, so the contact stays the mailbox owner's for good — invisible
// to every colleague, their manager and an admin — with nothing left to put it
// back in the queue.
//
// Driven through the real cap: the refusal has to be the one production
// produces, not a state the test invented.
func TestAContactTheCeilingRefusedIsAskedAboutByTheSweep(t *testing.T) {
	e := integration.Setup(t)
	const sender = "quiet@partner.example"

	// Fill this domain's share of the ceiling, so the create below finds no
	// room to ask.
	for i := 0; i < capture.PendingDeferralDomainCap; i++ {
		other := fmt.Sprintf("filler%d@partner.example", i)
		seedAttestedOutbound(t, e, fmt.Sprintf("quiet-fill-out-%d", i), other, fmt.Sprintf("quiet-fill-t%d", i))
		captureInboundThroughRealSink(t, e, e.Rep1, fmt.Sprintf("quiet-fill-in-%d", i), other, fmt.Sprintf("quiet-fill-t%d", i))
	}

	seedAttestedOutbound(t, e, "quiet-out-1", sender, "quiet-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "quiet-in-1", sender, "quiet-t1")

	visibility, found := personVisibility(t, e, sender)
	if !found {
		t.Fatal("the capture created no person for a corresponded sender")
	}
	if visibility != "owner" {
		t.Fatalf("the new contact is %q, want owner: an unjudged capture is the mailbox owner's", visibility)
	}
	if _, queued := openDisposition(t, e, sender); queued {
		t.Fatal("the ceiling did not refuse the question, so this test is not exercising the " +
			"case it exists for — the fixture no longer fills the cap")
	}

	// The sender never writes again. The queue drains, and the sweep is what
	// notices the contact nobody was ever asked about.
	runVerdict(t, e, &scriptedVerdictBrain{})

	store := people.NewStore(InstallationDB(e.Pool))
	worker := NewLinkReconcileWorkspaceWorkerForTest(e.Pool, store)
	if err := worker.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("a contact whose question the ceiling refused was never asked about again — " +
			"it stays the mailbox owner's for good, which is the defect this sweep exists to close")
	}

	// And the answer reaches the record: the whole point of asking.
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)
	if got, _ := personVisibility(t, e, sender); got != "workspace" {
		t.Errorf("after the delayed verdict the contact is %q, want workspace", got)
	}
}

// TestTheSweepDoesNotReaskASettledSender holds the other side of the rule.
//
// Any terminal answer counts as asked, `advisor` included: re-asking a settled
// question would put a decided sender back in front of a model, and an advisor
// whose record is deliberately the owner's would be judged again every night.
func TestTheSweepDoesNotReaskASettledSender(t *testing.T) {
	e := integration.Setup(t)
	const advisor = "berater@kanzlei.example"

	seedAttestedOutbound(t, e, "advisor-sweep-out-1", advisor, "advisor-sweep-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "advisor-sweep-in-1", advisor, "advisor-sweep-t1")
	dispositionID, queued := openDisposition(t, e, advisor)
	if !queued {
		t.Fatal("no question was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindAdvisor}}
	runVerdict(t, e, brain)
	if got, _ := personVisibility(t, e, advisor); got != "owner" {
		t.Fatalf("an advisor's record is %q, want owner", got)
	}

	before := countDispositions(t, e, advisor)
	store := people.NewStore(InstallationDB(e.Pool))
	worker := NewLinkReconcileWorkspaceWorkerForTest(e.Pool, store)
	if err := worker.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}
	if after := countDispositions(t, e, advisor); after != before {
		t.Fatalf("the sweep opened %d new question(s) about a settled sender; an advisor whose "+
			"record is deliberately the owner's would be judged again every night", after-before)
	}
}

// countDispositions counts every ledger row for an address, settled or not.
func countDispositions(t *testing.T, e *integration.Env, email string) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_pending_counterparty WHERE email = $1`, email).Scan(&n)
	}); err != nil {
		t.Fatalf("counting dispositions: %v", err)
	}
	return n
}

// TestTheSweepLeavesAContactAHumanKeptPrivate holds the decision a person made.
//
// A captured contact somebody has worked on is not retracted when the
// conversation turns out to be private: people.RetractCaptureOnlyPersonTx
// refuses to archive one carrying a human audit row, and it stays owner-private.
// That is a decision, and asking about it again would put it in front of a
// model whose `person` answer publishes it to the workspace — a transition
// nothing reverses.
func TestTheSweepLeavesAContactAHumanKeptPrivate(t *testing.T) {
	e := integration.Setup(t)
	const sender = "privat@kunde.example"

	// Fill the domain ceiling so the capture's own question is refused, which
	// is what puts this contact in the sweep's selector at all.
	for i := 0; i < capture.PendingDeferralDomainCap; i++ {
		other := fmt.Sprintf("kept%d@kunde.example", i)
		seedAttestedOutbound(t, e, fmt.Sprintf("kept-fill-out-%d", i), other, fmt.Sprintf("kept-fill-t%d", i))
		captureInboundThroughRealSink(t, e, e.Rep1, fmt.Sprintf("kept-fill-in-%d", i), other, fmt.Sprintf("kept-fill-t%d", i))
	}
	seedAttestedOutbound(t, e, "kept-out-1", sender, "kept-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "kept-in-1", sender, "kept-t1")
	if _, queued := openDisposition(t, e, sender); queued {
		t.Fatal("the ceiling did not refuse the question, so the fixture no longer fills the cap")
	}

	// Somebody works on the contact: the evidence a human touched it.
	personID := personIDFor(t, e, sender)
	seedHumanEdit(t, e, personID)

	runVerdict(t, e, &scriptedVerdictBrain{})
	store := people.NewStore(InstallationDB(e.Pool))
	worker := NewLinkReconcileWorkspaceWorkerForTest(e.Pool, store)
	if err := worker.reconcileLinksForWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the sweep failed: %v", err)
	}

	if _, queued := openDisposition(t, e, sender); queued {
		t.Fatal("the sweep re-asked about a contact a person had already worked on and kept " +
			"private; a `person` answer to that question publishes it to the workspace")
	}
	if got, _ := personVisibility(t, e, sender); got != "owner" {
		t.Fatalf("the contact is %q, want owner", got)
	}
}

// personIDFor reads the person minted for an address.
func personIDFor(t *testing.T, e *integration.Env, email string) ids.PersonID {
	t.Helper()
	var id ids.PersonID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT person_id FROM person_email WHERE email = $1`, email).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the person for %s: %v", email, err)
	}
	return id
}

// TestPublishingACapturedContactLeavesATrace holds the trail on the write that
// most needs one.
//
// A contact stops being one person's and becomes everybody's. "Which contacts
// were published, when, and on whose authority" is answered from audit_log or
// it is not answered at all — and until this, the visibility flip was a bare
// UPDATE while every other write on the record audited and emitted.
func TestPublishingACapturedContactLeavesATrace(t *testing.T) {
	e := integration.Setup(t)
	const sender = "trace@partner.example"

	seedAttestedOutbound(t, e, "trace-out-1", sender, "trace-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "trace-in-1", sender, "trace-t1")
	personID := personIDFor(t, e, sender)
	if got, _ := personVisibility(t, e, sender); got != "owner" {
		t.Fatalf("a fresh capture is %q, want owner", got)
	}
	before := countVisibilityAudits(t, e, personID)

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no question was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)
	if got, _ := personVisibility(t, e, sender); got != "workspace" {
		t.Fatalf("after a person verdict the contact is %q, want workspace", got)
	}

	if after := countVisibilityAudits(t, e, personID); after != before+1 {
		t.Fatalf("publishing the contact wrote %d audit row(s) naming visibility, want 1: "+
			"nothing records that this contact became visible to the workspace", after-before)
	}
	if n := countOutboxFor(t, e, personID); n == 0 {
		t.Fatal("publishing the contact emitted no event, so nothing downstream learns the " +
			"record changed hands")
	}
}

// countVisibilityAudits counts audit rows on a person naming the visibility
// column, in either image.
func countVisibilityAudits(t *testing.T, e *integration.Env, id ids.PersonID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'person' AND entity_id = $1
			   AND (after ? 'visibility' OR before ? 'visibility')`, id.UUID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting visibility audits: %v", err)
	}
	return n
}

// countOutboxFor counts the events published about a record.
func countOutboxFor(t *testing.T, e *integration.Env, id ids.PersonID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'person.updated'
			   AND envelope->'entity'->>'id' = $1
			   AND envelope->'payload'->'changed_fields' ? 'visibility'`,
			id.UUID.String()).Scan(&n)
	}); err != nil {
		t.Fatalf("counting outbox rows: %v", err)
	}
	return n
}

// TestAnOwnerPublishesTheirOwnCapturedContact is the door out of capture
// privacy for a contact no classifier will ever settle.
//
// The ceiling refuses to ask and the correspondence goes quiet, or the answer
// is `advisor` and the owner disagrees, or they simply know who this is. Until
// this, the only route out of `owner` was a verdict, so those contacts had none.
func TestAnOwnerPublishesTheirOwnCapturedContact(t *testing.T) {
	e := integration.Setup(t)
	const sender = "eigen@partner.example"

	seedAttestedOutbound(t, e, "own-out-1", sender, "own-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "own-in-1", sender, "own-t1")
	personID := personIDFor(t, e, sender)
	if got, _ := personVisibility(t, e, sender); got != "owner" {
		t.Fatalf("a fresh capture is %q, want owner", got)
	}

	store := people.NewStore(InstallationDB(e.Pool))
	if err := store.PromoteOwnCapturedPerson(seatCtx(e, e.Rep1), personID); err != nil {
		t.Fatalf("the owner publishing their own contact: %v", err)
	}
	if got, _ := personVisibility(t, e, sender); got != "workspace" {
		t.Fatalf("after the owner published it the contact is %q, want workspace", got)
	}
	// The trail, on the same terms as a verdict's promotion.
	if n := countVisibilityAudits(t, e, personID); n != 1 {
		t.Fatalf("publishing wrote %d audit row(s) naming visibility, want 1", n)
	}
}

// TestOnlyTheOwnerPublishesACapturedContact is the security case.
//
// Capture privacy is the importing user's, and seniority does not override it:
// an admin reading a colleague's unpromoted captured contacts is precisely the
// disclosure the boundary exists to prevent. A promotion that matched on the
// address rather than on the owner would have published a colleague's contact
// to anybody who could reach this door.
func TestOnlyTheOwnerPublishesACapturedContact(t *testing.T) {
	e := integration.Setup(t)
	const sender = "fremd@partner.example"

	seedAttestedOutbound(t, e, "other-out-1", sender, "other-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "other-in-1", sender, "other-t1")
	personID := personIDFor(t, e, sender)

	store := people.NewStore(InstallationDB(e.Pool))
	for _, seat := range []struct {
		name string
		user ids.UUID
	}{{"a colleague", e.Rep2}, {"an admin", e.AdminUser}} {
		err := store.PromoteOwnCapturedPerson(seatCtx(e, seat.user), personID)
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("%s publishing somebody else's capture-private contact: err = %v, want ErrNotFound — "+
				"a 403 would confirm the contact exists, which is what the boundary hides", seat.name, err)
		}
	}
	if got, _ := personVisibility(t, e, sender); got != "owner" {
		t.Fatalf("the contact is %q after two refused attempts, want owner", got)
	}
}

// seatCtx is one seat acting as themselves, which is the only principal the
// capture-privacy door accepts.
func seatCtx(e *integration.Env, user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(),
		UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestARecordSaysWhoItIsFor puts capture privacy on the wire.
//
// A client could see that it may not WRITE a row and not that the row it is
// reading is private to the person reading it. Without the field a page cannot
// tell "private to you" from "shared with everybody", which is the question the
// owner of a captured contact is actually asking.
//
// It discloses nothing: a caller is only ever sent a row they may already read.
func TestARecordSaysWhoItIsFor(t *testing.T) {
	e := integration.Setup(t)
	const sender = "sicht@partner.example"

	seedAttestedOutbound(t, e, "vis-out-1", sender, "vis-t1")
	captureInboundThroughRealSink(t, e, e.Rep1, "vis-in-1", sender, "vis-t1")
	personID := personIDFor(t, e, sender)

	store := people.NewStore(InstallationDB(e.Pool))
	owner := seatCtx(e, e.Rep1)

	got, err := store.GetPerson(owner, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("the owner reading their own captured contact: %v", err)
	}
	if got.Visibility == nil || *got.Visibility != crmcontracts.PersonVisibilityOwner {
		t.Fatalf("a fresh capture reads %v, want owner — a page cannot say \"private to you\" "+
			"about a field the server never sends", got.Visibility)
	}

	// And it follows the record when the owner publishes it.
	if err := store.PromoteOwnCapturedPerson(owner, personID); err != nil {
		t.Fatalf("publishing: %v", err)
	}
	after, err := store.GetPerson(owner, personID, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if after.Visibility == nil || *after.Visibility != crmcontracts.PersonVisibilityWorkspace {
		t.Fatalf("after publishing it reads %v, want workspace", after.Visibility)
	}

	// The list carries it too: the two paths share one scanner, and a field on
	// only one of them is how a page shows a badge that vanishes on refresh.
	listed, _, err := store.ListPeople(owner, people.ListPeopleInput{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, p := range listed {
		if ids.UUID(p.Id) == personID.UUID {
			if p.Visibility == nil || *p.Visibility != crmcontracts.PersonVisibilityWorkspace {
				t.Fatalf("the listed row reads %v, want workspace", p.Visibility)
			}
			return
		}
	}
	t.Fatal("the published contact is missing from the owner's own list")
}
