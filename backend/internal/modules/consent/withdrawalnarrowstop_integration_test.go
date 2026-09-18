// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What a stop narrowed to ONE marketing purpose binds, and what it leaves alone.
//
// A withdrawal credential minted for a single subscription is the only link
// whose press must not reach every marketing message the subject gets, and
// communication_suppression.purpose_id is what lets the row say so. These cover
// the press that writes it and the engine that reads it, in both directions:
// what a narrow stop refuses, and what it must go on allowing.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A NAMED-PURPOSE LINK HELD BY A LEAD STOPS ITS OWN LIST, NOT EVERY LIST.
//
// communication_suppression.purpose_id lets a row bind ONE consent_purpose
// rather than every marketing message. Writing this press as a BROAD row —
// what this path did before that column existed — would stop more than the
// recipient asked for and more than the link was issued to do; writing
// nothing at all — what it did in between — silently drops the one opt-out
// this credential exists to give a subject with no per-purpose consent row of
// their own. The correct write is the narrow one, and this asserts both
// halves: the narrow row lands, and no broad row does.
func TestANamedPurposeLeadLinkDoesNotStopAllMarketing(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	purpose := marketingPurposeID(t, e)
	var leadID ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Narrow Lead', $1, 'test', 'human:x') RETURNING id`,
		"narrow-lead@example.test").Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address:   "narrow-lead@example.test",
		LeadID:    ids.From[ids.LeadKind](leadID),
		Scope:     WithdrawalScopeNamedPurpose,
		PurposeID: purpose.UUID,
	})

	stopped, err := e.store.StopForCredential(pressCtx(e), token)
	if err != nil {
		t.Fatalf("the press errored: %v", err)
	}
	if !stopped {
		t.Error("the narrow press answered that it moved nothing, though it withdrew " +
			"the subscription the link names")
	}

	// the narrow stop IS recorded, exactly one row, scoped to the minted purpose
	var narrow int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression
		  WHERE lead_id = $1 AND purpose_id = $2 AND revoked_at IS NULL`,
		leadID, purpose.UUID).Scan(&narrow); err != nil {
		t.Fatalf("counting the narrow stop: %v", err)
	}
	if narrow != 1 {
		t.Errorf("narrow stops scoped to the minted purpose = %d, want 1 — the press this "+
			"link is for was never recorded", narrow)
	}
	// and NO all-marketing row was written (the authority handed was one list)
	var broad int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_suppression
		  WHERE lead_id = $1 AND purpose_id IS NULL AND revoked_at IS NULL`,
		leadID).Scan(&broad); err != nil {
		t.Fatalf("counting the broad stop: %v", err)
	}
	if broad != 0 {
		t.Errorf("a link for one subscription wrote %d broad stop(s) — the lead asked to "+
			"leave one list and every marketing message would stop", broad)
	}

	// AND THE ROW SAYS WHAT WROTE IT. The state alone is a fact with no
	// provenance, and an Art. 15 answer has to say what stopped the list: with
	// no consent row behind a lead's narrow stop, this row is the whole record
	// of it.
	var source string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT source FROM communication_suppression
		  WHERE lead_id = $1 AND purpose_id = $2 AND revoked_at IS NULL`,
		leadID, purpose.UUID).Scan(&source); err != nil {
		t.Fatalf("reading the press's own proof: %v", err)
	}
	if source != sourcePublicLink {
		t.Errorf("the narrow stop records source %q, want the press it came from (%q)",
			source, sourcePublicLink)
	}
}

// A NARROW STOP FROM A REAL PRESS BINDS ONLY ITS OWN PURPOSE, in every
// direction the engine can be asked about a lead: it stops the purpose the
// link was minted for, leaves every OTHER granted purpose exactly as it was,
// and never reaches a send the evidence arms allow on their own ground — a
// lead's own inbound mail resolves no purpose at all (applySuppression in
// authorizesuppression.go), and a narrow row must not touch a send it was
// never asked about.
//
// Seeded through StopForCredential, never a hand-INSERT: a test that wrote
// its own suppression row would prove the SQL narrows correctly and nothing
// about whether the real writer actually calls it that way — which is
// exactly the gap the prior "declines" behaviour hid behind.
func TestANarrowPressBindsOnlyItsOwnPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := context.Background()

	// Two marketing purposes, neither requiring DOI: a lead subject cannot be
	// granted a DOI purpose at all (TestLeadScopedDOIGrantIsRefused), so a
	// plain purpose is what lets both the pressed one and its sibling be
	// granted the same way — the only way a later denial proves the STOP
	// bound rather than merely reflecting a grant that was never there.
	var pressed, other ids.PurposeID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		VALUES ('narrow-bind-newsletter', 'Narrow Bind Newsletter', false)
		RETURNING id`).Scan(&pressed); err != nil {
		t.Fatalf("seeding the pressed purpose: %v", err)
	}
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		VALUES ('narrow-bind-promotions', 'Narrow Bind Promotions', false)
		RETURNING id`).Scan(&other); err != nil {
		t.Fatalf("seeding the other purpose: %v", err)
	}

	const leadEmail = "narrow-bind-lead@example.test"
	var leadID ids.UUID
	if err := e.owner.QueryRow(ctx,
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Narrow Bind Lead', $1, 'test', 'human:x') RETURNING id`,
		leadEmail).Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	lead := ids.From[ids.LeadKind](leadID)

	// setupChannelConsent's own principal grants only "contact" — the
	// subject that environment otherwise tests — so granting consent for a
	// LEAD here needs its own context naming that object.
	leadCtx := principal.WithActor(
		principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws), ids.NewV7()),
		principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
			Permissions: principal.Permissions{
				RoleKeys: []string{"admin"},
				Objects: map[string]principal.ObjectGrant{
					"lead": {Create: true, Read: true, Update: true, Delete: true},
				},
				RowScope: principal.RowScopeAll,
			},
		})
	for _, p := range []ids.PurposeID{pressed, other} {
		if _, err := e.store.Record(leadCtx, RecordInput{
			LeadID: lead, PurposeID: p, NewState: "granted", PolicyText: &grantWording,
		}); err != nil {
			t.Fatalf("granting the lead purpose %s: %v", p, err)
		}
	}

	// A message the lead sent us: the one arm that never consults a purpose
	// key at all (leadconsent_integration_test.go's inboundFromTheLead plants
	// the identical shape — activity_participant carries no lead_id column).
	anchor := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now(), 'gmail', 'human:x')`,
		anchor, "narrow-bind-thread-"+leadID.String()); err != nil {
		t.Fatalf("planting the lead's inbound message: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, contact_id, address, role)
		VALUES ($1, NULL, $2, 'from')`, anchor, leadEmail); err != nil {
		t.Fatalf("planting the participant: %v", err)
	}

	// THE PRESS ITSELF, through the real writer: a named-purpose withdrawal
	// credential scoped to the FIRST purpose, presented at the public
	// unsubscribe door exactly as a mailbox provider's POST would.
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: leadEmail, LeadID: lead,
		Scope: WithdrawalScopeNamedPurpose, PurposeID: pressed.UUID,
	})
	if _, err := e.store.StopForCredential(e.ctx, token); err != nil {
		t.Fatalf("the press errored: %v", err)
	}

	gate := NewGate(e.store)
	recipient := connector.Recipient{Email: leadEmail}
	decide := func(req commsauthz.Request, phase commsauthz.Phase) commsauthz.Decision {
		t.Helper()
		tx, err := e.store.db.Pool().Begin(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Errorf("rolling back: %v", err)
			}
		}()
		d, err := gate.decideOne(e.ctx, tx, recipient, req, phase)
		if err != nil {
			t.Fatalf("deciding: %v", err)
		}
		return d
	}

	// DIRECTION 1: the purpose the link was minted for is now denied, and
	// denied FOR THAT REASON — the grant is live, so only the press explains
	// the refusal.
	denied := decide(commsauthz.Request{LegacyPurposeKey: "narrow-bind-newsletter"}, commsauthz.PhaseTransmit)
	if denied.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict for the pressed purpose = %q (%s), want deny", denied.Verdict, denied.ReasonCode)
	}
	if denied.ReasonCode != commsauthz.ReasonObjection {
		t.Errorf("reason for the pressed purpose = %q, want %q — the grant is untouched, so "+
			"nothing but the press can explain this refusal", denied.ReasonCode, commsauthz.ReasonObjection)
	}

	// DIRECTION 2: a DIFFERENT marketing purpose, granted the same way, is
	// untouched. A broadly-recorded stop would deny this too — the lead asked
	// to leave one list and would find every list stopped.
	allowedOther := decide(commsauthz.Request{LegacyPurposeKey: "narrow-bind-promotions"}, commsauthz.PhaseTransmit)
	if allowedOther.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict for the other purpose = %q (%s), want allow — a narrow stop bound a "+
			"purpose it was never pressed against", allowedOther.Verdict, allowedOther.ReasonCode)
	}

	// DIRECTION 3: the NARROWNESS is what spared the other purpose, and this
	// is the leg that says so.
	//
	// DIRECTION 2 on its own is weak in the one direction that matters: it
	// passes just as well if the press wrote NOTHING, which is the defect
	// #5557 reports. Widening the row this press wrote — the same row, only
	// its purpose_id cleared — must flip that same send to deny. If it does
	// not, either the row is absent or the engine is not reading purpose, and
	// DIRECTION 2 was green for the wrong reason both times.
	if _, err := e.owner.Exec(ctx, `
		UPDATE communication_suppression SET purpose_id = NULL
		 WHERE lead_id = $1 AND revoked_at IS NULL`, lead.UUID); err != nil {
		t.Fatalf("widening the stop this press wrote: %v", err)
	}
	widened := decide(commsauthz.Request{LegacyPurposeKey: "narrow-bind-promotions"}, commsauthz.PhaseTransmit)
	if widened.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("the same send read %q once the row was widened, want deny — the row either "+
			"is not there or the engine never compared its purpose, which would make the "+
			"allow above prove nothing", widened.Verdict)
	}
	if widened.ReasonCode != commsauthz.ReasonObjection {
		t.Errorf("the widened stop denied for %q, want %q", widened.ReasonCode, commsauthz.ReasonObjection)
	}
}

// A NAMED-PURPOSE LINK NAMING ONLY AN ADDRESS STOPS THAT ONE LIST TOO.
//
// A link may be minted against an address with no lead and no contact behind
// it — a recipient the send path knew only as a header — and a narrow stop is
// exactly as recordable for that subject as a broad one: the row carries the
// address and the purpose, and the send gate reads it by address on the next
// message. Refusing here would hand the one recipient with no record of their
// own the one opt-out that does not work, and answering 200 while writing
// nothing would tell them it had.
func TestANamedPurposeLinkNamingOnlyAnAddressStopsThatOneList(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	purpose := marketingPurposeID(t, e)
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address:   "nobody-on-file@example.test",
		Scope:     WithdrawalScopeNamedPurpose,
		PurposeID: purpose.UUID,
	})

	stopped, err := e.store.StopForCredential(pressCtx(e), token)
	if err != nil {
		t.Fatalf("the press errored: %v", err)
	}
	if !stopped {
		t.Error("the press answered that it moved nothing, though it wrote the only record " +
			"of this stop there will ever be")
	}

	var narrow int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = $1 AND contact_id IS NULL AND lead_id IS NULL
		   AND purpose_id = $2 AND revoked_at IS NULL`,
		"nobody-on-file@example.test", purpose.UUID).Scan(&narrow); err != nil {
		t.Fatalf("counting the narrow stop: %v", err)
	}
	if narrow != 1 {
		t.Errorf("narrow stops on the address = %d, want 1 — the press this link is for "+
			"was never recorded", narrow)
	}

	// And nothing BROAD, which is the half a bare count cannot see: the
	// address holds no record to narrow against, and that is not a licence to
	// stop every list instead.
	var broad int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_suppression
		 WHERE lower(address) = $1 AND purpose_id IS NULL AND revoked_at IS NULL`,
		"nobody-on-file@example.test").Scan(&broad); err != nil {
		t.Fatalf("counting the broad stop: %v", err)
	}
	if broad != 0 {
		t.Errorf("a link for one subscription wrote %d broad stop(s) on an address that "+
			"asked to leave one list", broad)
	}
}

// A NAMED-PURPOSE LINK THAT NAMES NO PURPOSE IS NOT MINTABLE, which is what
// keeps the narrow press from ever widening.
//
// The press reads the scope to decide how narrow the row is, and takes the
// purpose off the credential. A credential claiming named_purpose while naming
// none would resolve with a zero purpose, and the press would write the BROAD
// row for a link minted to leave one list — the exact over-stop the column
// exists to prevent, arriving through the one path that looks correct.
//
// It cannot happen, and this says where that is decided: NOT in Go, but in
// withdrawal_credential_scope_names_its_target, which holds
// `(scope = 'named_purpose') = (purpose_id IS NOT NULL)` over every row any
// door has ever written. A second check in validWithdrawalMint would be a
// second writer of one invariant, and the weaker one — it would bind the two
// mint doors while the constraint binds the table. So the mint is asked to do
// the impossible thing here rather than the rule being restated beside it.
func TestANamedPurposeLinkMustNameItsPurpose(t *testing.T) {
	e := setupChannelConsent(t)

	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, mintErr := e.store.EnsureWithdrawalCredentialTx(e.ctx, tx, WithdrawalMintInput{
			Address: "no-purpose-named@example.test",
			Scope:   WithdrawalScopeNamedPurpose,
		})
		return mintErr
	})

	if err == nil {
		t.Fatal("a named-purpose credential minted without a purpose; its press would " +
			"resolve a zero purpose and write the broad all-marketing row for a link " +
			"issued to leave one list")
	}
	// WHICH refusal, because the wrong one would pass this test while leaving
	// the hazard open: a mint refused for want of a grant says nothing about
	// what the table accepts, and would go on passing if the constraint were
	// dropped tomorrow.
	if !strings.Contains(err.Error(), "scope_names_its_target") {
		t.Errorf("the mint was refused with %v, which is not the constraint this rests on — "+
			"a refusal from somewhere else leaves the widening press unproven", err)
	}
}
