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
	before, err := gate.decideOne(e.ctx, tx, recipient, "newsletter")
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
	}); err != nil {
		t.Fatal(err)
	}

	after, err := gate.decideOne(e.ctx, tx, recipient, "newsletter")
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
		d, err := gate.decideOne(e.ctx, tx, recipient, "newsletter")
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
