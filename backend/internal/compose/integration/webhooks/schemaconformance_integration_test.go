// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webhooks

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/authz/authztest"
)

// Does what we PUBLISH match what we DOCUMENT?
//
// The A7 conformance gate, in two halves: the payload a real domain write
// stages, and the envelope the delivery engine actually POSTs. Both re-derive
// their schema from api/public-events.yaml — the same file gen-payloads
// compiles — rather than from the generated Go types, so a body that satisfies
// the Go struct and has drifted from the documented wire contract is still
// caught.
//
// Beside the delivery tests rather than inside them: those ask whether a
// webhook arrives, retries and stops; these ask whether what arrived is the
// shape the contract promised. Two questions, and the file holding both had
// grown past what one reader should hold at once.

// TestDealStageChangedPayloadConformsToPublicSchema is the payload
// conformance gate (A7, payload-`data` only — the envelope-level assertion
// follows in TestPublicEventEnvelopeConformsToPublicSchema below, which
// exercises toWireEnvelope): a REAL event, one the deals module emits
// by actually advancing a deal through HTTP, must validate against the
// published PublicEventDealStageChanged component schema in
// api/public-events.yaml. This is deliberately independent of any Go struct —
// it re-derives the schema from the SAME source file gen-payloads compiles,
// so a payload that satisfies the generated Go type but drifted from the
// documented wire contract (or vice versa) is still caught.
func TestDealStageChangedPayloadConformsToPublicSchema(t *testing.T) {
	we := setupWebhooks(t)
	stages := apptest.DiscoverSeededPipeline(t, we.AppEnv)
	dealID := apptest.ExerciseDealToWon(t, we.AppEnv, stages)

	data := realEventPayload(t, we, "deal.stage_changed", dealID)
	schema := publicEventSchema(t, "PublicEventDealStageChanged")
	if err := schema.VisitJSON(data); err != nil {
		t.Fatalf("the real deal.stage_changed payload does not conform to its published schema: %v", err)
	}
}

// TestPublicEventEnvelopeConformsToPublicSchema is the ENVELOPE-level half
// of the A7 conformance gate (Task 6/Phase 5): the actual HTTP body the
// delivery engine POSTs for a real deal.stage_changed event — the exact
// bytes toWireEnvelope + json.Marshal produce, delivered by HandleEvent
// itself, not a hand-built fixture — must validate against the published
// PublicEventEnvelope component schema in api/public-events.yaml. The
// event fed to HandleEvent is read back from the outbox (realEventEnvelope),
// so this is the SAME internal envelope a bus consumer would receive in
// production, proving the mapping end to end rather than only at the unit
// level (wireenvelope_test.go covers the pure mapping in isolation).
func TestPublicEventEnvelopeConformsToPublicSchema(t *testing.T) {
	we := setupWebhooks(t)
	stages := apptest.DiscoverSeededPipeline(t, we.AppEnv)
	dealID := apptest.ExerciseDealToWon(t, we.AppEnv, stages)
	env := realEventEnvelope(t, we, "deal.stage_changed", dealID)

	rcv := newReceiver(t, http.StatusOK)
	now := time.Now().UTC()
	deliverer := newTestDeliverer(we, &now, rcv.server.Client())
	we.createSubscription(t, rcv.server.URL+"/hook", []string{"deal.stage_changed"})

	if err := deliverer.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling the real deal.stage_changed event: %v", err)
	}
	hits := rcv.snapshot()
	if len(hits) != 1 {
		t.Fatalf("got %d deliveries for the real event, want exactly 1", len(hits))
	}

	var delivered any
	if err := json.Unmarshal(hits[0].body, &delivered); err != nil {
		t.Fatalf("the delivered body is not valid JSON: %v", err)
	}
	schema := publicEventSchema(t, "PublicEventEnvelope")
	if err := schema.VisitJSON(delivered); err != nil {
		t.Fatalf("the real delivered envelope does not conform to PublicEventEnvelope: %v", err)
	}
}

// realEventEnvelope reads back the most recent outbox envelope of eventType
// naming entityID as its subject, decoded into the internal kevents.Envelope
// shape — the same row a bus consumer (HandleEvent, in production) would
// receive. It queries through the owner connection (the same RLS-bypassing
// role every other direct event_outbox assertion in this package uses).
func realEventEnvelope(t *testing.T, we *webhookEnv, eventType, entityID string) kevents.Envelope {
	t.Helper()
	var raw []byte
	err := we.Owner.QueryRow(context.Background(),
		`SELECT envelope FROM event_outbox
		 WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2
		 ORDER BY seq DESC LIMIT 1`,
		eventType, entityID).Scan(&raw)
	if err != nil {
		t.Fatalf("reading the real %s envelope for entity %s: %v", eventType, entityID, err)
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshaling the %s envelope: %v", eventType, err)
	}
	return env
}

// realEventPayload returns the AS-STAGED payload of the real envelope
// realEventEnvelope reads, decoded as generic JSON (any) —
// schema.VisitJSON's expected input shape. The point here is the event as
// the domain write staged it, not anything a delivery body wraps it in.
//
//craft:ignore naked-any generic JSON is exactly the input schema.VisitJSON expects — the payload shape varies per event type, so there is no concrete type to name here
func realEventPayload(t *testing.T, we *webhookEnv, eventType, entityID string) any {
	t.Helper()
	env := realEventEnvelope(t, we, eventType, entityID)
	if len(env.Payload) == 0 {
		t.Fatalf("%s envelope for entity %s carries no payload", eventType, entityID)
	}
	var data any
	if err := json.Unmarshal(env.Payload, &data); err != nil {
		t.Fatalf("unmarshaling the %s payload as generic JSON: %v", eventType, err)
	}
	return data
}

// publicEventSchema loads api/public-events.yaml — the SAME file
// gen-payloads compiles into crmcontracts — and returns the named
// component schema. kin-openapi (already a repo dependency, driving
// gen-payloads) loads this 3.1 document directly: none of today's schemas
// use a 3.1-only construct kin-openapi's 3.0-oriented loader can't parse, so
// no downgrade step is needed here (unlike gen-payloads, which also feeds
// oapi-codegen's stricter 3.0 subset).
func publicEventSchema(t *testing.T, name string) *openapi3.Schema {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../../../api/public-events.yaml")
	if err != nil {
		t.Fatalf("loading api/public-events.yaml: %v", err)
	}
	ref, ok := doc.Components.Schemas[name]
	if !ok || ref.Value == nil {
		t.Fatalf("api/public-events.yaml has no component schema %q", name)
	}
	return ref.Value
}

// AdmittedAuthority delegates to this fixture's own two reads; see
// authztest.AdmittedFromPair for why the body is not written out here.
func (f failingResolver) AdmittedAuthority(ctx context.Context, ws, human, _ ids.UUID) (authz.RBAC, principal.SeatType, error) {
	return authztest.AdmittedFromPair(ctx, ws, human, f.EffectiveRBAC, f.SeatType)
}
