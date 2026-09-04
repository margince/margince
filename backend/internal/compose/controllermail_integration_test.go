// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The lane the installation's own mail rides, asserted on the rows it actually
// writes.
//
// This is the seam where three modules meet, so it is tested here rather than in
// any of them: activities files the timeline row, comms stages the delivery,
// consent takes the decision, and the whole point is that they commit together.
//
// The obligation this suite exists for is the one the old direct-SMTP call could
// not meet. That call wrote nothing: no delivery row, no authorization decision,
// no timeline entry — so the one message the installation composes entirely by
// itself appeared in no subject-access export and no erasure reached it.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sealingVault stands where compose's keyvault-backed one stands. It keeps the
// plaintext so a test can assert what was sealed rather than only that something
// was.
type sealingVault struct{ sealed string }

func (v *sealingVault) Put(_ context.Context, secret string) (string, error) {
	v.sealed = secret
	return "sealed-ref", nil
}

// confirmLaneEnv is one subject with a live address and the real lane wired.
type confirmLaneEnv struct {
	e      *integration.Env
	ctx    context.Context
	person ids.UUID
	store  *consent.Store
	vault  *sealingVault
}

// setupConfirmLane wires the REAL machinery, not a double: the delivery staging
// enqueues its dispatch in the same transaction, so a test that replaced it
// would prove nothing about the property this suite is about.
func setupConfirmLane(t *testing.T) *confirmLaneEnv {
	t.Helper()
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	inserter, err := jobs.NewInserter(e.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("jobs.NewInserter: %v", err)
	}
	person := e.SeedPerson(t, "Confirm Subject", &e.Rep1)
	if _, err := e.Pool.Exec(context.Background(), `
		INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		VALUES ($1, 'subject@example.test', true, 'manual', 'human:test')`, person); err != nil {
		t.Fatal(err)
	}
	vault := &sealingVault{}
	return &confirmLaneEnv{
		e:      e,
		ctx:    e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms),
		person: person,
		vault:  vault,
		store: consent.NewStore(InstallationDB(e.Pool)).
			WithConfirmationLane(NewControllerMailQueue(e.Pool, inserter), vault, "https://crm.example.test"),
	}
}

// TestAConfirmLinkStagesADeliveryADecisionAndATimelineRow is the central claim:
// the installation's own mail leaves the same evidence every other message does.
func TestAConfirmLinkStagesADeliveryADecisionAndATimelineRow(t *testing.T) {
	l := setupConfirmLane(t)

	issued, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.PersonKind](l.person))
	if err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	if !issued.Staged {
		t.Fatal("the link was minted but no message was staged")
	}

	if n := l.e.WsCount(t, `SELECT count(*) FROM comms_outbound WHERE sender_kind = 'controller'`); n != 1 {
		t.Errorf("%d controller deliveries, want 1: without a delivery row the message appears in "+
			"no subject-access export and erasure cannot reach it", n)
	}
	// The decision is what makes the lane NOT a side door. A controller message
	// that transmitted without one would be the installation exempting itself
	// from the engine it applies to every rep.
	if n := l.e.WsCount(t, `
		SELECT count(*) FROM communication_decision d
		 JOIN comms_outbound o ON o.id = d.delivery_id AND o.sender_kind = 'controller'`); n != 1 {
		t.Errorf("%d staging decisions for the controller delivery, want 1", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM activity WHERE origin = 'system_notice'`); n != 1 {
		t.Errorf("%d notice activities, want 1: privacy's erasure and the subject-access export "+
			"both reach comms_outbound THROUGH the activity", n)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM river_job`); n != 1 {
		t.Errorf("%d dispatch jobs, want 1: a staged message nothing will carry is a link the "+
			"subject never receives", n)
	}
}

// TestTheEngineAllowsAConfirmationOnItsOwnEvidence pins WHAT the decision says.
//
// A row saying "denied" would satisfy the count above while the message parked
// forever, so the count alone is not the property. The basis is asserted for the
// same reason it is legal_obligation in the validator: a confirmation that
// rested on consent could never be sent to somebody who has not consented yet.
func TestTheEngineAllowsAConfirmationOnItsOwnEvidence(t *testing.T) {
	l := setupConfirmLane(t)

	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.PersonKind](l.person)); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}

	var verdict, category, basis string
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT d.verdict, d.resolved_category, coalesce(d.basis, '')
		  FROM communication_decision d
		  JOIN comms_outbound o ON o.id = d.delivery_id AND o.sender_kind = 'controller'
		 ORDER BY d.decided_at DESC LIMIT 1`).Scan(&verdict, &category, &basis); err != nil {
		t.Fatalf("reading the staging decision: %v", err)
	}
	if verdict != "allow" {
		t.Errorf("the engine answered %q for a confirmation with a live link — the installation "+
			"cannot ask somebody to check what is held about them", verdict)
	}
	if category != "record_confirmation" {
		t.Errorf("resolved category %q, want record_confirmation", category)
	}
	if basis != "legal_obligation" {
		t.Errorf("basis %q, want legal_obligation", basis)
	}
}

// TestTheStagedMessageCarriesNoLiveLink holds the plaintext out of every row a
// human or an export can read.
func TestTheStagedMessageCarriesNoLiveLink(t *testing.T) {
	l := setupConfirmLane(t)

	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.PersonKind](l.person)); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	if l.vault.sealed == "" {
		t.Fatal("nothing was sealed, so there is no link for the dispatcher to substitute")
	}

	// The token itself, as it appears in the sealed link. Searching for it
	// rather than for a URL shape is what makes this catch a leak into a
	// column that stores the token alone.
	_, token, found := lastCut(l.vault.sealed, "#/confirm/")
	if !found {
		t.Fatalf("the sealed value is not a confirm link: %q", l.vault.sealed)
	}

	for _, probe := range []struct {
		what  string
		query string
	}{
		{"the delivery row", `SELECT count(*) FROM comms_outbound WHERE body LIKE '%' || $1 || '%'`},
		{"the timeline", `SELECT count(*) FROM activity WHERE coalesce(body, '') LIKE '%' || $1 || '%'`},
		// Every jsonb an audit row carries, not one of them: the token could
		// land in any, and a probe naming a single column would pass while the
		// leak sat in the next one along.
		{"an audit payload", `
			SELECT count(*) FROM audit_log
			 WHERE coalesce(before::text, '') || coalesce(after::text, '') || coalesce(evidence::text, '')
			       LIKE '%' || $1 || '%'`},
		{"an outbox event", `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`},
	} {
		var n int
		if err := l.e.Pool.QueryRow(context.Background(), probe.query, token).Scan(&n); err != nil {
			t.Fatalf("scanning %s: %v", probe.what, err)
		}
		if n != 0 {
			t.Errorf("the live confirm token reached %s (%d row(s)). Anyone who can read that "+
				"column can open the subject's record without ever holding their mailbox", probe.what, n)
		}
	}
}

// lastCut splits on the final occurrence of sep, so a link whose ORIGIN happens
// to contain the separator still yields the token.
func lastCut(s, sep string) (before, after string, found bool) {
	at := lastIndex(s, sep)
	if at < 0 {
		return s, "", false
	}
	return s[:at], s[at+len(sep):], true
}

func lastIndex(s, sub string) int {
	for i := len(s) - len(sub); i >= 0; i-- {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
