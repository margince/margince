// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where a BYOK key is resolved from, and what happens when the vault cannot
// answer. The failure paths matter more than the happy one: each of them
// decides whether an installation keeps serving or goes dark.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sealed(t *testing.T, vault keyvault.Vault, ws ids.WorkspaceID, secret string) string {
	t.Helper()
	ref, err := vault.Put(context.Background(), ws, []byte(secret))
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}
	return string(ref)
}

func TestASealedKeyIsResolvedFromTheVaultRatherThanTheEnvironment(t *testing.T) {
	ctx := context.Background()
	vault := keyvault.NewMemory()
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	refs := map[string]string{providerGemini: sealed(t, vault, ws, "from-the-vault")}
	// The environment still carries the OLD value, which is the ordinary state
	// of an installation that has not yet dropped the variable. The vault must
	// win, or sealing changes nothing.
	env := config.Static(map[string]string{"GEMINI_API_KEY": "from-the-environment"})

	got := SealedKeys(ctx, vault, ws, refs, env)("GEMINI_API_KEY")

	if got != "from-the-vault" {
		t.Errorf("resolved %q, want the sealed value — the environment outranked the vault", got)
	}
}

// The migration path: a key the environment carries and the vault has not
// sealed yet still answers, so an installation keeps working across the move
// without anyone doing anything.
func TestAnUnsealedKeyStillResolvesFromTheEnvironment(t *testing.T) {
	ctx := context.Background()
	env := config.Static(map[string]string{"ANTHROPIC_API_KEY": "not-sealed-yet"})

	got := SealedKeys(ctx, keyvault.NewMemory(), ids.From[ids.WorkspaceKind](ids.NewV7()), nil, env)("ANTHROPIC_API_KEY")

	if got != "not-sealed-yet" {
		t.Errorf("resolved %q; an unsealed key must still answer from the environment", got)
	}
}

// A ref the vault cannot resolve — deleted, or sealed under another workspace —
// falls back rather than failing. An installation whose AI went dark because a
// vault read failed, with no sentence naming the vault, is a worse outcome than
// one still serving on the variable it always had.
func TestAnUnresolvableRefFallsBackToTheEnvironment(t *testing.T) {
	ctx := context.Background()
	vault := keyvault.NewMemory()
	ws := ids.From[ids.WorkspaceKind](ids.NewV7())
	// A ref sealed under a DIFFERENT workspace. The vault refuses it, which is
	// the isolation guarantee working, not a fault to propagate.
	elsewhere := sealed(t, vault, ids.From[ids.WorkspaceKind](ids.NewV7()), "another workspace's key")
	env := config.Static(map[string]string{"OPENAI_API_KEY": "still-here"})

	got := SealedKeys(ctx, vault, ws, map[string]string{providerOpenAI: elsewhere}, env)("OPENAI_API_KEY")

	if got == "another workspace's key" {
		t.Fatal("a ref sealed under another workspace resolved; workspace isolation is broken")
	}
	if got != "still-here" {
		t.Errorf("resolved %q, want the environment's value", got)
	}
}

// A variable that is not a BYOK key is none of this lookup's business — it is
// still a config.Lookup, and the rest of the process reads other variables
// through the same shape.
func TestANonCredentialVariableIsPassedStraightThrough(t *testing.T) {
	env := config.Static(map[string]string{"MARGINCE_LOG_LEVEL": "debug"})
	got := SealedKeys(context.Background(), keyvault.NewMemory(), ids.From[ids.WorkspaceKind](ids.NewV7()), nil, env)("MARGINCE_LOG_LEVEL")
	if got != "debug" {
		t.Errorf("resolved %q, want debug", got)
	}
}

// A ref for a provider that takes no key is refused on the way in. A credential
// sealed against a name nothing routes is one nobody can use and nobody will
// remember is there.
func TestARefForAProviderThatTakesNoKeyIsRefused(t *testing.T) {
	if err := ProviderKeys.ValidateJSON([]byte(`{"ollama":"kv:something"}`)); err == nil {
		t.Error("a ref was accepted for a local provider that takes no api key")
	}
	if err := ProviderKeys.ValidateJSON([]byte(`{"gemini":""}`)); err == nil {
		t.Error("an empty credential reference was accepted")
	}
	if err := ProviderKeys.ValidateJSON([]byte(`{"gemini":"kv:something"}`)); err != nil {
		t.Errorf("a real ref was refused: %v", err)
	}
}

// The sealed credentials survive a data reset for the same reason the binding
// does: a reset wipes an installation's data, not the credentials it uses to
// reach the vendors it chose. Losing them would leave refs pointing at blobs
// nothing can name.
func TestSealedCredentialsSurviveADataReset(t *testing.T) {
	if !ProviderKeys.SurvivesDataReset() {
		t.Error("a data reset would delete the credential refs, stranding every sealed key in the vault")
	}
}

// The sealer's provider list is DERIVED from cloudKeyEnv, and this is what
// makes that claim true rather than a comment. A vendor added there must be
// sealed the day it appears; one the derivation missed would keep its key in
// the environment forever, and nothing else in the tree would say so.
func TestEveryProviderWithAKeyIsOneTheSealerCoversAndNamesAVariable(t *testing.T) {
	covered := map[string]bool{}
	for _, provider := range CloudProvidersNeedingKeys() {
		covered[provider] = true
		if KeyEnvVarFor(provider) == "" {
			t.Errorf("%s is sealed but names no environment variable, so nothing can supply its key", provider)
		}
	}
	for provider := range cloudKeyEnv {
		if !covered[provider] {
			t.Errorf("%s takes an api key but the sealer does not cover it — its key stays in the environment", provider)
		}
	}
	// And nothing local sneaks in: a provider that needs no key must not be
	// asked for one, or an operator is told to set a variable that means
	// nothing.
	for _, provider := range CloudProvidersNeedingKeys() {
		if localProviders[provider] {
			t.Errorf("%s runs locally and needs no api key", provider)
		}
	}
	if len(covered) == 0 {
		t.Fatal("the sealer covers no provider at all; this gate would pass vacuously")
	}
}

// A lookup built with no environment behind it answers empty rather than
// panicking. It is the shape a role that resolved no config hands over, and a
// nil dereference here would take the boot with it.
func TestALookupWithNoEnvironmentAnswersEmpty(t *testing.T) {
	got := SealedKeys(context.Background(), keyvault.NewMemory(), ids.From[ids.WorkspaceKind](ids.NewV7()), nil, nil)("GEMINI_API_KEY")
	if got != "" {
		t.Errorf("resolved %q with no environment and no vault entry, want empty", got)
	}
}

// CloudProvidersBound is what the setup report gates onboarding on: it names the
// vendors a binding uses that need a key. Getting it wrong in either direction
// is a real stall — too many and a complete installation is told it is not, too
// few and onboarding waves through a binding that cannot serve a single call.
func TestCloudProvidersBoundNamesEveryCloudVendorAndOnlyThose(t *testing.T) {
	// Derived from the owner rather than restated: a hard-coded list here would
	// be a second copy of cloudKeyEnv and would go quietly short the day
	// somebody adds a vendor — which is the one failure this cannot afford.
	cloud := CloudProvidersNeedingKeys()
	if len(cloud) < 2 {
		t.Fatalf("CloudProvidersNeedingKeys() = %v; the tree carries several cloud vendors, so a near-empty list means the source moved", cloud)
	}

	t.Run("every cloud vendor a tier names is reported", func(t *testing.T) {
		for _, provider := range cloud {
			cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{"fast": {Provider: provider}}}
			if got := cfg.CloudProvidersBound(); len(got) != 1 || got[0] != provider {
				t.Errorf("a binding on %q reports %v, want exactly [%s]", provider, got, provider)
			}
		}
	})

	// The embeddings lane binds separately from the chat tiers, and a per-tier
	// walk forgets it — an installation whose ONLY cloud vendor is its embedder
	// would be called complete with no key for it.
	t.Run("the embeddings lane counts even when every tier is local", func(t *testing.T) {
		cfg := RoutingConfig{
			Tiers:      map[Tier]ProviderConfig{"fast": {Provider: providerOllama}},
			Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: cloud[0]}},
		}
		if got := cfg.CloudProvidersBound(); len(got) != 1 || got[0] != cloud[0] {
			t.Errorf("an embeddings-only cloud binding reports %v, want [%s]", got, cloud[0])
		}
	})

	// A sovereign installation is complete with no keys at all: local providers
	// are absent by construction, and refusing them would block the deployment
	// that most deliberately chose them.
	t.Run("a fully local binding names nothing", func(t *testing.T) {
		cfg := RoutingConfig{
			Tiers:      map[Tier]ProviderConfig{"fast": {Provider: providerOllama}, "deep": {Provider: ProviderFake}},
			Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOllama}},
		}
		if got := cfg.CloudProvidersBound(); len(got) != 0 {
			t.Errorf("a fully local binding reports %v, want nothing", got)
		}
	})

	// One vendor across several tiers is one entry, not three: the report asks
	// which credentials are needed, and a duplicate would have onboarding
	// waiting on a key it already holds.
	t.Run("one vendor named by several tiers is reported once", func(t *testing.T) {
		cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
			"fast": {Provider: cloud[0]}, "deep": {Provider: cloud[0]}, "vision": {Provider: cloud[0]},
		}}
		if got := cfg.CloudProvidersBound(); len(got) != 1 {
			t.Errorf("three tiers on %q report %v, want one entry", cloud[0], got)
		}
	})
}
