// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keyedRoute stands in for a route whose idempotency middleware claims the key.
// The first call under a key runs; a repeat replays and notes no effect.
type keyedRoute struct{ keys []string }

func (k *keyedRoute) run(ctx context.Context, _ RouteCall) (json.RawMessage, error) {
	key := RouteRetryKey(ctx)
	for _, seen := range k.keys {
		if seen == key {
			agentvolume.NoteEffects(ctx, 0)
		}
	}
	k.keys = append(k.keys, key)
	return json.RawMessage(`{"id":"g1"}`), nil
}

func newKeyedRouteFixture(t *testing.T) (*Registry, *keyedRoute, *countingCharger, context.Context) {
	t.Helper()
	route, charger := &keyedRoute{}, newCountingCharger()
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}), WithIdempotency(&recordingClaims{}),
		WithVolumeCharger(charger), WithRouteRetryKeys(map[string]bool{shareRecordVerb.Verb: true}))
	RegisterRouteVerbs(r, []RouteVerb{shareRecordVerb}, route.run)
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(), PassportID: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
	return r, route, charger, ctx
}

// The route claims the key, so the surface hands it over, and a replay the route
// answers charges no second write.
func TestARouteKeyedRetryReachesTheRouteAndChargesOnce(t *testing.T) {
	r, route, charger, ctx := newKeyedRouteFixture(t)
	call := json.RawMessage(`{"request_body":{},"idempotency_key":"k1"}`)
	for range 2 {
		if _, err := r.Invoke(ctx, shareRecordVerb.Verb, call); err != nil {
			t.Fatalf("a keyed route call was refused: %v", err)
		}
	}
	if len(route.keys) != 2 || route.keys[0] != "k1" || route.keys[1] != "k1" {
		t.Fatalf("the route was handed keys %v, want k1 twice", route.keys)
	}
	if got := charger.charged[agentvolume.Writes]; got != 1 {
		t.Fatalf("a run and its replay charged %d writes, want 1", got)
	}
}

// The route claims only once the call runs, after an approval is redeemed, so a
// keyed call presenting one is refused before anything runs.
func TestARouteKeyedCallRefusesAnApproval(t *testing.T) {
	r, route, _, ctx := newKeyedRouteFixture(t)
	_, err := r.Invoke(ctx, shareRecordVerb.Verb,
		json.RawMessage(`{"request_body":{},"idempotency_key":"k1","approval_id":"`+ids.NewV7().String()+`"}`))
	if bad := (*BadArgsError)(nil); !errors.As(err, &bad) {
		t.Fatalf("err = %v, want the arguments refused", err)
	}
	if len(route.keys) != 0 {
		t.Fatal("the route ran under an approval it could not replay")
	}
}
