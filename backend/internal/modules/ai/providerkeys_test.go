// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Where a BYOK key is resolved from, and what happens when the vault cannot
// answer. The failure paths matter more than the happy one: each of them
// decides whether an installation keeps serving or goes dark.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keyRow is the row a program registers for a provider's key.
func keyRow(t *testing.T, s *config.Setting[string]) settings.Definition {
	t.Helper()
	for _, def := range configtest.MustLoadEnv(t, Settings()).StoredDefinitions() {
		if def.Key() == s.Key() {
			return def
		}
	}
	t.Fatalf("no provider key is keyed %s", s.Key())
	return nil
}

// A key the environment carries is read through its setting, with no database
// and no vault: a boot value is read where it is.
func TestAKeyTheEnvironmentSetsResolvesThroughItsSetting(t *testing.T) {
	cfg := configtest.MustLoadEnv(t, Settings(), "ANTHROPIC_API_KEY=not-sealed-yet")
	ctx := principal.SystemActing(context.Background(), "test")

	got := KeyLookup(ctx, cfg, slog.New(slog.DiscardHandler))("ANTHROPIC_API_KEY")

	if got != "not-sealed-yet" {
		t.Errorf("resolved %q; a key the environment sets must answer", got)
	}
}

// The lookup answers provider keys only. Another variable is not read from the
// process environment behind the registry's back.
func TestANonCredentialVariableIsNotThisLookupsToAnswer(t *testing.T) {
	t.Setenv("MARGINCE_LOG_LEVEL", "debug")
	got := KeyLookup(context.Background(), nil, slog.New(slog.DiscardHandler))("MARGINCE_LOG_LEVEL")
	if got != "" {
		t.Errorf("resolved %q, want nothing for a variable that is no provider key", got)
	}
}

// A key whose row cannot be read, here for want of a database, answers empty.
// The log names the setting and never a key.
func TestAKeyWithNoDatabaseToReadAnswersEmptyAndLogsNoSecret(t *testing.T) {
	cfg := configtest.MustLoadEnv(t, Settings(), "OPENAI_API_KEY=sk-never-logged")
	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))

	got := KeyLookup(principal.SystemActing(context.Background(), "test"), cfg, log)("GEMINI_API_KEY")

	if got != "" {
		t.Errorf("resolved %q from an unreadable key, want empty", got)
	}
	if !strings.Contains(logged.String(), "no database") || !strings.Contains(logged.String(), GeminiKey.Key()) ||
		strings.Contains(logged.String(), "sk-never-logged") {
		t.Errorf("the log %q must name the setting and no key", logged.String())
	}
}

// A provider that takes no key is refused on the way in, and a row naming no
// sealed blob is refused at the settings layer. A credential stored against a
// name nothing routes is one nobody can use and nobody will remember is there.
func TestAKeyForAProviderThatTakesNoKeyIsRefused(t *testing.T) {
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	err := (&ProviderKeyStore{}).Set(admin, providerOllama, ProviderCredential{APIKey: "kv:something"})
	if _, ok := errors.AsType[settings.InvalidValue](err); !ok {
		t.Errorf("a key was accepted for a local provider that takes no api key: %v", err)
	}
	if err := keyRow(t, GeminiKey).ValidateJSON([]byte(`""`)); err == nil {
		t.Error("an empty credential reference was accepted")
	}
	if err := keyRow(t, GeminiKey).ValidateJSON([]byte(`"kv:something"`)); err != nil {
		t.Errorf("a real ref was refused: %v", err)
	}
}

// The sealed credentials survive a data reset for the same reason the binding
// does: a reset wipes an installation's data, not the credentials it uses to
// reach the vendors it chose. Losing them would leave refs pointing at blobs
// nothing can name.
func TestSealedCredentialsSurviveADataReset(t *testing.T) {
	for _, s := range Settings() {
		if !keyRow(t, keySettings[providerOf(t, s)]).SurvivesDataReset() {
			t.Errorf("a data reset would delete %s, stranding its sealed key in the vault", s.Key())
		}
	}
}

func providerOf(t *testing.T, s config.Any) string {
	t.Helper()
	for provider, setting := range keySettings {
		if setting.Key() == s.Key() {
			return provider
		}
	}
	t.Fatalf("%s is no provider's key", s.Key())
	return ""
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

// A lookup built with no configuration behind it answers empty rather than
// panicking. It is the shape a role that resolved no config hands over, and a
// nil dereference here would take the boot with it.
func TestALookupWithNoEnvironmentAnswersEmpty(t *testing.T) {
	got := KeyLookup(principal.SystemActing(context.Background(), "test"), nil, slog.New(slog.DiscardHandler))("GEMINI_API_KEY")
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

	// The decisions lane binds apart from the tiers, so an installation on
	// Gemini tiers whose ONLY TypeSafe binding is the lane still needs the
	// TypeSafe key.
	t.Run("the decisions lane names its own key", func(t *testing.T) {
		cfg := RoutingConfig{
			Tiers:      map[Tier]ProviderConfig{"fast": {Provider: providerGemini}, "deep": {Provider: providerGemini}},
			Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerGemini}},
			Decisions:  &DecisionsConfig{Provider: providerJev, Model: "jev-1.13.0"},
		}
		if got := cfg.CloudProvidersBound(); !slices.Contains(got, providerJev) || !slices.Contains(got, providerGemini) {
			t.Errorf("a Gemini binding with a jev decisions lane reports %v, want %s and %s", got, providerGemini, providerJev)
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

// A key a boot source sets refuses its removal before the vault is asked, as
// a save does: a role with no vault still answers the lock.
func TestRemovingABootSetKeyIsLockedBeforeTheVaultIsAsked(t *testing.T) {
	store := NewProviderKeyStore(configtest.MustLoadEnv(t, Settings(), "GEMINI_API_KEY=sk-boot"))
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	if err := store.Remove(admin, providerGemini); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("removing a boot-set key with no vault = %v, want a conflict", err)
	}
}
