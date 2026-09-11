// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every account-scoped producer, run against the shape capture actually writes.
//
// This is a fitness test, not a feature test: it owns no behaviour of its own
// and asserts nothing about what any producer concludes. It seeds ONE workspace
// the way a connector does — mail filed against a PERSON, the account reachable
// only through that person's employment, and no direct company link
// anywhere — and requires that each producer still finds the account.
//
// A fixture that hand-writes a link no connector emits proves the producer
// against data that does not exist: it passes while the producer finds nothing
// on every real workspace, and nothing about the green tells anyone. Seeding
// what the writer writes is the only way a test can fail for the right reason.
//
// A producer added later belongs in this list. The cost of joining it is a
// couple of lines; the cost of staying out of it is a feature that looks
// finished, passes review, and does nothing.

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// captureShapeClock pins the fixture's instants so a conversation lands on the
// settled side of every window by construction rather than by whatever the wall
// clock says when CI runs.
var captureShapeClock = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

// seedAccountAsCaptureWould builds the one fixture this file is about: an
// account worth working, a contact who works there, and a conversation filed
// against that contact and nobody else.
func seedAccountAsCaptureWould(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	company := e.SeedCompany(t, "Capture Shape Co", &e.Rep1)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employeeOf(t, e, company, "Ada at Capture Shape Co")
	// Old enough for the ghosted rule's fortnight and settled for the
	// extractor's six hours, so one fixture serves every producer.
	seedMessage(t, e, contact, "thread-capture-shape", "Proposal",
		"Sending our proposal over.", "outbound", captureShapeClock.AddDate(0, 0, -30))

	var direct int
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM activity_link
			 WHERE entity_type = 'company'`).Scan(&direct)
	}); err != nil {
		t.Fatalf("count the direct account links: %v", err)
	}
	if direct != 0 {
		t.Fatalf("the fixture wrote %d direct company links; capture writes none, "+
			"and a producer proved against one is proved against nothing", direct)
	}
	return company
}

// The deterministic producer reaches an account it can only see through
// employment. It needs no model, so this half must work on an installation
// that bought none.
func TestTheGhostedRuleReachesAnAccountCaptureLinkedThroughAPerson(t *testing.T) {
	e := Setup(t)
	company := seedAccountAsCaptureWould(t, e)

	pass := ghostedPass(t, e, captureShapeClock)
	if pass.Considered == 0 {
		t.Fatal("the rule considered no account at all: the walk from a captured " +
			"message to its account resolves nothing")
	}
	if pass.Raised != 1 {
		t.Fatalf("the rule wrote %d signals, want the one unanswered tail", pass.Raised)
	}
	if kinds := openSignalKinds(t, e, company); len(kinds) != 1 || kinds[0] != "ghosted_thread" {
		t.Fatalf("the account carries %v, want the ghosted_thread the comparison found", kinds)
	}
}

// The model producer is offered the same conversation. What it concludes is
// the model's business and no assertion is made about it — that the
// conversation reaches the queue at all is the invariant under test.
func TestTheExtractorIsOfferedAConversationCaptureLinkedThroughAPerson(t *testing.T) {
	e := Setup(t)
	seedAccountAsCaptureWould(t, e)

	brain := &scriptedBrain{reply: `{"events": []}`}
	extractor := compose.NewSignalExtractor(e.Pool, brain,
		func() time.Time { return captureShapeClock }, slog.Default())
	pass, err := extractor.RunWorkspace(e.Admin(), ids.From[ids.WorkspaceKind](e.WS))
	if err != nil {
		t.Fatalf("signal extract: %v", err)
	}
	if pass.Due == 0 {
		t.Fatal("the queue offered no conversation: the walk from a captured " +
			"message to its account resolves nothing")
	}
	if brain.calls != 1 {
		t.Fatalf("the model was asked %d times, want the one settled conversation", brain.calls)
	}
}

// The reader-facing side of the same walk. The producers and the timeline the
// account's page shows must agree about which messages belong to it: a signal
// about correspondence the reader cannot find on the page is unanswerable.
//
// It asks activities.CompanyLinkedActivityExists rather than a copy of the three
// arms. A hand-spelled walk here would keep passing against whatever the arms
// used to be, which is the failure this whole file exists to prevent, wearing
// a test's clothes.
func TestTheAccountTimelineCountsMailCaptureLinkedThroughAPerson(t *testing.T) {
	e := Setup(t)
	company := seedAccountAsCaptureWould(t, e)

	var reached int
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM activity a
			 WHERE a.archived_at IS NULL AND `+activities.CompanyLinkedActivityExists(1),
			company).Scan(&reached)
	}); err != nil {
		t.Fatalf("count the account's reachable mail: %v", err)
	}
	if reached != 1 {
		t.Fatalf("the account reaches %d messages, want the one captured against its contact", reached)
	}
}

// A finding drawn from what messages SAY is only as shareable as the messages.
//
// Capture auto-creates contacts owner-private, so a conversation filed against
// nobody else answers to its capturing user alone. The account it reaches can
// still be one the whole workspace sees — that is the ordinary state of a
// promoted account with unpromoted contacts — and the summary the model writes
// about that conversation would then be readable by everyone while the
// correspondence behind it is readable by one person. Capture privacy does not
// yield to row_scope=all, so the summary must not either.
func TestAModelReadOfPrivateMailIsPrivateEvenOnASharedAccount(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Promoted Co", &e.Rep1)
	// The account is the workspace's; the contact is not yet.
	e.WsExec(t, `UPDATE company SET visibility = 'workspace', lifecycle = 'opportunity'
		 WHERE id = $1`, company)
	contact := employeeOf(t, e, company, "Ada Unpromoted")
	e.WsExec(t, `UPDATE person SET visibility = 'owner', owner_id = $2 WHERE id = $1`,
		contact, e.Rep1)
	notice := seedMessage(t, e, contact, "thread-private", "Renewal for 2027",
		"We have decided not to renew.", "inbound", captureShapeClock.Add(-48*time.Hour))

	brain := &scriptedBrain{reply: reply(t, "contract_ended", notice,
		"They wrote that they will not renew.", 0.95)}
	extractor := compose.NewSignalExtractor(e.Pool, brain,
		func() time.Time { return captureShapeClock }, slog.Default())
	if _, err := extractor.RunWorkspace(e.Admin(), ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("signal extract: %v", err)
	}

	var visibility string
	var owner ids.UUID
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT visibility, owner_id FROM signal
			 WHERE resolved_company_id = $1 AND kind = 'contract_ended'`, company).Scan(&visibility, &owner)
	}); err != nil {
		t.Fatalf("read the signal's visibility: %v", err)
	}
	if visibility != "owner" {
		t.Fatalf("a summary of owner-private correspondence is %q, want owner — every "+
			"seat that can see the account would read it", visibility)
	}
	if owner != e.Rep1 {
		t.Fatalf("the signal answers to %s, want the reader whose correspondence it "+
			"summarizes (%s)", owner, e.Rep1)
	}

	// And the gate holds on the way out: a colleague who can see the account
	// cannot read a finding drawn from mail that is not theirs.
	if kinds := openSignalKindsAs(t, e, e.Rep2, company); len(kinds) != 0 {
		t.Fatalf("a colleague reads %v on the shared account, want nothing drawn "+
			"from another person's private mail", kinds)
	}
	if kinds := openSignalKindsAs(t, e, e.Rep1, company); len(kinds) != 1 {
		t.Fatalf("the owner reads %v, want the finding from their own correspondence", kinds)
	}
}

// The deterministic half stays shared, and carries no text to share.
//
// It states what anyone with the account could count for themselves — we spoke
// last, and that was N days ago — so it is the whole workspace's, and it CITES
// the message rather than quoting it. The subject line would be content, and
// content is what the owner-private half exists to hold back.
func TestTheGhostedRuleIsSharedAndQuotesNothing(t *testing.T) {
	e := Setup(t)
	company := seedAccountAsCaptureWould(t, e)
	e.WsExec(t, `UPDATE company SET visibility = 'workspace' WHERE id = $1`, company)

	if pass := ghostedPass(t, e, captureShapeClock); pass.Raised != 1 {
		t.Fatalf("the rule wrote %d signals, want the one unanswered tail", pass.Raised)
	}

	var visibility, snippet string
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT visibility, coalesce(evidence->0->>'snippet', '')
			 FROM signal WHERE resolved_company_id = $1 AND kind = 'ghosted_thread'`,
			company).Scan(&visibility, &snippet)
	}); err != nil {
		t.Fatalf("read the ghosted signal: %v", err)
	}
	if visibility != "workspace" {
		t.Errorf("a comparison over metadata is %q, want workspace — nothing in it "+
			"is anyone's private business", visibility)
	}
	if snippet != "" {
		t.Errorf("the shared finding quotes %q from a message its readers may not "+
			"open; cite the message instead", snippet)
	}
}

// A conversation nobody else may read, whose reader cannot be named, is not
// read at all — and the state it needs can no longer be built.
//
// The visibility decision had two answers and a gap between them. "Shared" and
// "private to this person" are both actionable; "private to nobody in
// particular" was not — and because a signal that names no owner IS a shared
// signal, letting the gap fall through resolved it to the widest audience
// available.
//
// The gap is closed at the estate now: company_owner_private_names_its_owner
// refuses an owner-private account with no owner, so the producer can never be
// handed one (#2137). This test used to seed that row and assert the producer
// declined it. It asserts the refusal instead, because the seed is the thing
// that no longer happens — and a test that quietly stopped being able to build
// its own premise would keep passing while proving nothing.
//
// The producer's own guard is kept. It is unreachable while the constraint
// stands, which is what defence in depth means rather than an argument against
// it: the constraint is one migration from being dropped by somebody who did
// not read this.
func TestAnAccountPrivateToNobodyInParticularCannotBeBuilt(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Unattributable Co", &e.Rep1)

	err := e.WsExecErr(t, `UPDATE company SET visibility = 'owner', owner_id = NULL,
		 lifecycle = 'opportunity' WHERE id = $1`, company)
	if err == nil {
		t.Fatal("an owner-private account with no owner was accepted — a finding on it would have " +
			"no owner to answer to, and a finding with no owner is a SHARED finding, which is the " +
			"widest possible answer to a question the producer cannot answer at all")
	}
	if !strings.Contains(err.Error(), "company_owner_private_names_its_owner") {
		t.Fatalf("the row was refused by something else: %v", err)
	}
}

// The owner comes from whichever record is private, not from contacts alone.
//
// An account can be capture-private too, and a message filed straight against
// one is exactly as unshareable as a message filed against a private contact.
// Reading the owner only off the person left this case answering to nobody,
// which the row then rendered as shared.
func TestAPrivateAccountSuppliesTheReaderItsOwnMailAnswersTo(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Private Co", &e.Rep1)
	e.WsExec(t, `UPDATE company SET visibility = 'owner', owner_id = $2,
		 lifecycle = 'opportunity' WHERE id = $1`, company, e.Rep1)
	notice := seedUnlinkedMessage(t, e, "thread-private-account", "Renewal for 2027",
		"We have decided not to renew.", "inbound", captureShapeClock.Add(-48*time.Hour))
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, company_id)
		VALUES ($1, 'company', $2)`, notice, company)

	brain := &scriptedBrain{reply: reply(t, "contract_ended", notice,
		"They wrote that they will not renew.", 0.95)}
	extractor := compose.NewSignalExtractor(e.Pool, brain,
		func() time.Time { return captureShapeClock }, slog.Default())
	if _, err := extractor.RunWorkspace(e.Admin(), ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("signal extract: %v", err)
	}

	var visibility string
	var owner ids.UUID
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT visibility, owner_id FROM signal
			 WHERE resolved_company_id = $1 AND kind = 'contract_ended'`, company).Scan(&visibility, &owner)
	}); err != nil {
		t.Fatalf("read the signal's visibility: %v", err)
	}
	if visibility != "owner" || owner != e.Rep1 {
		t.Fatalf("a finding from a private account's own mail is %q owned by %s, "+
			"want owner-private to %s", visibility, owner, e.Rep1)
	}
}
