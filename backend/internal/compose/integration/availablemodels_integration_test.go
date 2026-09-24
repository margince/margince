// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Asking a vendor what it serves, end to end.
//
// Every one of these needs a real database, because each turns on what the
// INSTALLATION holds rather than on what this process was handed: the profile
// that decides whether a vendor may be reached at all, the grant that decides
// whether this reader may ask, and the stored binding that supplies the host.
// None can be shown against a store built from a literal.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The offline fake publishes a list, so the whole path — grant, profile,
// binding, adapter, decode — is exercisable with no vendor and no network.
func TestAvailableModelsAsksTheBoundAdapter(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "fake-stored")); err != nil {
		t.Fatalf("storing the binding: %v", err)
	}

	got, err := store.ListAvailableModels(e.adminRoutingCtx(), ai.AvailableModelsQuery{Provider: "fake"})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if got.Unavailable != ai.AvailabilityOK {
		t.Fatalf("the fake should answer, got %q", got.Unavailable)
	}
	if len(got.Models) == 0 {
		t.Fatal("the fake published no models")
	}
}

// The profile is a claim about where inference may happen, and a list call is
// egress like any other. Refused BEFORE the adapter is built, so a sovereign
// installation never opens a connection to a cloud vendor to find out.
func TestSovereignRefusesToAskACloudVendor(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), sovereignRouting(t)); err != nil {
		t.Fatalf("storing the sovereign binding: %v", err)
	}

	got, err := store.ListAvailableModels(e.adminRoutingCtx(), ai.AvailableModelsQuery{Provider: "anthropic"})
	if err != nil {
		t.Fatalf("a refused vendor is a state, not an error: %v", err)
	}
	if got.Unavailable != ai.AvailabilityProfileForbids {
		t.Fatalf("sovereign must forbid a cloud vendor, got %q", got.Unavailable)
	}
	if len(got.Models) != 0 {
		t.Fatalf("a forbidden vendor must carry no models: %+v", got.Models)
	}

	// And a LOCAL vendor is still askable under the same profile — the refusal
	// is about egress, not about the surface being switched off.
	local, err := store.ListAvailableModels(e.adminRoutingCtx(), ai.AvailableModelsQuery{Provider: "fake"})
	if err != nil {
		t.Fatalf("listing a local vendor: %v", err)
	}
	if local.Unavailable == ai.AvailabilityProfileForbids {
		t.Fatal("sovereign forbade a local vendor")
	}
}

// OpenRouter is cloud egress like any other broker, but it is reached with NO
// stored binding at all — its read skips SelectBrain entirely and asks the
// vendor unauthenticated. The profile check has to run before that shortcut
// gets a chance to bypass it, or a sovereign installation would read a public
// vendor's list regardless of its own posture: the profile refuses an
// unbound vendor exactly as it refuses a bound one, and this proves it does
// so BEFORE any network call, the same way the bound case above does.
func TestSovereignRefusesOpenRouterToo(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), sovereignRouting(t)); err != nil {
		t.Fatalf("storing the sovereign binding: %v", err)
	}

	got, err := store.ListAvailableModels(e.adminRoutingCtx(), ai.AvailableModelsQuery{Provider: "openrouter"})
	if err != nil {
		t.Fatalf("a refused vendor is a state, not an error: %v", err)
	}
	if got.Unavailable != ai.AvailabilityProfileForbids {
		t.Fatalf("sovereign must forbid openrouter, got %q", got.Unavailable)
	}
	if len(got.Models) != 0 {
		t.Fatalf("a forbidden vendor must carry no models: %+v", got.Models)
	}
}

// A cloud vendor with no credential is a state a reader can act on — paste a
// key — rather than a failure, and it is reported without ever reaching the
// network.
func TestACloudVendorWithNoKeyReportsItRatherThanFailing(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "fake-stored")); err != nil {
		t.Fatalf("storing the binding: %v", err)
	}

	got, err := store.ListAvailableModels(e.adminRoutingCtx(), ai.AvailableModelsQuery{Provider: "anthropic"})
	if err != nil {
		t.Fatalf("an unkeyed vendor is a state, not an error: %v", err)
	}
	if got.Unavailable != ai.AvailabilityNoKey {
		t.Fatalf("want no_key, got %q", got.Unavailable)
	}
}

// The grant is the one refusal that IS an error: it is about the reader rather
// than about the vendor, and a seat without it must not learn which vendors this
// installation can reach.
func TestAvailableModelsNeedsTheRoutingReadGrant(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			// Every grant except the one this read asks for.
			Objects:  map[string]principal.ObjectGrant{"automation": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := store.ListAvailableModels(ctx, ai.AvailableModelsQuery{Provider: "fake"}); err == nil {
		t.Fatal("a seat without ai_routing:read was served the vendor list")
	} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("want a permission denial, got %v", err)
	}
}

// Saving a gemini_vertex binding asks its location for the model first, on
// the real store: with no service-account key held, nothing can be asked, and
// the save is refused naming the lane rather than storing a binding nobody
// checked.
func TestSavingAVertexBindingIsRefusedWhenItsModelCannotBeChecked(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	cfg, err := ai.ParseRouting([]byte(`profile: eu_resident
tiers:
  local_small: {provider: fake, model: fake-local}
  premium: {provider: gemini_vertex, location: eu, model: gemini-3.5-flash}
embeddings: {provider: fake, model: fake-embed, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("parsing the vertex fixture: %v", err)
	}

	_, err = store.Replace(e.adminRoutingCtx(), cfg)

	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Reason, "tier premium") || !strings.Contains(invalid.Reason, "service-account key") {
		t.Fatalf("want a 422 naming the tier and the missing key, got %v", err)
	}
	if stored, err := store.Get(e.adminRoutingCtx()); err != nil || !stored.Unconfigured() {
		t.Errorf("the refused binding was stored anyway: %+v, %v", stored, err)
	}
}

func sovereignRouting(t *testing.T) ai.RoutingConfig {
	t.Helper()
	cfg, err := ai.ParseRouting([]byte(`profile: sovereign
tiers:
  local_small: {provider: fake, model: fake-local}
  cheap_cloud: {provider: fake, model: fake-local}
  premium: {provider: fake, model: fake-local}
  frontier: {provider: fake, model: fake-local}
embeddings: {provider: fake, model: fake-embed, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("parsing the sovereign fixture: %v", err)
	}
	return cfg
}
