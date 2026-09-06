// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The lead arm of consent (E12.20) over a real migrated Postgres: a
// grant recorded against a lead lands lead-scoped (person_id NULL),
// stays idempotent on re-assertion, reads back through LeadConsent, is
// refused for DOI purposes (the round-trip is person-keyed), and
// authorizes the outbound gate for the lead's email.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type leadConsentEnv struct {
	owner      *pgx.Conn
	store      *Store
	ctx        context.Context
	ws, user   ids.UUID
	newsletter ids.PurposeID
	doiNews    ids.PurposeID
	lead       ids.LeadID
	leadEmail  string
}

func setupLeadConsent(t *testing.T) *leadConsentEnv {
	t.Helper()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	appDSN := os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	// To head before anything else touches this database: testdb.Pool refuses
	// until EnsureSchema has run, and EnsureSchema still REBUILDS whenever it
	// cannot prove the database is a fresh lane clone — so a seed written
	// before it would be dropped rather than reset.
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	// Every test in this package seeds its own workspace into ONE database, so
	// the separation between them has to be real: reset before seeding, as
	// compose/integration's harness does.
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &leadConsentEnv{
		owner: owner,
		ws:    ids.NewV7(), user: ids.NewV7(),
		newsletter: ids.New[ids.PurposeKind](),
		doiNews:    ids.New[ids.PurposeKind](),
		lead:       ids.New[ids.LeadKind](),
	}
	e.leadEmail = "lena-" + e.lead.String() + "@warm.example"
	if _, err := owner.Exec(ctx,
		`INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, e.user, "rep-"+e.user.String()+"@lc.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO consent_purpose (id, key, label, requires_double_opt_in)
		VALUES ($1, 'newsletter', 'Newsletter', false), ($2, 'doi_newsletter', 'DOI Newsletter', true)`,
		e.newsletter, e.doiNews); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO lead (id, full_name, email, status, source, captured_by)
		 VALUES ($1, 'Lena Lead', lower($2), 'contacted', 'inbound', 'human:x')`,
		e.lead, e.leadEmail); err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	// Registered where the pool is handed out, before the test adds any cleanup
	// of its own, so it runs last and sees a package that has genuinely stopped.
	// The pool outlives the test now, so a goroutine still holding a connection
	// would go on writing into the database the NEXT test just reset.
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))

	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	e.ctx = principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"lead":   {Create: true, Read: true, Update: true, Delete: true},
				"person": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return e
}

func TestLeadScopedConsentRecordsProofAndReadsBack(t *testing.T) {
	e := setupLeadConsent(t)

	state, err := e.store.Record(e.ctx, RecordInput{
		LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	})
	if err != nil {
		t.Fatalf("recording a lead-scoped grant: %v", err)
	}
	if state.State != "granted" || state.PurposeKey != "newsletter" {
		t.Fatalf("recorded state = %+v", state)
	}

	// The state row is lead-scoped: lead arm set, person arm NULL.
	var personArm *ids.UUID
	var rowState string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT person_id, state FROM person_consent WHERE lead_id = $1 AND purpose_id = $2`,
		e.lead, e.newsletter).Scan(&personArm, &rowState); err != nil {
		t.Fatalf("reading the state row: %v", err)
	}
	if personArm != nil || rowState != "granted" {
		t.Fatalf("state row = (person_id=%v, state=%q), want a lead-scoped granted row", personArm, rowState)
	}

	// Re-asserting the same state appends no second proof row.
	if _, err := e.store.Record(e.ctx, RecordInput{
		LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	}); err != nil {
		t.Fatalf("re-asserting the grant: %v", err)
	}
	var proofRows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_event WHERE lead_id = $1 AND purpose_id = $2`,
		e.lead, e.newsletter).Scan(&proofRows); err != nil {
		t.Fatal(err)
	}
	if proofRows != 1 {
		t.Fatalf("proof rows after idempotent re-assert = %d, want 1", proofRows)
	}

	// The lead arm of the read answers granted for the purpose and the
	// honest unknown for the untouched DOI purpose.
	states, events, err := e.store.LeadConsent(e.ctx, e.lead)
	if err != nil {
		t.Fatalf("LeadConsent: %v", err)
	}
	byKey := map[string]string{}
	for _, st := range states {
		byKey[st.PurposeKey] = st.State
	}
	if byKey["newsletter"] != "granted" || byKey["doi_newsletter"] != "unknown" {
		t.Fatalf("lead consent states = %v", byKey)
	}
	if len(events) != 1 {
		t.Fatalf("lead proof log length = %d, want 1", len(events))
	}
}

func TestLeadScopedDOIGrantIsRefused(t *testing.T) {
	e := setupLeadConsent(t)
	_, err := e.store.Record(e.ctx, RecordInput{
		LeadID: e.lead, PurposeID: e.doiNews, NewState: "granted",
		PolicyText: &grantWording,
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("a DOI grant on a lead subject: got %v, want a ValidationError (the round-trip is person-keyed)", err)
	}
}

func TestOutboundGateAcceptsTheLeadArm(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)

	// Default-deny before the grant…
	if err := gate.RequireGrantedForEmails(e.ctx, []string{e.leadEmail}, "newsletter"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("pre-grant gate: %v, want ErrConsentNotGranted", err)
	}
	if _, err := e.store.Record(e.ctx, RecordInput{
		LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	}); err != nil {
		t.Fatal(err)
	}
	// …and the lead-scoped grant authorizes exactly that purpose.
	if err := gate.RequireGrantedForEmails(e.ctx, []string{e.leadEmail}, "newsletter"); err != nil {
		t.Fatalf("post-grant gate: %v, want pass", err)
	}
	if err := gate.RequireGrantedForEmails(e.ctx, []string{e.leadEmail}, "doi_newsletter"); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("a grant for one purpose authorized another: %v", err)
	}
}

// The ENGINE's own answer about a lead, which is a different question from the
// legacy gate's and had no answer at all until now.
//
// Without a lead arm the engine returned `review`/no-subject for every
// lead-only recipient. That was harmless while the engine only observed and
// would have become an inversion the day a category moved to enforce: the
// conjunction would refuse exactly the sends the legacy gate allows, so a
// rollout step meant to tighten marketing would have silently stopped ordinary
// correspondence with every unpromoted lead.
func TestTheEngineAnswersAboutALeadRatherThanShrugging(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	recipient := connector.Recipient{Email: e.leadEmail}

	tx, err := e.store.db.Pool().Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	// Before the grant the engine denies, and names WHY: no consent, not "we
	// could not find anybody". The distinction is the whole point — a
	// no-subject verdict is absolute and would deny in every mode.
	before, err := gate.decideOne(e.ctx, tx, recipient, commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding about an ungranted lead: %v", err)
	}
	if before.SubjectKind != entityLead {
		t.Errorf("subject kind = %q, want lead — the engine did not recognise the subject", before.SubjectKind)
	}
	if before.Verdict != commsauthz.VerdictDeny {
		t.Errorf("verdict = %q, want deny for a lead with no grant", before.Verdict)
	}
	if before.ReasonCode == commsauthz.ReasonNoSubject {
		t.Error("an identified lead was recorded as nobody, which denies in every mode")
	}

	if _, err := e.store.Record(e.ctx, RecordInput{
		LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted",
		PolicyText: &grantWording,
	}); err != nil {
		t.Fatal(err)
	}

	after, err := gate.decideOne(e.ctx, tx, recipient, commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding about a granted lead: %v", err)
	}
	if after.Verdict != commsauthz.VerdictAllow {
		t.Errorf("verdict = %q (%s), want allow — the engine refused what the legacy gate allows",
			after.Verdict, after.ReasonCode)
	}
	if after.SubjectID != e.lead.UUID {
		t.Errorf("subject id = %v, want the lead's own id %v", after.SubjectID, e.lead.UUID)
	}
}

// The engine and the legacy gate agree about a lead, which is what makes it
// safe to enforce. Two implementations of "may we write to this lead" would be
// two answers, and the one that stopped matching would look exactly like the
// one that still did.
func TestTheEngineAndTheLegacyGateAgreeAboutALead(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	recipient := connector.Recipient{Email: e.leadEmail}

	for _, step := range []struct {
		name  string
		grant bool
	}{{"before the grant", false}, {"after the grant", true}} {
		if step.grant {
			if _, err := e.store.Record(e.ctx, RecordInput{
				LeadID: e.lead, PurposeID: e.newsletter, NewState: "granted",
				PolicyText: &grantWording,
			}); err != nil {
				t.Fatal(err)
			}
		}
		legacyErr := gate.RequireGrantedForEmails(e.ctx, []string{e.leadEmail}, "newsletter")
		legacyAllows := legacyErr == nil

		tx, err := e.store.db.Pool().Begin(e.ctx)
		if err != nil {
			t.Fatal(err)
		}
		d, err := gate.decideOne(e.ctx, tx, recipient, commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
		if err != nil {
			t.Fatalf("%s: deciding: %v", step.name, err)
		}
		if err := tx.Rollback(e.ctx); err != nil {
			t.Fatalf("%s: rolling back: %v", step.name, err)
		}
		engineAllows := d.Verdict == commsauthz.VerdictAllow

		if legacyAllows != engineAllows {
			t.Errorf("%s: legacy allows=%v, engine allows=%v (%s) — enforcing this category would change who can be written to",
				step.name, legacyAllows, engineAllows, d.ReasonCode)
		}
	}
}

// A lead who withdrew is told they withdrew.
//
// A withdrawal and an absence are different things that happened — Art. 7(3)
// against default-deny — and the reason code is what the subject reads when
// they ask. The lead arm first reported every refusal as an absence, so a lead
// who had exercised their right to withdraw would have been told the
// installation merely never held consent: a false statement in a record Art. 15
// discloses. It is also the difference between a refusal a rollout mode may
// soften and one it may not.
func TestAWithdrawnLeadIsNotReportedAsNeverGranted(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	recipient := connector.Recipient{Email: e.leadEmail}

	for _, state := range []string{"granted", "withdrawn"} {
		if _, err := e.store.Record(e.ctx, RecordInput{
			LeadID: e.lead, PurposeID: e.newsletter, NewState: state,
			// Both arms of the loop carry it: the grant needs it, and the
			// withdrawal ignores it, so one literal covers the pair.
			PolicyText: &grantWording,
		}); err != nil {
			t.Fatalf("recording %s: %v", state, err)
		}
	}

	tx, err := e.store.db.Pool().Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	d, err := gate.decideOne(e.ctx, tx, recipient, commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding about a withdrawn lead: %v", err)
	}
	if d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("verdict = %q, want deny after a withdrawal", d.Verdict)
	}
	if d.ReasonCode != commsauthz.ReasonConsentWithdrawn {
		t.Errorf("reason = %q, want %q — the subject is shown this, and they did withdraw",
			d.ReasonCode, commsauthz.ReasonConsentWithdrawn)
	}
}

// And the converse, or the test above would pass with every refusal renamed: a
// lead who never granted anything is still an absence.
func TestALeadWhoNeverGrantedIsReportedAsAnAbsence(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)

	tx, err := e.store.db.Pool().Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	d, err := gate.decideOne(e.ctx, tx, connector.Recipient{Email: e.leadEmail}, commsauthz.Request{LegacyPurposeKey: "newsletter"}, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if d.ReasonCode != commsauthz.ReasonNoMarketingConsent {
		t.Errorf("reason = %q, want %q for a lead who granted nothing",
			d.ReasonCode, commsauthz.ReasonNoMarketingConsent)
	}
}

// inboundFromTheLead plants a message the lead SENT us, the way the real writer
// records one: activity_participant has no lead_id column, so a lead's own mail
// is stored with person_id NULL and the bare address.
func (e *leadConsentEnv) inboundFromTheLead(ctx context.Context, t *testing.T) ids.UUID {
	t.Helper()
	anchor := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now(), 'gmail', 'human:x')`,
		anchor, "lead-thread-"+e.lead.String()); err != nil {
		t.Fatalf("planting the lead's inbound message: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, person_id, address, role)
		VALUES ($1, NULL, $2, 'from')`, anchor, e.leadEmail); err != nil {
		t.Fatalf("planting the participant: %v", err)
	}
	return anchor
}

// leadActorContext binds the actor recordBasis stamps captured_by from.
func (e *leadConsentEnv) leadActorContext() context.Context {
	return principal.WithActor(
		principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()),
		principal.Principal{Type: principal.PrincipalHuman, ID: "human:rep"})
}

// A LEAD WHO WROTE TO US CAN BE ANSWERED, with no grant on file.
//
// This is the shape that was refused in the shipped default: the composer sends
// no consent_purpose, so the key arrives empty, no purpose row matches, and the
// lead arm denied with unknown_purpose while every category enforces. A rep
// answering a lead's own mail got "not granted" — stricter than the rule for the
// same human one promotion later, and stricter than the law.
//
// The participant row carries person_id NULL and the bare address, which is how
// a lead's inbound mail is actually recorded: activity_participant has no
// lead_id column at all.
func TestALeadWhoWroteToUsCanBeAnsweredWithoutAGrant(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	ctx := e.leadActorContext()

	anchor := e.inboundFromTheLead(ctx, t)

	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	// No LegacyPurposeKey, exactly as the composer sends it.
	d, err := gate.decideOne(ctx, tx, connector.Recipient{Email: e.leadEmail},
		commsauthz.Request{AnchorActivityID: anchor}, commsauthz.PhaseStaging)
	if err != nil {
		t.Fatalf("deciding about a lead who wrote to us: %v", err)
	}
	if d.SubjectKind != entityLead {
		t.Errorf("subject kind = %q, want lead", d.SubjectKind)
	}
	if d.SubjectID != e.lead.UUID {
		t.Errorf("subject id = %v, want the lead %v", d.SubjectID, e.lead)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: a lead who wrote to us may be answered, "+
			"and refusing is an inversion — the same human allows once promoted", d.Verdict, d.ReasonCode)
	}
	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved = %q, want reply_to_inbound", d.Resolved)
	}
	if d.Basis != commsauthz.BasisSubjectInitiatedCorrespondence {
		t.Errorf("basis = %q, want subject_initiated_correspondence", d.Basis)
	}

	// The ground is RECORDED, not merely concluded: an allow that writes no
	// basis is what made a subject-access export answer "we relied on nothing".
	var bases int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM communication_basis WHERE lead_id = $1`, e.lead).Scan(&bases); err != nil {
		t.Fatal(err)
	}
	if bases != 1 {
		t.Errorf("communication_basis rows for the lead = %d, want 1: the send stands on a ground "+
			"nothing wrote down", bases)
	}
}

// EVIDENCE OUTRANKS THE PURPOSE KEY, which is the ORDER the fix is about.
//
// The previous test passes no purpose key, so it cannot tell "evidence first"
// apart from "evidence at all": both send the same lead down the same arm. Here
// a purpose key IS supplied and no grant exists for it, so the old order —
// purpose row, then grant — denied. Evidence must be consulted first and win.
func TestALeadsEvidenceOutranksAnUngrantedPurpose(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	ctx := e.leadActorContext()
	anchor := e.inboundFromTheLead(ctx, t)

	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	d, err := gate.decideOne(ctx, tx, connector.Recipient{Email: e.leadEmail},
		commsauthz.Request{AnchorActivityID: anchor, LegacyPurposeKey: "newsletter"},
		commsauthz.PhaseStaging)
	if err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: the lead's own message is the ground, and a "+
			"purpose key nobody granted must not outrank it", d.Verdict, d.ReasonCode)
	}
	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved = %q, want reply_to_inbound — the grant's class answered instead of the evidence", d.Resolved)
	}
}

// A LEAD NEVER TAKES AUTHORITY FROM VerdictForPerson.
//
// This holds a claim that otherwise lives only in a comment on decideLead.
// VerdictForPerson's ClassTransactional arm returns an unconditional allow
// without reading any grant (verdict.go, "the contract itself is the basis").
// Routing a lead through decideResolved's unsupported fallthrough would hand it
// that allow from a purpose row never checked against lead grants. There is no
// evidence here and no lead grant, so the only way this can come back allow is
// if somebody reroutes the lead arm through the person one.
func TestALeadTakesNoAuthorityFromATransactionalPurpose(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	ctx := e.leadActorContext()

	invoices := ids.New[ids.PurposeKind]()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, 'invoices', 'Invoices', 'transactional', false)`, invoices); err != nil {
		t.Fatalf("seeding the transactional purpose: %v", err)
	}

	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	d, err := gate.decideOne(ctx, tx, connector.Recipient{Email: e.leadEmail},
		commsauthz.Request{LegacyPurposeKey: "invoices"}, commsauthz.PhaseStaging)
	if err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow (%s) for a lead with no evidence and no grant: the lead arm is "+
			"taking VerdictForPerson's unconditional transactional allow, which was never "+
			"checked against a lead grant", d.ReasonCode)
	}
}

// THE SAME REPLY, ONE PHASE LATER. Staging is not where a message is sent.
//
// The thread arm is the only supported arm a lead can reach, and staging asks it
// with the anchor the caller named. Nothing carries that anchor to transmit, so
// this goes through stagedRequestFor with a real delivery row: if the thread
// cannot be re-derived there, the message is authorized at staging and parked at
// transmit, which reads to a rep as the same refusal the fix removed.
func TestALeadsReplyIsStillAllowedAtTransmit(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	ctx := e.leadActorContext()
	anchor := e.inboundFromTheLead(ctx, t)
	thread := "lead-thread-" + e.lead.String()

	// A real delivery on that thread, the way the send path stages one: the
	// thread_key is what transmit has instead of the anchor.
	delivery := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject,
		   references_chain, body, consent_purpose, thread_key)
		VALUES ($1, $2, $3, 'gmail', $5, $6::jsonb, '[]'::jsonb, 'Re: hello',
		        '[]'::jsonb, 'Thanks for writing.', 'newsletter', $4)`,
		delivery, anchor, e.user, thread,
		"<"+delivery.String()+"@margince.test>",
		`["`+e.leadEmail+`"]`); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}

	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	// Through stagedRequestFor, which is what authorizetransmit.go calls.
	// Handing decideOne the anchor directly would test a request shape the
	// transmit phase never builds — and that shape passes even when transmit
	// cannot see the thread at all.
	recipient := connector.Recipient{Email: e.leadEmail}
	threadKey, err := deliveryThreadKey(ctx, tx, delivery)
	if err != nil {
		t.Fatalf("reading the delivery's thread: %v", err)
	}
	if threadKey != thread {
		t.Fatalf("delivery thread = %q, want %q — transmit cannot name the conversation", threadKey, thread)
	}
	transmitReq := stagedRequestFor(
		commsauthz.TransmitRequest{DeliveryID: delivery, Recipients: []connector.Recipient{recipient}},
		recipient, map[string]stagedClaim{}, threadKey)

	d, err := gate.decideOne(ctx, tx, recipient, transmitReq, commsauthz.PhaseTransmit)
	if err != nil {
		t.Fatalf("deciding at transmit: %v", err)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("transmit verdict = %q (%s), want allow: staging authorized this reply and "+
			"transmit refuses it, so the message parks and the rep sees the old refusal",
			d.Verdict, d.ReasonCode)
	}
	if d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Errorf("resolved = %q, want reply_to_inbound", d.Resolved)
	}
}

// A LEAD WHO WROTE TO US ON ANOTHER THREAD CAN STILL BE WRITTEN TO.
//
// The reply arm needs the anchor's own thread. An unprompted follow-up has no
// anchor and rests on the recent-inbound arm instead — which reads the same
// authorship spelling and answers about a lead. Until this, validate() bailed
// on every non-person before that arm could run, so a lead who wrote to us last
// week was refused for want of a grant.
func TestALeadWhoWroteRecentlyCanBeFollowedUp(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	ctx := e.leadActorContext()
	e.inboundFromTheLead(ctx, t)

	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back: %v", err)
		}
	}()

	// No anchor: this is a fresh message, not a reply to a named one.
	d, err := gate.decideOne(ctx, tx, connector.Recipient{Email: e.leadEmail},
		commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}, commsauthz.PhaseStaging)
	if err != nil {
		t.Fatalf("deciding: %v", err)
	}
	if d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow: the lead wrote to us inside the window, which is "+
			"the same evidence that answers for a person", d.Verdict, d.ReasonCode)
	}
	if d.Basis != commsauthz.BasisSubjectInitiatedCorrespondence {
		t.Errorf("basis = %q, want subject_initiated_correspondence", d.Basis)
	}
}
