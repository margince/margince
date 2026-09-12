// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Checking a composed message against what its jurisdiction requires.
//
// gates/messagingruleapplied_test.go carried "nothing prepends it" about
// SubjectPrefix since the Vietnamese pack shipped: Decree 91/2020 fixes a [QC]
// label for advertising mail, the pack declares it, and no step between a
// composed subject and the provider ever consulted the rules. An advertising
// message left unmarked and nothing reported it.

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// labelledIn binds the store to a pack fixing an advertising label.
// composerCtx is a seat that may read the timeline a delivery hangs off, which
// is what CheckMessage asks for.
//
// A REAL GRANT rather than a system principal, because the gate is what these
// tests would otherwise not exercise: an ungated check would let anybody
// holding a delivery id learn whether it was judged as advertising by watching
// a finding appear and disappear.
func composerCtx(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"activity": {Read: true},
	}
	return principal.WithActor(e.ctx, actor)
}

// THE CODE IS MINTED, never written down. testJurisdiction (authorizecaplock)
// hands out the qm-qz range from a shared counter, and this package's cap tests
// draw from the same one — so a literal here takes a code that helper will hand
// to somebody else, and Register panics for every test after it. The failure
// passes alone and fails in the suite, which is the worst shape to diagnose.
//
// The label is the decree's own, which is why it is a constant rather than a
// parameter: a test that could choose it would be testing string comparison,
// and what these tests are about is that the pack's label reaches the check.
func labelledIn(t *testing.T, e *channelConsentEnv) *Store {
	t.Helper()
	code := testJurisdiction(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: code, Version: 1, SubjectPrefix: "[QC]",
	})
	return e.store.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return code, nil
		}))
}

// judgedAs plants a delivery the engine resolved to these categories, one
// decision row per recipient, and answers the delivery and its decision set.
//
// THE SET is what a requirement is judged against: a delivery can hold several,
// and reading by delivery alone would judge a message by a category it no
// longer has.
func judgedAs(
	t *testing.T, e *channelConsentEnv, categories ...commsauthz.Category,
) (string, string) {
	t.Helper()
	activity := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'email', 'outbound', 'manual', now(), 'human:x')`, activity); err != nil {
		t.Fatalf("planting the activity the delivery hangs off: %v", err)
	}
	delivery := ids.NewV7()
	// The mail shape the table's own CHECK demands: a message id, an envelope
	// and a subject. A channel delivery is the other arm and carries none of
	// them, which is not what a subject-prefix requirement is about.
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO comms_outbound (id, activity_id, user_id, provider, message_id, recipients, cc,
		                            subject, references_chain, body, status, sender_kind,
		                            consent_purpose)
		VALUES ($1, $2, $3, 'test', $4, '["someone@corp.test"]'::jsonb, '[]'::jsonb,
		        'planted', '[]'::jsonb, 'body', 'pending', 'user', 'newsletter')`,
		delivery, activity, e.user, "<"+delivery.String()+"@test>"); err != nil {
		t.Fatalf("planting the delivery: %v", err)
	}
	setID := ids.NewV7()
	for i, category := range categories {
		if _, err := e.owner.Exec(context.Background(), `
			INSERT INTO communication_decision
			  (delivery_id, attempt, decision_set_id, recipient_address, phase,
			   resolved_category, verdict, reason_code, mode, actor)
			VALUES ($1, 0, $2, $3, 'staging', $4, 'allow', '', 'enforce', 'test')`,
			delivery, setID, fmt.Sprintf("someone%d@corp.test", i),
			string(category)); err != nil {
			t.Fatalf("planting the decision: %v", err)
		}
	}
	return delivery.String(), setID.String()
}

// TestAnAdvertisingSubjectWithoutItsLabelIsAFinding is the register line this
// closes.
func TestAnAdvertisingSubjectWithoutItsLabelIsAFinding(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, set := judgedAs(t, e, commsauthz.CategoryMarketing)

	findings, err := store.CheckMessage(composerCtx(e), delivery, set, "Half price this week")
	if err != nil {
		t.Fatalf("checking the message: %v", err)
	}
	if len(findings) != 1 || findings[0].Requirement != RequirementSubjectPrefix {
		t.Fatalf("findings = %+v, want one subject_prefix — an advertising message went out "+
			"unmarked and nothing reported it, which is the defect this closes", findings)
	}
	if findings[0].Detail == "" {
		t.Error("the finding says nothing about what was expected")
	}
}

// TestALabelledAdvertisingSubjectIsNoFinding is the positive control: the
// finding above must be about the missing label and not about the check
// refusing everything.
func TestALabelledAdvertisingSubjectIsNoFinding(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, set := judgedAs(t, e, commsauthz.CategoryMarketing)

	for _, subject := range []string{
		"[QC] Half price this week",
		"[qc] half price",
		"  [QC] leading space",
	} {
		findings, err := store.CheckMessage(composerCtx(e), delivery, set, subject)
		if err != nil {
			t.Fatalf("checking %q: %v", subject, err)
		}
		if len(findings) != 0 {
			t.Errorf("%q was reported as %+v — a message carrying the label was refused, and "+
				"a case difference is not what the decree is about", subject, findings)
		}
	}
}

// TestAnInvoiceNeverCarriesTheAdvertisingLabel.
//
// Putting it there would tell a recipient their payment reminder is an
// advertisement: a false statement made to satisfy a check, which is worse than
// the omission it would be fixing.
func TestAnInvoiceNeverCarriesTheAdvertisingLabel(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, set := judgedAs(t, e, commsauthz.CategoryInvoiceOrPayment)

	findings, err := store.CheckMessage(composerCtx(e), delivery, set, "Your April invoice")
	if err != nil {
		t.Fatalf("checking the invoice: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("an invoice was told to carry an advertising label: %+v", findings)
	}
}

// TestAnInstallationUnderNoPackOwesNothing. Nothing owed and nothing checked
// are the same value and different facts; the caller parks on a finding, so
// answering one here would refuse lawful mail.
func TestAnInstallationUnderNoPackOwesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	delivery, set := judgedAs(t, e, commsauthz.CategoryMarketing)

	findings, err := e.store.CheckMessage(composerCtx(e), delivery, set, "Half price this week")
	if err != nil {
		t.Fatalf("checking with no country declared: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("an installation declaring no country was told it owes %+v", findings)
	}
}

// TestADeliveryTheEngineNeverJudgedOwesNothing. Parking mail on an absence of
// evidence rather than on evidence of absence would refuse deliveries this
// check knows nothing about.
func TestADeliveryTheEngineNeverJudgedOwesNothing(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)

	findings, err := store.CheckMessage(composerCtx(e), ids.NewV7().String(), ids.NewV7().String(), "Half price this week")
	if err != nil {
		t.Fatalf("checking an unjudged delivery: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("a delivery with no decision was told it owes %+v", findings)
	}
}

// TestAMessageAdvertisingToOnlySomeRecipientsIsNotLabelled is Codex's finding.
//
// A subject line is ONE line for everybody it reaches. A send read as an
// active-deal follow-up for one addressee and as marketing for another would be
// labelled for both or for neither, and neither answer is right — so the
// requirement does not fire, and the composition that produced the mix is the
// thing to fix rather than the label.
//
// The alternative, firing on any marketing recipient, parks the message on a
// label that cannot be correct for everyone.
func TestAMessageAdvertisingToOnlySomeRecipientsIsNotLabelled(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, set := judgedAs(t, e,
		commsauthz.CategoryMarketing, commsauthz.CategoryActiveDealFollowup)

	findings, err := store.CheckMessage(composerCtx(e), delivery, set, "How did the trial go?")
	if err != nil {
		t.Fatalf("checking the mixed send: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("a send that is advertising to only some of its recipients was told to "+
			"carry an advertising label: %+v — the same subject line reaches the addressee "+
			"it is not advertising to", findings)
	}
}

// TestAStaleDecisionSetDoesNotJudgeThisSend.
//
// A delivery can hold several decision sets: a paced message redispatched after
// its circumstances changed writes a fresh one beside the old. Reading by
// delivery id alone picked among them, so a send authorized as marketing could
// be judged by an earlier non-marketing row and go out unlabelled.
func TestAStaleDecisionSetDoesNotJudgeThisSend(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, stale := judgedAs(t, e, commsauthz.CategoryActiveDealFollowup)

	// The set this send is actually going out on, written beside the old one
	// exactly as a redispatch does.
	live := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 0, $2, 'someone@corp.test', 'staging', 'marketing', 'allow', '', 'enforce', 'test')`,
		delivery, live); err != nil {
		t.Fatalf("planting the live decision: %v", err)
	}

	findings, err := store.CheckMessage(composerCtx(e), delivery, live.String(), "Half price this week")
	if err != nil {
		t.Fatalf("checking the redispatched send: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %+v, want one: the live set says marketing, and an earlier set "+
			"on the same delivery must not answer for it", findings)
	}

	// And the stale set still answers for itself, which is what proves the
	// binding is to the set rather than to whichever row sorts first.
	stale2, err := store.CheckMessage(composerCtx(e), delivery, stale, "Half price this week")
	if err != nil {
		t.Fatalf("checking against the stale set: %v", err)
	}
	if len(stale2) != 0 {
		t.Errorf("the stale set reported %+v, want none", stale2)
	}
}

// TestACallerWhoCannotReadTheTimelineLearnsNothing.
//
// The answer varies with what the engine decided about a delivery, so an
// ungated exported method would let anybody holding a delivery id learn whether
// it was judged as advertising by watching a finding appear and disappear. The
// send worker is a system principal and passes; a seat without the grant does
// not.
func TestACallerWhoCannotReadTheTimelineLearnsNothing(t *testing.T) {
	e := setupChannelConsent(t)
	store := labelledIn(t, e)
	delivery, set := judgedAs(t, e, commsauthz.CategoryMarketing)

	actor, _ := principal.Actor(e.ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{}
	stranger := principal.WithActor(e.ctx, actor)

	if _, err := store.CheckMessage(stranger, delivery, set, "Half price this week"); err == nil {
		t.Fatal("a caller with no activity grant was told what this delivery was judged as")
	}
}
