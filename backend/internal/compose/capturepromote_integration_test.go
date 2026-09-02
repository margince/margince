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
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
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
func seedAttestedOutbound(t *testing.T, e *integration.Env, sourceID, counterparty string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id,
			                      source, captured_by, counterparty_email, counterparty_outbound_attested)
			VALUES ('email', 'Angebot', 'Anbei.', 'outbound', 'gmail', $1,
			        'gmail:'||$1, 'connector:gmail', $2, true)`, sourceID, counterparty)
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
	t *testing.T, e *integration.Env, owner ids.UUID, sourceID, counterparty string,
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
		Source: "gmail:" + sourceID, CapturedBy: "connector:gmail",
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
	seedAttestedOutbound(t, e, "promote-out-1", sender)
	// Then they reply, and the ladder creates the person on sight.
	captureInboundThroughRealSink(t, e, e.Rep1, "promote-in-1", sender)

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

	seedAttestedOutbound(t, e, "advisor-out-1", sender)
	captureInboundThroughRealSink(t, e, e.Rep1, "advisor-in-1", sender)

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

	seedAttestedOutbound(t, e, "settled-out-1", sender)
	captureInboundThroughRealSink(t, e, e.Rep1, "settled-in-1", sender)

	dispositionID, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatal("no verdict was opened for a corresponded sender")
	}
	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPerson}}
	runVerdict(t, e, brain)

	// They write again, long after the answer.
	captureInboundThroughRealSink(t, e, e.Rep1, "settled-in-2", sender)

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

	seedAttestedOutbound(t, e, "supplier-out-1", sender)
	captureInboundThroughRealSink(t, e, e.Rep1, "supplier-in-1", sender)

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
	var admission string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(context.Background(),
			`SELECT coalesce(admission, '') FROM organization_domain_disposition WHERE domain = $1`,
			"supplier.example").Scan(&admission)
		if err == pgx.ErrNoRows {
			admission = ""
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the domain admission: %v", err)
	}
	if admission == people.DomainSuppressed {
		t.Error("a newsletter verdict refused a company the workspace corresponds with — " +
			"hiding the blast is right, refusing the supplier on the strength of it is not")
	}
}
