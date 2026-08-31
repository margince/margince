// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The automatic-enrichment consumer end to end: a person.created envelope
// reaches it and a run exists for that person — admitted, fenced, frozen and
// reserved, with the submit job committed beside it.
//
// ONE installation-wide posture decides whether an automatic lookup happens at
// all: integrations.automatic_lookup, in place of the per-connection switches
// that used to ask which WRITER a purchase followed. An automatic run buys only
// what the provider gives away, so that distinction stopped paying for itself.
//
// WHO created the person still decides whether there is an automatic trigger to
// admit, and that is the subject of most of these cases: the same event is
// emitted by four writers, and an agent creating a contact must not reach
// through it what the REST policy denies the agent at the door.

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

type providerConsumerEnv struct {
	env      *Env
	consumer *compose.PersonDataEnrich
	personID ids.UUID
	enqueued *int
}

// setupProviderConsumer seeds a subject and a connected provider in the named
// mode, and wires the consumer over the REAL cross-module binding.
// setupProviderConsumer builds the consumer with the installation's automatic
// lookup switched on or off.
//
// The installation SETTING, not the connection's mode: one answer governs every
// automatic trigger now, and the per-connection mode and switches are still
// written and still answered on the wire while nothing reads what they say. A
// fixture that expressed "off" as `mode = on_demand` was expressing nothing.
func setupProviderConsumer(t *testing.T, automaticLookup bool) *providerConsumerEnv {
	t.Helper()
	// Before the database: this is a contradiction between the fixture and the
	// provider's own cost table, and neither a schema nor a seeded row can
	// change the answer.
	seeded := []string{"professional_email", "linkedin_profile"}
	requireAnAutomaticRunCanBuy(t, seeded)

	e := Setup(t)
	c := &providerConsumerEnv{env: e, enqueued: new(int), personID: seedSubject(t, e)}

	// One PRICED category and one FREE one, and both halves are load-bearing.
	// An automatic run may take only what the provider gives away, so a
	// connection selecting nothing free never queues anything — and the two
	// cases here that assert a run EXISTS are the only ones that reach that
	// code, every other case in this file being refused earlier by the actor
	// or the toggle. Those two are this file's positive control: without them
	// the toggle could be wired to a constant false and every assertion here
	// would still be green. The priced category stays because narrowing to the
	// free set is the behaviour under test, and a connection carrying only free
	// categories cannot tell narrowing from taking everything.
	//
	// Named once above and passed to both the INSERT and the guard, so the
	// check reads the selection this connection actually carries rather than a
	// second copy of it that can drift from it silently.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO provider_connection
			       (id, provider, status, mode, preset, categories, automatic_individual_create)
			VALUES (gen_random_uuid(), 'surfe', 'connected', 'automatic_on_create', 'full', $1, true)
			ON CONFLICT (provider) DO UPDATE
			   SET status = 'connected', mode = 'automatic_on_create', categories = $1,
			       automatic_individual_create = true`, seeded)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// The one answer that decides. Written as a row the way the settings
	// surface leaves it, because the read is machinery-applied and this
	// fixture has no caller for a gated write.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO setting (key, value) VALUES ($1, to_jsonb($2::bool))
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
			integrations.AutomaticLookup.Key(), automaticLookup)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	reg, err := integrations.NewRegistry(integrations.NewOfflineProvider(0, time.Now))
	if err != nil {
		t.Fatal(err)
	}
	store, err := integrations.NewStore(e.DB(), keyvault.NewMemory(), reg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	// The real binding, plus a counting enqueue: a queued run with no job is
	// the failure QueueRun refuses to commit, and only a counter proves it.
	bound := compose.BindProviderDomain(store).WithSubmitEnqueue(
		func(context.Context, pgx.Tx, string, string) error {
			*c.enqueued++
			return nil
		})
	c.consumer = compose.NewPersonDataEnrich(e.Pool, bound, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	return c
}

// requireAnAutomaticRunCanBuy fails the fixture, not the case, when the seeded
// connection has nothing an automatic run may take.
//
// What it protects is this file's positive control. The refusal is silent — a
// skipped run, no error — so a fixture that drifts out of the provider's free
// set presents as "the toggle bought nothing", which is indistinguishable from
// the toggle being wired to a constant false. Free() is DERIVED from the cost
// table, so a provider that starts charging for this category fails HERE,
// naming the fixture, rather than in two cases that read as though a policy
// broke.
func requireAnAutomaticRunCanBuy(t *testing.T, selected []string) {
	t.Helper()
	free := map[provider.Category]bool{}
	for _, c := range integrations.NewOfflineProvider(0, time.Now).Descriptor().Free() {
		free[c] = true
	}
	for _, c := range selected {
		if free[provider.Category(c)] {
			return
		}
	}
	t.Fatalf("the fixture connection selects %v, and this provider charges for all of "+
		"them — an automatic run takes only free categories, so it would queue nothing "+
		"and the two cases asserting a run exists would fail as though a toggle broke",
		selected)
}

// humanCreated is what the relay delivers for a person a HUMAN typed — the
// individual create the automatic_individual_create toggle governs.
func humanCreated(personID ids.UUID) events.Envelope {
	return actorEnvelope(personID, "human", "human:"+ids.NewV7().String())
}

func actorEnvelope(personID ids.UUID, actorType, actorID string) events.Envelope {
	return events.Envelope{
		EventID:    ids.NewV7(),
		Type:       "person.created",
		OccurredAt: time.Now().UTC(),
		Actor:      events.Actor{Type: actorType, ID: actorID},
		Entity:     events.EntityRef{Type: "person", ID: personID},
		Trace:      events.Trace{CorrelationID: ids.NewV7()},
	}
}

func (c *providerConsumerEnv) runsForPerson(t *testing.T) int {
	t.Helper()
	var runs int
	if err := database.WithWorkspaceTx(c.env.Admin(), c.env.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM provider_run WHERE person_id = $1`, c.personID).Scan(&runs)
	}); err != nil {
		t.Fatal(err)
	}
	return runs
}

// A human creating a contact, with the toggle on: the event buys the data.
func TestPersonCreatedQueuesAnEnrichmentRun(t *testing.T) {
	c := setupProviderConsumer(t, true)

	if err := c.consumer.HandleEvent(context.Background(), humanCreated(c.personID)); err != nil {
		t.Fatal(err)
	}

	// Read directly, and BEFORE anything else queues: the consumer swallows
	// configuration refusals by design, so a probe that queued its own run
	// first would create the row this asserts and pass even if HandleEvent
	// did nothing at all.
	var state, skipReason string
	if err := database.WithWorkspaceTx(c.env.Admin(), c.env.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT state, coalesce(skip_reason, '') FROM provider_run WHERE person_id = $1`,
			c.personID).Scan(&state, &skipReason)
	}); err != nil {
		t.Fatalf("the consumer queued nothing for a newly created person: %v", err)
	}
	if skipReason != "" {
		t.Fatalf("the run was skipped (%s) rather than queued — the fixture trips a fence it was not meant to", skipReason)
	}
	if state != string(provider.RunQueued) {
		t.Errorf("the run is %s, want queued — the consumer must not call the provider on the event lane", state)
	}
	if *c.enqueued != 1 {
		t.Errorf("%d submit jobs were committed, want 1 — a queued run with no job sits in the live-run index forever, blocking every later attempt at that subject", *c.enqueued)
	}
}

// Auto-enrich switched off: the refusal is the configuration working, so the
// consumer reports success and buys nothing.
func TestPersonCreatedWithAutoEnrichOffBuysNothingAndDoesNotError(t *testing.T) {
	c := setupProviderConsumer(t, false)

	if err := c.consumer.HandleEvent(context.Background(), humanCreated(c.personID)); err != nil {
		t.Fatalf("a switched-off toggle surfaced as a failure, which would wedge the consumer group: %v", err)
	}
	if runs := c.runsForPerson(t); runs != 0 {
		t.Errorf("%d runs exist with auto-enrich off — the customer's credits were spent against their own setting", runs)
	}
}

// Buying provider data is human-only on the REST surface (x-agent-access,
// ADR-0055). An agent that can create a person must not reach through this
// event what the policy denies it at the door: the seat cap is what the
// agent's human could do through the same gate, and an event has no gate.
func TestAnAgentCreatedPersonBuysNothing(t *testing.T) {
	c := setupProviderConsumer(t, true)

	if err := c.consumer.HandleEvent(context.Background(),
		actorEnvelope(c.personID, "agent", "agent:overnight")); err != nil {
		t.Fatal(err)
	}
	if runs := c.runsForPerson(t); runs != 0 {
		t.Errorf("an agent-created person triggered %d paid runs — an agent holding only createPerson just spent the customer's credits through a route the policy makes human-only", runs)
	}
}

// The other half of the posture: switched ON, a captured counterparty is looked
// up exactly like a typed one — the writer is not the question any more.
//
// Without this case the refusal beside it passes against a gate that admits
// nobody: the setting could be wired to a constant false and every assertion in
// this file would still be green.
func TestACapturedContactIsLookedUpOnceThePostureIsOn(t *testing.T) {
	c := setupProviderConsumer(t, true)

	if err := c.consumer.HandleEvent(context.Background(),
		actorEnvelope(c.personID, "connector", "connector:gmail")); err != nil {
		t.Fatal(err)
	}
	if runs := c.runsForPerson(t); runs != 1 {
		t.Errorf("%d runs exist for a captured counterparty with automatic lookups on, want 1 — the installation left the posture on and got nothing", runs)
	}
	if *c.enqueued != 1 {
		t.Errorf("%d submit jobs were committed, want 1 — a queued run with no job blocks every later attempt at that subject", *c.enqueued)
	}
}

// A captured counterparty is governed by the same installation-wide answer as
// any other automatic trigger.
//
// Capture creates a person per external counterparty, so a mailbox connect with
// a year of history reaches thousands of them.
//
// It used to be governed by a switch of its own — the writer mattered, because
// a connector's thousands of contacts each spent credits. An automatic run now
// buys only what the provider gives away, so the distinction stopped paying for
// itself and the remaining question is the installation's posture. Turning that
// off is what stops a connected mailbox looking up every sender in its history.
func TestAConnectorCreatedPersonIsGovernedByTheInstallationsPosture(t *testing.T) {
	c := setupProviderConsumer(t, false)

	if err := c.consumer.HandleEvent(context.Background(),
		actorEnvelope(c.personID, "connector", "connector:gmail")); err != nil {
		t.Fatal(err)
	}
	if runs := c.runsForPerson(t); runs != 0 {
		t.Errorf("a captured counterparty triggered %d runs with automatic lookups switched off — connecting a mailbox would look up every sender in its history", runs)
	}
}

// Only person.created buys. An edit must not re-purchase.
func TestOnlyPersonCreatedTriggersAPurchase(t *testing.T) {
	c := setupProviderConsumer(t, true)
	env := humanCreated(c.personID)
	env.Type = "person.updated"

	if err := c.consumer.HandleEvent(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if runs := c.runsForPerson(t); runs != 0 {
		t.Errorf("an edit bought data (%d runs): every typo fixed on a contact would charge the customer again", runs)
	}
}
