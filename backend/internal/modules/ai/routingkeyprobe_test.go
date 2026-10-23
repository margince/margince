// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// A binding the stack cannot rebind is refused at the save.
//
// Accepted, the API and worker refuse it every 30 s and keep serving what they
// had, while the routing read answers the saved binding. The screen then
// reports a model that is not running, and nothing says why.
func TestASaveIsRefusedForAProviderHoldingNoKey(t *testing.T) {
	anthropicEnv := cloudKeyEnv[providerAnthropic]
	if anthropicEnv == "" {
		t.Fatal("anthropic is not a key-bearing provider, so this case proves nothing")
	}
	// Every key but the one the save needs, so the refusal cannot come from
	// an unrelated provider.
	withoutAnthropic := map[string]string{}
	for provider, env := range cloudKeyEnv {
		if provider != providerAnthropic {
			withoutAnthropic[env] = "k"
		}
	}
	store := &RoutingStore{keys: config.Static(withoutAnthropic)}

	next := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"},
	}}

	err := store.refuseKeylessBindings(context.Background(), RoutingConfig{}, next)
	if err == nil {
		t.Fatal("a save binding a keyless provider was accepted")
	}
	for _, want := range []string{"tier local_small", anthropicEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// The decisions lane is a binding site of its own, and the one the report was
// filed against.
func TestASaveIsRefusedForAKeylessDecisionsLane(t *testing.T) {
	store := &RoutingStore{keys: config.Static(map[string]string{})}
	next := RoutingConfig{Decisions: &DecisionsConfig{
		Provider: providerAnthropic, Model: "claude-x",
	}}

	err := store.refuseKeylessBindings(context.Background(), RoutingConfig{}, next)
	if err == nil {
		t.Fatal("a keyless decisions lane was accepted")
	}
	if !strings.Contains(err.Error(), "decisions") {
		t.Errorf("the refusal %q does not name the lane", err)
	}
}

func TestASaveIsAdmittedWhenTheProviderHoldsItsKey(t *testing.T) {
	store := &RoutingStore{keys: allCloudKeys(t)}
	next := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"},
	}}

	if err := store.refuseKeylessBindings(context.Background(), RoutingConfig{}, next); err != nil {
		t.Errorf("a save of a keyed provider was refused: %v", err)
	}
}

// Only what the save introduces is checked. An installation whose key was
// removed after the binding was stored keeps its document editable, the same
// bound the Vertex probe holds.
func TestAnUntouchedKeylessBindingDoesNotRefuseALaterSave(t *testing.T) {
	store := &RoutingStore{keys: config.Static(map[string]string{})}
	stored := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierLocalSmall: {Provider: providerAnthropic, Model: "claude-x"},
	}}
	next := stored
	next.Profile = ProfileSovereign

	if err := store.refuseKeylessBindings(context.Background(), stored, next); err != nil {
		t.Errorf("an edit leaving the keyless binding alone was refused: %v", err)
	}
}
