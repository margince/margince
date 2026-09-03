// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the frequency cap counts, which is the whole rule.
//
// The ceiling is on messages the recipient RECEIVED. Two shapes look like a
// delivered advertising message and are not: a delivery that was staged and
// then parked, and a decision taken in observe mode while the old gate still
// ruled. Counting either would consume somebody's statutory allowance for mail
// that never arrived, and they would be silenced for a day by an accounting
// error nobody could see — the decision rows would all look correct.
//
// This is an integration test because the invariant IS the SQL: the join
// between a sent delivery and its transmit decision is what separates the three
// cases, and no unit test of a Go function can show that the query does it.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// marketingPurposeKey names the purpose the cap test sends under. Advertising
// is the only category a frequency cap binds, so a test that sent under any
// other purpose would prove nothing about the ceiling.
const marketingPurposeKey = "marketing_email"

type capEnv struct {
	store    *Store
	ctx      context.Context
	owner    *pgx.Conn
	ws, user ids.UUID
	activity ids.UUID
	address  string
	// decisionDelivery is the delivery the transmit decisions are recorded
	// against. It is deliberately NOT one of the delivered rows the counter
	// reads: a decision row must be able to exist for a delivery that has not
	// been sent, which is the whole distinction under test.
	decisionDelivery ids.UUID
}

// seedMarketingSubject gives the address a person with a live marketing grant,
// so the engine reaches an allow and the cap is what refuses. Without the grant
// every message would be denied for want of consent and the ceiling would never
// be the reason.
func (e *capEnv) seedMarketingSubject(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	personID := ids.New[ids.PersonKind]()
	purposeID := ids.New[ids.PurposeKind]()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO person (id, full_name, source, captured_by)
		VALUES ($1, 'Cap Subject', 'manual', 'human:x')`, personID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO person_email (person_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:x')`, personID, e.address); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, $2, 'Marketing email', 'marketing', false)`,
		purposeID, marketingPurposeKey); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO person_consent (person_id, purpose_id, state, lawful_basis, captured_at, source)
		VALUES ($1, $2, 'granted', 'consent', now(), 'test')`, personID, purposeID); err != nil {
		t.Fatal(err)
	}
	// The delivery each authorization under test attaches its decision to. It
	// stays 'pending' throughout, and the test moves it to 'parked' between
	// authorizations so the previous decision stops counting as in-flight —
	// which is what really happens when a delivery does not go out.
	e.decisionDelivery = ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		   consent_purpose, references_chain, status)
		VALUES ($1, $2, $3, 'gmail', $4, $5, '[]'::jsonb, 'Offer', 'body', $6, '[]'::jsonb, 'pending')`,
		e.decisionDelivery, e.activity, e.user, "msg-"+e.decisionDelivery.String(),
		`["`+e.address+`"]`, marketingPurposeKey); err != nil {
		t.Fatal(err)
	}
}

func setupCap(t *testing.T) *capEnv {
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
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := testdb.Reset(ctx, owner); err != nil {
		t.Fatal(err)
	}

	e := &capEnv{
		ws: ids.NewV7(), user: ids.NewV7(), activity: ids.NewV7(),
		owner: owner, address: "buyer@cap.test",
	}
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, e.ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		e.user, "rep-"+e.user.String()+"@cap.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'email', 'outbound', 'manual', now(), 'human:x')`, e.activity); err != nil {
		t.Fatal(err)
	}

	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testdb.AssertPoolsQuiesced(t) })
	e.store = NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](e.ws)))

	opCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	opCtx = principal.WithCorrelationID(opCtx, ids.NewV7())
	e.ctx = principal.WithActor(opCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"person": {Create: true, Read: true, Update: true, Delete: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return e
}

// deliveredAdvertising plants what the cap is supposed to count: a delivery
// that reached 'sent' with an allowing transmit decision for advertising.
func (e *capEnv) deliveredAdvertising(t *testing.T, sentAt time.Time) {
	t.Helper()
	e.plant(t, plantSpec{
		status: "sent", sentAt: &sentAt, phase: "transmit",
		verdict: "allow", category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})
}

type plantSpec struct {
	status   string
	sentAt   *time.Time
	phase    string
	verdict  string
	category string
	mode     string
}

// plant writes one delivery and one decision about it, exactly as the two
// writers would. The point of the test is which COMBINATIONS count, so the
// shapes are built from a spec rather than from four near-identical helpers.
func (e *capEnv) plant(t *testing.T, spec plantSpec) {
	t.Helper()
	ctx := context.Background()
	deliveryID := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		   consent_purpose, references_chain, status, sent_at)
		VALUES ($1, $2, $3, 'gmail', $4, $5, '[]'::jsonb, 'Offer', 'body',
		        'marketing', '[]'::jsonb, $6, $7)`,
		deliveryID, e.activity, e.user, "msg-"+deliveryID.String(),
		`["`+e.address+`"]`, spec.status, spec.sentAt); err != nil {
		t.Fatalf("planting the delivery: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 1, $2, $3, $4, $5, $6, 'allowed', $7, 'test')`,
		deliveryID, ids.NewV7(), e.address, spec.phase, spec.category,
		spec.verdict, spec.mode); err != nil {
		t.Fatalf("planting the decision: %v", err)
	}
}

// settleDecisionDelivery parks the delivery the previous authorization decided
// against, so its decision stops counting as a message in flight. A real
// dispatch settles the row the same way — either sent or parked — and leaving
// it pending would make each authorization under test count its own
// predecessor twice.
func (e *capEnv) settleDecisionDelivery(t *testing.T) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE comms_outbound SET status = 'parked', reason = 'test' WHERE id = $1`,
		e.decisionDelivery); err != nil {
		t.Fatalf("settling the decision delivery: %v", err)
	}
}

// received runs the counter the cap consults.
func (e *capEnv) received(t *testing.T, window time.Duration) int {
	t.Helper()
	var count int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		count, err = advertisingMessagesReceived(context.Background(), tx, e.address, time.Now().Add(-window))
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count
}

// A delivered advertising message counts. Without this the three refusals below
// would all pass against a counter that counts nothing at all.
func TestADeliveredAdvertisingMessageCountsTowardTheCap(t *testing.T) {
	e := setupCap(t)
	e.deliveredAdvertising(t, time.Now().Add(-time.Hour))
	if got := e.received(t, 24*time.Hour); got != 1 {
		t.Fatalf("counted %d delivered advertising messages, want 1", got)
	}
}

// THE INVARIANT. A parked delivery is a message nobody received, and it must
// not consume the recipient's allowance.
//
// Mutation: count communication_decision rows instead of joining to the
// delivery's own status, and this fails — which is exactly the defect it exists
// to catch, because a parked row's decision looks perfectly ordinary.
func TestAParkedDeliveryDoesNotConsumeTheCap(t *testing.T) {
	e := setupCap(t)
	sent := time.Now().Add(-time.Hour)

	// Staged, decided, and then parked: the decision exists, the mail does not.
	e.plant(t, plantSpec{
		status: "parked", phase: "transmit", verdict: "allow",
		category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})
	// A refusal recorded against a delivery that never left.
	e.plant(t, plantSpec{
		status: "parked", phase: "transmit", verdict: "deny",
		category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})
	// An observe-mode row on a parked delivery: the engine recorded what it
	// would have said while the old gate ruled, and nothing went out.
	e.plant(t, plantSpec{
		status: "parked", phase: "transmit", verdict: "allow",
		category: string(commsauthz.CategoryMarketing), mode: "observe",
	})

	if got := e.received(t, 24*time.Hour); got != 0 {
		t.Fatalf("counted %d messages the recipient never received, want 0", got)
	}

	// And one that really went, to prove the counter was awake for all of it.
	e.deliveredAdvertising(t, sent)
	if got := e.received(t, 24*time.Hour); got != 1 {
		t.Fatalf("counted %d, want 1 — only the delivered message counts", got)
	}
}

// A message BETWEEN its authorization and its delivery counts, and it has to.
//
// The authorization commits before the provider is called, and 'sent' is
// written afterwards by a different transaction. A counter that saw only
// delivered mail would read the same number for every worker inside that
// window, and each would send: the ceiling would be exceeded by however many
// workers happened to be running. An allowing transmit decision on a still
// pending delivery is that in-flight message.
//
// Mutation: drop the pending arm from the count and this fails, which is the
// bug that made the whole cap unenforceable under any concurrency.
func TestAMessageInFlightAlreadyConsumesTheCap(t *testing.T) {
	e := setupCap(t)
	e.plant(t, plantSpec{
		status: "pending", phase: "transmit", verdict: "allow",
		category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})
	if got := e.received(t, 24*time.Hour); got != 1 {
		t.Fatalf("counted %d, want 1 — an authorized message on its way out is one the recipient is getting", got)
	}
}

// A queued delivery nothing has authorized yet does NOT count. Only the
// transmit decision makes a pending row an in-flight message; the row alone is
// a message that may still be refused.
func TestAQueuedDeliveryWithNoTransmitDecisionDoesNotCount(t *testing.T) {
	e := setupCap(t)
	e.plant(t, plantSpec{
		status: "pending", phase: "staging", verdict: "allow",
		category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})
	if got := e.received(t, 24*time.Hour); got != 0 {
		t.Fatalf("counted %d, want 0 — nothing has authorized this delivery to go out", got)
	}
}

// A staging decision is not a transmit decision. Both phases write a row about
// the same delivery, so counting without the phase filter would double every
// sent message and halve the effective ceiling.
func TestAStagingDecisionDoesNotCountBesideItsTransmitRow(t *testing.T) {
	e := setupCap(t)
	sent := time.Now().Add(-time.Hour)
	e.deliveredAdvertising(t, sent)
	// The staging row the same delivery would carry.
	e.plant(t, plantSpec{
		status: "sent", sentAt: &sent, phase: "staging", verdict: "allow",
		category: string(commsauthz.CategoryMarketing), mode: "enforce",
	})

	if got := e.received(t, 24*time.Hour); got != 1 {
		t.Fatalf("counted %d, want 1 — a staging row is not a second message", got)
	}
}

// An operational message is not advertising. A ceiling that counted every mail
// would silence a customer's invoices for a day because somebody sent them
// three offers.
func TestAnOperationalMessageDoesNotConsumeTheAdvertisingCap(t *testing.T) {
	e := setupCap(t)
	sent := time.Now().Add(-time.Hour)
	e.plant(t, plantSpec{
		status: "sent", sentAt: &sent, phase: "transmit", verdict: "allow",
		category: string(commsauthz.CategoryInvoiceOrPayment), mode: "enforce",
	})

	if got := e.received(t, 24*time.Hour); got != 0 {
		t.Fatalf("counted %d, want 0 — an invoice is not advertising", got)
	}
}

// The window rolls. A message older than the window has stopped counting, or the
// cap would be a permanent ban rather than a daily ceiling.
func TestAMessageOlderThanTheWindowHasStoppedCounting(t *testing.T) {
	e := setupCap(t)
	e.deliveredAdvertising(t, time.Now().Add(-25*time.Hour))

	if got := e.received(t, 24*time.Hour); got != 0 {
		t.Fatalf("counted %d, want 0 — the message fell outside the 24h window", got)
	}
	if got := e.received(t, 48*time.Hour); got != 1 {
		t.Fatalf("counted %d over 48h, want 1 — the message is still there", got)
	}
}

// One mailbox, two spellings. Addresses are compared case-insensitively on both
// sides, or a capped recipient could be reached again by capitalising their
// address.
func TestTheCapFollowsTheMailboxNotItsSpelling(t *testing.T) {
	e := setupCap(t)
	sent := time.Now().Add(-time.Hour)
	deliveryID := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO comms_outbound
		  (id, activity_id, user_id, provider, message_id, recipients, cc, subject, body,
		   consent_purpose, references_chain, status, sent_at)
		VALUES ($1, $2, $3, 'gmail', $4, '[]'::jsonb, '[]'::jsonb, 'Offer', 'body',
		        'marketing', '[]'::jsonb, 'sent', $5)`,
		deliveryID, e.activity, e.user, "msg-"+deliveryID.String(), sent); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO communication_decision
		  (delivery_id, attempt, decision_set_id, recipient_address, phase,
		   resolved_category, verdict, reason_code, mode, actor)
		VALUES ($1, 1, $2, $3, 'transmit', 'marketing', 'allow', 'allowed', 'enforce', 'test')`,
		deliveryID, ids.NewV7(), "Buyer@Cap.Test"); err != nil {
		t.Fatal(err)
	}
	if got := e.received(t, 24*time.Hour); got != 1 {
		t.Fatalf("counted %d, want 1 — Buyer@Cap.Test is the same mailbox as buyer@cap.test", got)
	}
}

// The cap REFUSES, and only after the ceiling is reached — driven through
// AuthorizeTransmit, which is the entry point the dispatcher actually calls.
//
// Going through the real door rather than calling applyFrequencyCap directly is
// the point. A test of the helper alone stays green when the helper stops being
// called, which is how a working guard becomes an unwired one that every gate
// still passes.
func TestTheCapRefusesTheMessageAfterTheCeiling(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	// A code no pack in the tree claims, so registering it here cannot collide
	// with a real jurisdiction's rules and cannot be affected by one.
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: "zc", Version: 1,
		FrequencyCap: &messagingrules.FrequencyCap{Messages: 3, Window: 24 * time.Hour},
	})
	gate := NewGate(e.store).WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return "zc", nil
		}))

	// Both halves of the answer: what the row records, and whether the
	// dispatcher is actually allowed to send. Asserting only the row would pass
	// with the ceiling recorded and the message going out anyway, which is
	// exactly the shape a rollout mode could otherwise produce.
	decide := func(t *testing.T, attempt int) (commsauthz.Decision, bool) {
		t.Helper()
		ticket, err := gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: e.decisionDelivery, Attempt: attempt,
			Recipients: []connector.Recipient{{Email: e.address}},
			PurposeKey: marketingPurposeKey, Subject: "Offer", Body: "body",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		var out commsauthz.Decision
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `
				SELECT verdict, reason_code FROM communication_decision
				 WHERE decision_set_id = $1 AND phase = 'transmit'`,
				ticket.DecisionSetID).Scan(&out.Verdict, &out.ReasonCode)
		}); err != nil {
			t.Fatalf("reading back the decision: %v", err)
		}
		return out, ticket.Allowed
	}

	for i := range 3 {
		got, allowed := decide(t, i+1)
		if got.Verdict != commsauthz.VerdictAllow {
			t.Fatalf("message %d refused with %q, want an allow below the ceiling", i+1, got.ReasonCode)
		}
		if !allowed {
			t.Fatalf("message %d was not ticketed, want it to go out below the ceiling", i+1)
		}
		// That message goes out, and the delivery it was decided against
		// settles — so the next authorization sees three DELIVERED messages
		// rather than its own predecessor still counted as in flight.
		e.deliveredAdvertising(t, time.Now().Add(-time.Minute))
		e.settleDecisionDelivery(t)
	}
	fourth, allowed := decide(t, 4)
	if fourth.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("the fourth message in 24h was recorded as allowed, want a refusal at the ceiling")
	}
	if fourth.ReasonCode != commsauthz.ReasonFrequencyCapReached {
		t.Errorf("refused with %q, want %q", fourth.ReasonCode, commsauthz.ReasonFrequencyCapReached)
	}
	// THE half that matters. Every category defaults to observe, so a ceiling
	// that only wrote a deny row would let the fourth message go out on every
	// installation that has not set a mode — the pack would claim an enforced
	// limit and enforce nothing. The cap denies regardless of mode because it
	// is in the absolute set.
	if allowed {
		t.Fatal("the fourth message was ticketed for sending despite the ceiling — the cap recorded a refusal and permitted the send")
	}
}

// A jurisdiction that declares no ceiling caps nothing, and an installation
// with no country stated resolves no jurisdiction at all. Both must leave mail
// exactly as it was, or this change would silently throttle every installation
// that never asked for a cap.
func TestNoCountryAndNoCeilingBothLeaveTheMessageAlone(t *testing.T) {
	e := setupCap(t)
	for range 5 {
		e.deliveredAdvertising(t, time.Now().Add(-time.Minute))
	}
	// Registered with no FrequencyCap: a real jurisdiction that imposes no
	// ceiling, which must behave like no jurisdiction at all for this rule.
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zu", Version: 1})
	allowed := commsauthz.Decision{
		Verdict:   commsauthz.VerdictAllow,
		Resolved:  commsauthz.CategoryMarketing,
		Recipient: connector.Recipient{Email: e.address},
	}
	for _, tc := range []struct {
		name string
		gate *Gate
	}{
		{"no country stated", NewGate(e.store)},
		{"a jurisdiction that declares no ceiling", NewGate(e.store).WithInstallationCountry(
			InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
				return "zu", nil
			}))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out commsauthz.Decision
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				var err error
				out, err = tc.gate.applyFrequencyCap(context.Background(), tx, allowed, time.Now())
				return err
			}); err != nil {
				t.Fatalf("applying the cap: %v", err)
			}
			if out.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("refused with %q, want the message left alone", out.ReasonCode)
			}
		})
	}
}
