// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Lead→contact promotion must carry lead-scoped consent through
// (data-model §7: subject re-pointed, proof preserved), applying the
// merge.go precedent where the target contact already holds a state:
// withdrawal wins with an appended proof event, an existing contact state
// stands, and untouched purposes re-point wholesale. Proven over a real
// migrated Postgres — the constraint interplay (both unique keys, the
// subject CHECK) is the thing under test.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promoteConsentEnv is this suite's fixture over the already-migrated
// database: one fresh workspace, one user, two consent purposes, and the
// store under an unbounded principal.
type promoteConsentEnv struct {
	owner               *pgx.Conn
	store               *Store
	ctx                 context.Context
	ws, user            ids.UUID
	newsletter, updates ids.UUID
}

func setupPromoteConsent(t *testing.T) *promoteConsentEnv {
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

	e := &promoteConsentEnv{
		owner: owner,
		ws:    ids.NewV7(), user: ids.NewV7(),
		newsletter: ids.NewV7(), updates: ids.NewV7(),
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`, e.user, "rep-"+e.user.String()+"@pc.test"); err != nil {
		t.Fatal(err)
	}
	for id, key := range map[ids.UUID]string{e.newsletter: "newsletter", e.updates: "product_updates"} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO consent_purpose (id, key, label) VALUES ($1, $2, $2)`,
			id, key); err != nil {
			t.Fatal(err)
		}
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
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws))).
		WithSettings(settings.New(pool, settings.NewRegistry(Definitions()...))).
		// The grant read compose binds to identity. Spelled here over the same
		// role tables rather than stubbed to true: a stub would make every
		// nomination pass and the refusal it exists to prove would be the one
		// case the suite could not reach.
		WithSeatReadsLeads(func(ctx context.Context, tx pgx.Tx, seat ids.UUID) (bool, error) {
			var reads bool
			err := tx.QueryRow(ctx, `
				SELECT EXISTS (
				  SELECT 1 FROM role_assignment ra JOIN role r ON r.id = ra.role_id
				   WHERE ra.user_id = $1
				     AND (r.permissions->'objects'->'lead'->>'read')::boolean)`, seat).Scan(&reads)
			return reads, err
		})

	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	e.ctx = principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"lead":    {Create: true, Read: true, Update: true, Delete: true},
				"contact": {Create: true, Read: true, Update: true, Delete: true},
				// The lead vocabularies share the custom-field catalog's grant.
				"custom_field": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return e
}

func (e *promoteConsentEnv) seedLead(t *testing.T, email string) ids.LeadID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO lead (id, full_name, email, status, source, captured_by)
		 VALUES ($1, 'Lena Lead', lower($2), 'contacted', 'inbound', 'human:x')`,
		id, email); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.LeadKind](id)
}

func (e *promoteConsentEnv) seedLeadConsent(t *testing.T, lead ids.LeadID, purpose ids.UUID, state string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_consent (lead_id, purpose_id, state, captured_at, source)
		 VALUES ($1, $2, $3, $4, 'form')`,
		lead, purpose, state, now); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_event (lead_id, purpose_id, new_state, source, policy_text, policy_version, captured_at, captured_by)
		 VALUES ($1, $2, $3, 'form', 'seeded wording', 'v1', $4, 'human:x')`,
		lead, purpose, state, now); err != nil {
		t.Fatal(err)
	}
}

// consentRow reads the subject's state row for one purpose straight off
// the table (the contacts package may not import the consent module).
func (e *promoteConsentEnv) consentRow(t *testing.T, column string, subject ids.UUID, purpose ids.UUID) (state string, found bool) {
	t.Helper()
	err := e.owner.QueryRow(context.Background(),
		`SELECT state FROM contact_consent WHERE `+column+` = $1 AND purpose_id = $2`,
		subject, purpose).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return state, true
}

func TestPromotionRepointsLeadConsentToTheNewContact(t *testing.T) {
	e := setupPromoteConsent(t)
	lead := e.seedLead(t, "lena@fresh.example")
	e.seedLeadConsent(t, lead, e.newsletter, "granted")

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if merged {
		t.Fatal("no contact existed; promotion must create, not merge")
	}
	contactID := ids.UUID(contact.Id)

	// The state re-pointed: contact arm set, lead arm cleared — the carried
	// consent no longer rides the retired lead's lifecycle.
	state, found := e.consentRow(t, "contact_id", contactID, e.newsletter)
	if !found || state != "granted" {
		t.Fatalf("carried consent = (%q, %v), want granted on the contact", state, found)
	}
	if _, still := e.consentRow(t, "lead_id", lead.UUID, e.newsletter); still {
		t.Fatal("the lead-scoped state row must re-point, not duplicate")
	}
	var leadArm *ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT lead_id FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2`,
		contactID, e.newsletter).Scan(&leadArm); err != nil {
		t.Fatal(err)
	}
	if leadArm != nil {
		t.Fatal("lead_id must clear on re-point (a lead deletion would cascade the contact's consent away)")
	}

	// Proof preserved: the historical lead-scoped event stays AS WRITTEN.
	var leadEvents int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_event WHERE lead_id = $1 AND contact_id IS NULL AND new_state = 'granted'`,
		lead).Scan(&leadEvents); err != nil {
		t.Fatal(err)
	}
	if leadEvents != 1 {
		t.Fatalf("historical lead-scoped proof events = %d, want 1 untouched", leadEvents)
	}
}

func TestPromotionMergeAppliesWithdrawalWinsAndStateStands(t *testing.T) {
	e := setupPromoteConsent(t)
	email := "lena@collision.example"
	lead := e.seedLead(t, email)
	// The lead withdrew newsletter and granted product updates.
	e.seedLeadConsent(t, lead, e.newsletter, "withdrawn")
	e.seedLeadConsent(t, lead, e.updates, "granted")

	// A live contact already holds the email — promotion will merge — and
	// already has newsletter granted.
	contactID := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by)
		 VALUES ($1, 'Lena Contact', 'manual', 'human:x')`, contactID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
		 VALUES ($1, lower($2), 'work', true, 1, 'manual', 'human:x')`, contactID, email); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO contact_consent (contact_id, purpose_id, state, captured_at, source)
		 VALUES ($1, $2, 'granted', now(), 'manual')`, contactID, e.newsletter); err != nil {
		t.Fatal(err)
	}

	contact, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "inbound_reply"})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !merged || ids.UUID(contact.Id) != contactID {
		t.Fatalf("promotion must merge into the existing contact: merged=%v id=%s", merged, contact.Id)
	}

	// Withdrawal wins: the contact's newsletter grant flips, with proof.
	state, found := e.consentRow(t, "contact_id", contactID, e.newsletter)
	if !found || state != "withdrawn" {
		t.Fatalf("newsletter after merge = (%q, %v), want the lead's withdrawal to win", state, found)
	}
	var proof int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_event
		 WHERE contact_id = $1 AND purpose_id = $2 AND new_state = 'withdrawn' AND source = 'promotion'`,
		contactID, e.newsletter).Scan(&proof); err != nil {
		t.Fatal(err)
	}
	if proof != 1 {
		t.Fatalf("withdrawal-wins proof events = %d, want exactly 1 (a state change without proof breaks Art. 7(1))", proof)
	}

	// The purpose the contact had no row for travels over.
	state, found = e.consentRow(t, "contact_id", contactID, e.updates)
	if !found || state != "granted" {
		t.Fatalf("product_updates after merge = (%q, %v), want the lead's grant carried", state, found)
	}
	// No lead-scoped state rows remain on the retired lead.
	var leftovers int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM contact_consent WHERE lead_id = $1`, lead).Scan(&leftovers); err != nil {
		t.Fatal(err)
	}
	if leftovers != 0 {
		t.Fatalf("lead-scoped state rows left behind = %d, want 0", leftovers)
	}
}
