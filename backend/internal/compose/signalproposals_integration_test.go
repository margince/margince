// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The offer a signal produces, over a real Postgres.
//
// This is where the ScaleCommerce failure is finally closed. The mail said the
// contract ended, the record said Prospect, and nothing joined them. Now the
// signal states it, the page shows the disagreement, and this offers the fix —
// to a human, who accepts it, edits it, or says no and is not asked again.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// seedAccountAtStage plants the account the whole overhaul is named for,
// filed under a stage.
func seedAccountAtStage(t *testing.T, e *integration.Env, stage string) ids.UUID {
	t.Helper()
	company := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO company (id, display_name, lifecycle, source, captured_by, owner_id)
			VALUES ($1, 'ScaleCommerce', $2, 'gmail:seed', 'connector:gmail', $3)`, company, stage, e.AdminUser)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return company
}

// seedOpenContractEnded plants the signal the extraction site produces when a
// conversation says the relationship is over.
func seedOpenContractEnded(t *testing.T, e *integration.Env, company ids.UUID) ids.UUID {
	t.Helper()
	return seedScopedContractEnded(t, e, company, "company", "We are ending all business with your company.")
}

func seedScopedContractEnded(t *testing.T, e *integration.Env, company ids.UUID, scope, quote string) ids.UUID {
	t.Helper()
	at := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	return seedScopedContractEndedAt(t, e, company, scope, quote, at, at)
}

func seedScopedContractEndedAt(t *testing.T, e *integration.Env, company ids.UUID, scope, quote string, at, detected time.Time, workstream ...string) ids.UUID {
	t.Helper()
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Relationship decision"), Body: &quote, OccurredAt: &at, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, ok := values.ProveQuote(quote, quote)
	if !ok {
		t.Fatal("fixture quote has no source proof")
	}
	named := ""
	if len(workstream) > 0 {
		named = workstream[0]
	}
	fingerprint := ids.NewV7().String()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := signals.RecordDerived(e.Admin(), tx, signals.DerivedSignal{
			Kind: signalKindContractEnded, CompanyID: company, Summary: "The source reports an ending.", Severity: "warn", Fingerprint: fingerprint,
			Evidence: []signals.DerivedEvidence{{ActivityID: ids.UUID(source.Id), Snippet: "Termination reported", SourceQuote: &proof, LifecycleScope: scope, Workstream: named}},
		}, detected)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	signal, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM signal WHERE fingerprint=$1`, fingerprint))
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

// proposePass runs one reconciler pass and reports how many offers it staged.
func proposePass(t *testing.T, e *integration.Env) int {
	t.Helper()
	proposer := NewSignalProposer(e.Pool, approvalsServiceWithEffects(e.Pool),
		slog.New(slog.DiscardHandler))
	staged, err := proposer.RunWorkspace(e.As(e.AdminUser, nil, integration.AdminWithSignals))
	if err != nil {
		t.Fatalf("RunWorkspace: %v", err)
	}
	return staged
}

// stagedOffer reads the one offer standing against an account.
func stagedOffer(t *testing.T, e *integration.Env, company ids.UUID) ids.UUID {
	t.Helper()
	var approvalID ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM approval WHERE kind = 'lifecycle_change' AND target_entity_id = $1 AND status = 'pending'`,
			company).Scan(&approvalID)
	}); err != nil {
		t.Fatalf("reading the staged offer: %v", err)
	}
	return approvalID
}

// accountStage reads what the record currently says it is.
func accountStage(t *testing.T, e *integration.Env, company ids.UUID) string {
	t.Helper()
	var stage string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT lifecycle FROM company WHERE id = $1`, company).Scan(&stage)
	}); err != nil {
		t.Fatalf("reading the account's stage: %v", err)
	}
	return stage
}

// signalStatus reads where the triage of a signal stands.
func signalStatus(t *testing.T, e *integration.Env, signal ids.UUID) string {
	t.Helper()
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM signal WHERE id = $1`, signal).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the signal: %v", err)
	}
	return status
}

// The whole arc: the mail ends the contract, the record still says Prospect, a
// human is asked, and their yes moves the account and settles the signal.
func TestAcceptingTheOfferMovesTheAccountAndSettlesTheSignal(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "prospect")
	signal := seedOpenContractEnded(t, e, company)

	if staged := proposePass(t, e); staged != 1 {
		t.Fatalf("the pass staged %d offers, want the one the contradiction deserves", staged)
	}
	// Staging alone leaves the account unchanged; this path releases it manually.
	if stage := accountStage(t, e, company); stage != "prospect" {
		t.Fatalf("the account moved to %q before anyone decided", stage)
	}

	if _, err := approvalsServiceWithEffects(e.Pool).Decide(
		e.As(e.Rep1, nil, integration.AdminWithSignals),
		ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), true, nil); err != nil {
		t.Fatalf("accepting the offer: %v", err)
	}

	if stage := accountStage(t, e, company); stage != "former_customer" {
		t.Errorf("the account reads %q after the accept, want former_customer", stage)
	}
	if status := signalStatus(t, e, signal); status != "acknowledged" {
		t.Errorf("the signal reads %q after the accept, want acknowledged — a page that "+
			"moved the account while still shouting the signal that moved it contradicts itself",
			status)
	}
}

// An account in set-up or live is one we deliver for, so a mail ending its
// contract asks the same question a customer's would.
func TestAContractEndedMailOffersTheEndToAnAccountWeDeliverFor(t *testing.T) {
	for _, stage := range []string{"onboarding", "live"} {
		t.Run(stage, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, stage)
			seedOpenContractEnded(t, e, company)

			if staged := proposePass(t, e); staged != 1 {
				t.Fatalf("the pass staged %d offers on a %s account, want 1", staged, stage)
			}
			if _, err := approvalsServiceWithEffects(e.Pool).Decide(
				e.As(e.Rep1, nil, integration.AdminWithSignals),
				ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), true, nil); err != nil {
				t.Fatalf("accepting the offer: %v", err)
			}
			if got := accountStage(t, e, company); got != "former_customer" {
				t.Errorf("the account reads %q after the accept, want former_customer", got)
			}
		})
	}
}

// A decider who edits the offer to live still cannot set it: the effect runs as
// the system, and only a signed-in user may say the first scope is in use.
func TestAnOfferEditedToLiveIsRefusedWhenTheSystemAppliesIt(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "onboarding")
	seedOpenContractEnded(t, e, company)
	proposePass(t, e)
	offer := stagedOffer(t, e, company)
	edited := e.WsScalar(t, `
		SELECT jsonb_set(proposed_change, '{proposed_lifecycle}', '"live"')::text
		  FROM approval WHERE id = $1`, offer)

	_, err := approvalsServiceWithEffects(e.Pool).DecideEdited(
		e.As(e.Rep1, nil, integration.AdminWithSignals),
		ids.From[ids.ApprovalKind](offer), []byte(edited))
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Code != "lifecycle_live_needs_a_human" {
		t.Fatalf("an offer edited to live: want the live refusal, got %v", err)
	}
	if got := accountStage(t, e, company); got != "onboarding" {
		t.Errorf("the account reads %q after the edited accept, want onboarding", got)
	}
}

// Saying no means the reader judged the record right and the reading wrong.
// The signal that produced the offer stays open forever, so without a durable
// memory of the refusal the same question comes back every hour.
func TestARefusedOfferIsNotAskedAgain(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	signal := seedOpenContractEnded(t, e, company)

	proposePass(t, e)
	if _, err := approvalsServiceWithEffects(e.Pool).Decide(
		e.As(e.Rep1, nil, integration.AdminWithSignals),
		ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), false, nil); err != nil {
		t.Fatalf("declining the offer: %v", err)
	}

	// Twice, because the refusal must hold on every later pass rather than
	// only the one that immediately follows the decline.
	proposePass(t, e)
	proposePass(t, e)

	if n := e.WsCount(t, `
		SELECT count(*) FROM approval
		 WHERE kind = 'lifecycle_change' AND target_entity_id = $1`, company); n != 1 {
		t.Errorf("%d offers after a decline, want the one that was declined", n)
	}
	if stage := accountStage(t, e, company); stage != "customer" {
		t.Errorf("the account reads %q after a decline, want the stage the reader kept", stage)
	}
	// The reader said the RECORD was right, not that the mail never happened.
	if status := signalStatus(t, e, signal); status != "open" {
		t.Errorf("the signal reads %q after a decline, want it left open — declining the "+
			"consequence is not a claim that the correspondence said something else", status)
	}
}

// An hourly reconciler must not stack the same question in the inbox.
func TestTheSameContradictionIsOfferedOnce(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "opportunity")
	seedOpenContractEnded(t, e, company)

	if standing := proposePass(t, e); standing != 1 {
		t.Fatalf("the first pass left %d offers standing, want 1", standing)
	}
	// The second pass finds the same contradiction and joins the offer it made
	// last time, so one offer still stands — and only one row exists.
	if standing := proposePass(t, e); standing != 1 {
		t.Errorf("a second pass reports %d offers standing on an unchanged account", standing)
	}
	if n := e.WsCount(t, `
		SELECT count(*) FROM approval
		 WHERE kind = 'lifecycle_change' AND target_entity_id = $1`, company); n != 1 {
		t.Errorf("the inbox carries %d copies of one question", n)
	}
}

// A human who fixed the stage by hand while the offer waited has already
// answered it. The offer is pinned to the record's version, so accepting it is
// REFUSED rather than silently spent — the decider is told the record moved
// under them, which is the one thing they need to know before deciding again.
func TestAnAcceptOverACorrectedRecordIsRefused(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "prospect")
	seedOpenContractEnded(t, e, company)
	proposePass(t, e)
	approvalID := stagedOffer(t, e, company)

	// Someone reads the same mail and files the account as disqualified.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE company SET lifecycle = 'disqualified' WHERE id = $1`, company)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	_, err := approvalsServiceWithEffects(e.Pool).Decide(
		e.As(e.Rep1, nil, integration.AdminWithSignals),
		ids.From[ids.ApprovalKind](approvalID), true, nil)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("accepting a stale offer returned %v, want a version conflict — a decider "+
			"acting on a record that moved under them must be told, not obeyed", err)
	}
	if stage := accountStage(t, e, company); stage != "disqualified" {
		t.Errorf("the account reads %q, want the stage the human set — a stale offer must "+
			"not overwrite the edit that answered it", stage)
	}
}

// An account already filed as over is not in conflict with the mail that says
// so, and there is nothing to offer.
func TestNoOfferIsMadeOnAnAccountAlreadyFiledAsOver(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "former_customer")
	seedOpenContractEnded(t, e, company)

	if staged := proposePass(t, e); staged != 0 {
		t.Fatalf("the pass staged %d offers on an account already filed as former_customer", staged)
	}
}

func TestAcceptingSettlesOnlyTheCitedContradiction(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	first := seedOpenContractEnded(t, e, company)
	second := seedOpenContractEnded(t, e, company)
	third := seedOpenContractEnded(t, e, company)

	proposePass(t, e)
	cited := e.WsScalar(t, `SELECT proposed_change->>'signal_id' FROM approval WHERE target_entity_id=$1`, company)
	later := seedScopedContractEndedAt(t, e, company, "company", "We ended all business earlier.",
		time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC))
	if _, err := approvalsServiceWithEffects(e.Pool).Decide(
		e.As(e.Rep1, nil, integration.AdminWithSignals),
		ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), true, nil); err != nil {
		t.Fatalf("accepting the offer: %v", err)
	}

	for _, signal := range []ids.UUID{first, second, third, later} {
		want := "open"
		if signal.String() == cited {
			want = "acknowledged"
		}
		if status := signalStatus(t, e, signal); status != want {
			t.Errorf("signal %s status=%s, want %s", signal, status, want)
		}
	}
}

// teamScopedDecider is a rep who may decide a lifecycle offer — the two
// grants the kind requires — but whose row scope is their TEAM, not the
// workspace. Every fixture above decides as an admin, which is RowScopeAll and
// therefore cannot show what the scope bound does.
var teamScopedDecider = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"company":  {Read: true, Update: true},
		"signal":   {Read: true, Update: true},
		"deal":     {Read: true},
		"contact":  {Read: true},
		"activity": {Read: true},
		"contract": {Read: true},
		"project":  {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func TestHiddenContradictionBlocksAccountLifecycleDecision(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	e.WsExec(t, "UPDATE company SET owner_id = $1 WHERE id = $2", e.Rep1, company)
	mine := seedOpenContractEnded(t, e, company)
	proposePass(t, e)
	theirs := seedOpenContractEndedOnContact(t, e, company, seedCapturePrivateContactOf(t, e, e.Rep3))

	_, err := approvalsServiceWithEffects(e.Pool).Decide(
		e.As(e.Rep1, nil, teamScopedDecider),
		ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), true, nil)
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("incomplete evidence decision error = %v, want version skew", err)
	}
	if got := e.WsScalar(t, "SELECT lifecycle FROM company WHERE id = $1", company); got != "customer" {
		t.Fatalf("account lifecycle = %s, want customer", got)
	}
	for _, signal := range []ids.UUID{mine, theirs} {
		if got := signalStatus(t, e, signal); got != "open" {
			t.Errorf("signal %s status = %s, want open", signal, got)
		}
	}
}

// seedCapturePrivateContactOf plants a contact capture-private to one rep, so
// a signal subjected to it sits outside every other seat's read scope.
func seedCapturePrivateContactOf(t *testing.T, e *integration.Env, owner ids.UUID) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Their contact", &owner)
	e.MakeCapturePrivate(t, "contact", contact, owner)
	return contact
}

// seedOpenContractEndedOnContact files a contradiction that RESOLVES to the
// account but is ABOUT a contact — the shape where resolved_company_id and the
// subject scope disagree.
func seedOpenContractEndedOnContact(t *testing.T, e *integration.Env, company, contact ids.UUID) ids.UUID {
	t.Helper()
	signal := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO signal (id, kind, source_channel, entity_type, entity_id,
			                    resolved_company_id, resolution_state, severity, summary, status,
			                    detected_at, source, captured_by)
			VALUES ($1, 'contract_ended', 'derived', 'contact', $2, $3, 'resolved',
			        'warn', 'Their contact wrote that the renewal will not proceed.', 'open',
			        now(), 'signal-scan', 'agent:contract_ended')`,
			signal, contact, company)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return signal
}

func TestNewEvidenceSupersedesThePendingLifecycleDecision(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	firstAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	seedScopedContractEndedAt(t, e, company, "company", "We ended all business.", firstAt, firstAt)
	if proposePass(t, e) != 1 {
		t.Fatal("first evidence did not stage a decision")
	}
	first := stagedOffer(t, e, company)
	newer := firstAt.AddDate(0, 0, 1)
	seedScopedContractEndedAt(t, e, company, "company", "We confirm the end of all business.", newer, newer)
	if proposePass(t, e) != 1 {
		t.Fatal("new evidence did not stage a decision")
	}
	if got := stagedOffer(t, e, company); got == first {
		t.Fatal("pending decision kept the superseded evidence")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE target_entity_id=$1 AND kind='lifecycle_change' AND status='pending'`, company); got != 1 {
		t.Fatalf("pending decisions=%d, want 1", got)
	}
}

func TestWhitespaceNormalizedSourceProofAllowsTheDecision(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	signal := seedScopedContractEnded(t, e, company, "company", "We are ending\nall business\twith your company.")
	if proposePass(t, e) != 1 {
		t.Fatal("normalized source words failed to stage")
	}
	_, err := approvalsServiceWithEffects(e.Pool).Decide(e.As(e.Rep1, nil, integration.AdminWithSignals), ids.From[ids.ApprovalKind](stagedOffer(t, e, company)), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if accountStage(t, e, company) != "former_customer" || signalStatus(t, e, signal) != "acknowledged" {
		t.Fatal("normalized proof did not settle the cited decision")
	}
}

func TestLifecycleProposalKeepsSourceWordsBehindTheSourceReader(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	quote := "We are ending all business, including Atlas support, with your company."
	at := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	seedScopedContractEndedAt(t, e, company, "company", quote, at, at, "Atlas support")
	if proposePass(t, e) != 1 {
		t.Fatal("fixture did not stage a decision")
	}
	stored := e.WsScalar(t, `SELECT jsonb_build_object('change', proposed_change, 'evidence', evidence)::text FROM approval WHERE id=$1`, stagedOffer(t, e, company))
	if strings.Contains(stored, quote) || strings.Contains(stored, "Atlas support") || strings.Contains(stored, `"occurred_at"`) {
		t.Fatal("proposal copied source words outside source authorization")
	}
	view, err := approvalsServiceWithEffects(e.Pool).GetWire(e.As(e.Rep1, nil, integration.AdminWithSignals), ids.From[ids.ApprovalKind](stagedOffer(t, e, company)))
	if err != nil {
		t.Fatal(err)
	}
	if view.Preview == nil || view.Preview.Evidence == nil || len(*view.Preview.Evidence) == 0 || (*view.Preview.Evidence)[0].EvidenceSnippet != quote {
		t.Fatal("authorized preview did not reconstruct its source quote")
	}
}
