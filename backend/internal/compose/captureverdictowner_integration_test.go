// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The two verdicts about a sender who belongs to the MAILBOX OWNER rather than
// to the business.
//
// A founder's mailbox carries their lawyer and their family alongside their
// customers. Both were previously answerable only with one of six business
// kinds, so the engine said `person` and published them — a founder's sister as
// a workspace contact, and a shareholder negotiation as a colleague's reading.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAdvisorVerdictMakesTheRecordAndKeepsItTheOwnersAlone(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "k.bauer@kanzlei.example", "Entwurf Gesellschaftervereinbarung")
	dispositionID := seedPendingDisposition(t, e, "k.bauer@kanzlei.example", "kanzlei.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindAdvisor}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// `real`, not noise: a lawyer the owner engaged is genuine correspondence,
	// and calling it noise would hide mail the owner wants.
	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusReal {
		t.Fatalf("disposition status = %q, want real", got)
	}
	// The record IS made. Withholding it would lose the owner their own contact.
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		 WHERE pe.email = 'k.bauer@kanzlei.example'`); n != 1 {
		t.Fatalf("%d persons for an advisor verdict, want 1 — the owner keeps their own contact", n)
	}
	// And it stays theirs. This is the whole point of the kind: publishing the
	// record announces to every colleague that the founder has a lawyer.
	if got := visibilityIn(t, e, "k.bauer@kanzlei.example"); got != "owner" {
		t.Fatalf("an advisor's record is %q, want owner — a workspace-visible one discloses the engagement itself", got)
	}
	// The message is not hidden from the owner either.
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, activityID); n != 1 {
		t.Fatal("an advisor verdict archived the message it was judging")
	}
}

func TestAPersonalVerdictMakesNoRecordAtAll(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "s.renner@webmail.example", "Fotos vom Wochenende")
	dispositionID := seedPendingDisposition(t, e, "s.renner@webmail.example", "webmail.example", activityID)

	brain := &scriptedVerdictBrain{verdicts: map[string]string{dispositionID.String(): capture.KindPersonal}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if got := dispositionStatus(t, e, dispositionID); got != capture.PendingStatusNoise {
		t.Fatalf("disposition status = %q, want noise", got)
	}
	// No contact, at any visibility. A family member is not a counterparty of
	// the business and there is nothing here for the CRM to hold.
	if n := countIn(t, e, `
		SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		 WHERE pe.email = 's.renner@webmail.example'`); n != 0 {
		t.Fatalf("%d persons for a personal verdict, want 0", n)
	}
	// The domain is NOT marked company-refused. A refusal is a statement about
	// a business, and a consumer mail host is not one — nor is the private
	// domain a family member might write from something to publish a verdict
	// about.
	if n := countIn(t, e, `
		SELECT count(*) FROM organization_domain_disposition
		 WHERE domain = 'webmail.example' AND status = 'suppressed'`); n != 0 {
		t.Fatal("a personal verdict suppressed the sender's domain — that is a claim about a company, not about a family member")
	}
	// The mail itself survives this change. Destroying it is the purge's job,
	// and the purge has an undo window in front of it; a verdict that deleted
	// on the spot would leave the owner nothing to undo.
	if n := countIn(t, e, `SELECT count(*) FROM activity WHERE id = $1`, activityID); n != 1 {
		t.Fatal("a personal verdict destroyed the message; destruction belongs to the purge, behind its undo window")
	}
}

// visibilityIn answers how a person record is scoped. It fails when there is no
// such person, so a test that meant to find one cannot pass on an empty string.
func visibilityIn(t *testing.T, e *integration.Env, email string) string {
	t.Helper()
	var visibility string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.visibility FROM person p
			  JOIN person_email pe ON pe.person_id = p.id
			 WHERE pe.email = $1 AND p.archived_at IS NULL`, email).Scan(&visibility)
	})
	if err != nil {
		t.Fatalf("reading the visibility of %s: %v", email, err)
	}
	return visibility
}
