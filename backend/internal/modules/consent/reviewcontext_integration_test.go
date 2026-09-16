// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A review must bind a stop the way the send path binds it: by the purpose the
// SEND resolved to, not merely by category.
//
// applySuppression is purpose-aware: a narrow marketing objection binds only
// the one purpose it was pressed against. aStopThatBindsTheMessage
// — the review path's own reader, asked when a rep tries to answer a refusal
// with a statement of fact — could not make the same comparison: nothing
// carried the send's resolved purpose as far as the stored refusal, so it
// treated ANY live stop as binding EVERY marketing refusal for the subject.
// A lead refused for one purpose could not be answered because they had
// objected to a different one — over-refusal the send path itself does not
// commit.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// TestAReviewBindsAStopByTheSendsResolvedPurpose is both directions in one
// test, because the risk is symmetric: a fix that only widens could stop
// binding a stop that DOES apply, which is the opposite failure and just as
// wrong.
//
// EVERYTHING IS SEEDED THROUGH THE REAL WRITER. The stop is pressed through
// StopForCredential exactly as a mailbox provider's POST would press it
// (withdrawalnarrowstop_integration_test.go's TestANarrowPressBindsOnlyItsOwnPurpose
// is the harness this borrows). The refused decision comes from gate.decideOne
// itself, not a hand-built commsauthz.Decision, so the test also proves
// decideOne/decideLead actually carry PurposeID onto the decision rather than
// only using it locally. The review comes from OpenReviewTx, so the refusal
// this test reads has round-tripped through the same jsonb column a rep's
// screen reads from.
func TestAReviewBindsAStopByTheSendsResolvedPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := e.ctx

	// Two plain marketing purposes, neither granted for the lead. Neither
	// needs to be: what refuses each send here is the ABSENCE of a grant
	// (leadRefusalReason's default, ReasonNoMarketingConsent) — a reason with
	// nothing to do with either purpose's stop, so a decision denied for it
	// still proves the purpose comparison rather than piggy-backing on it.
	var pressed, other ids.PurposeID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		VALUES ('review-bind-newsletter', 'Review Bind Newsletter', false)
		RETURNING id`).Scan(&pressed); err != nil {
		t.Fatalf("seeding the pressed purpose: %v", err)
	}
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		VALUES ('review-bind-promotions', 'Review Bind Promotions', false)
		RETURNING id`).Scan(&other); err != nil {
		t.Fatalf("seeding the other purpose: %v", err)
	}

	const leadEmail = "review-bind-lead@example.test"
	var leadID ids.UUID
	if err := e.owner.QueryRow(ctx,
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Review Bind Lead', $1, 'test', 'human:x') RETURNING id`,
		leadEmail).Scan(&leadID); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	lead := ids.From[ids.LeadKind](leadID)

	// THE PRESS ITSELF, through the real writer: a named-purpose withdrawal
	// credential scoped to the PRESSED purpose, presented exactly as a
	// mailbox provider's one-click POST would present it. StopForCredentialTx
	// refuses a contact's link outright (withdrawalpress.go), so this is the
	// lead-only shape a narrow stop can take today.
	token := mintWithdrawal(t, e, WithdrawalMintInput{
		Address: leadEmail, LeadID: lead,
		Scope: WithdrawalScopeNamedPurpose, PurposeID: pressed.UUID,
	})
	if _, err := e.store.StopForCredential(e.ctx, token); err != nil {
		t.Fatalf("the press errored: %v", err)
	}

	gate := NewGate(e.store)
	recipient := connector.Recipient{Email: leadEmail}
	decide := func(purposeKey string) commsauthz.Decision {
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
		d, err := gate.decideOne(e.ctx, tx, recipient, commsauthz.Request{LegacyPurposeKey: purposeKey}, commsauthz.PhaseTransmit)
		if err != nil {
			t.Fatalf("deciding: %v", err)
		}
		return d
	}

	pressedDecision := decide("review-bind-newsletter")
	if pressedDecision.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict for the pressed purpose = %q (%s), want deny", pressedDecision.Verdict, pressedDecision.ReasonCode)
	}
	if pressedDecision.PurposeID == nil || *pressedDecision.PurposeID != pressed.UUID {
		t.Fatalf("decideOne's own PurposeID = %v, want the pressed purpose %s — the send's resolved "+
			"purpose never reached the decision", pressedDecision.PurposeID, pressed.UUID)
	}

	otherDecision := decide("review-bind-promotions")
	if otherDecision.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict for the other purpose = %q (%s), want deny (no grant, unrelated to the stop)",
			otherDecision.Verdict, otherDecision.ReasonCode)
	}
	if otherDecision.PurposeID == nil || *otherDecision.PurposeID != other.UUID {
		t.Fatalf("decideOne's own PurposeID = %v, want the other purpose %s", otherDecision.PurposeID, other.UUID)
	}

	openReview := func(d commsauthz.Decision) Review {
		t.Helper()
		var out Review
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			out, err = OpenReviewTx(ctx, tx, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{d}}, ids.Nil)
			return err
		}); err != nil {
			t.Fatalf("opening the review: %v", err)
		}
		if len(out.Refusals) != 1 {
			t.Fatalf("refusals = %+v, want exactly one", out.Refusals)
		}
		return out
	}

	pressedReview := openReview(pressedDecision)
	otherReview := openReview(otherDecision)

	// The subject key aStopThatBindsTheMessage is asked with is typed
	// ids.ContactID (RecordContext's own caller shape), but the query beneath
	// it matches contact_id, lead_id or the bare address alike — the same
	// entity-agnostic keying the send path uses. A lead's id travels under
	// that type here for exactly that reason.
	subject := ids.From[ids.ContactKind](leadID)

	// DIRECTION 1: the review for the SAME purpose the stop was pressed
	// against. The stop binds, and names itself.
	binding, err := aStopThatBindsTheMessage(ctx, e.store.db, subject, pressedReview.Refusals[0])
	if err != nil {
		t.Fatalf("asking whether a stop binds the pressed-purpose review: %v", err)
	}
	if binding != commsauthz.ReasonObjection {
		t.Errorf("binding for the pressed purpose's own review = %q, want %q — the narrow stop should "+
			"refuse a rep answering a refusal for the very purpose it was pressed against",
			binding, commsauthz.ReasonObjection)
	}

	// DIRECTION 2: the review for a DIFFERENT purpose. Before this change,
	// the review path had no send-purpose to compare and treated any live
	// stop as binding every marketing refusal — this is the over-refusal the
	// fix removes.
	binding, err = aStopThatBindsTheMessage(ctx, e.store.db, subject, otherReview.Refusals[0])
	if err != nil {
		t.Fatalf("asking whether a stop binds the other-purpose review: %v", err)
	}
	if binding != "" {
		t.Errorf("binding for a review about a DIFFERENT purpose = %q, want none — a stop pressed "+
			"against one marketing purpose must not refuse a rep answering a refusal about another",
			binding)
	}
}
