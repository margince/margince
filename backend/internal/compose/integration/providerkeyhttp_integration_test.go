// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The credential surface over the real router, with a real vault and a real
// admin session — the wiring the store's own tests cannot see.
//
// What it is here to prove is a negative: that no response on this surface, on
// any path, carries the key back. A store test can assert the setting holds a
// ref; only this can assert that the HTTP layer never echoes the bytes it was
// handed.

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	"github.com/gradionhq/margince/backend/internal/platform/keyvault"
)

const providerKeySecret = "sk-live-must-never-come-back"

func setupProviderKeyApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating a test root key: %v", err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: key, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatalf("building the local vault: %v", err)
	}
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault))
	apptest.BootstrapWorkspaceSession(t, e, "Provider Keys", "ada@example.com", "Ada Admin")
	return e
}

func TestTheKeyIsWriteOnlyOverHTTP(t *testing.T) {
	e := setupProviderKeyApp(t)

	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/gemini",
		apptest.AnyMap{"api_key": providerKeySecret}, nil, nil); code != 204 {
		t.Fatalf("PUT provider key = %d, want 204", code)
	}

	// The list is the only read path, and it must not carry the bytes — nor a
	// length, a prefix or a masked tail, each of which narrows a brute force
	// while feeling harmless.
	var raw map[string]any
	if code := e.Call(t, "GET", "/v1/ai/provider-keys", nil, nil, &raw); code != 200 {
		t.Fatalf("GET provider keys = %d, want 200", code)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	body := string(encoded)
	if strings.Contains(body, providerKeySecret) {
		t.Fatalf("the list carried the credential: %s", body)
	}
	// A fragment long enough to be recognisable must not appear either.
	if strings.Contains(body, providerKeySecret[:12]) {
		t.Fatalf("the list carried a recognisable fragment of the credential: %s", body)
	}
	if !strings.Contains(body, "gemini") {
		t.Fatalf("the list does not mention the provider that was just configured: %s", body)
	}
}

func TestTheListSaysWhichVendorsAreConfigured(t *testing.T) {
	e := setupProviderKeyApp(t)

	var before struct {
		Providers []struct {
			Provider   string `json:"provider"`
			Configured bool   `json:"configured"`
			EnvVar     string `json:"env_var"`
		} `json:"providers"`
	}
	if code := e.Call(t, "GET", "/v1/ai/provider-keys", nil, nil, &before); code != 200 {
		t.Fatalf("GET = %d", code)
	}
	// An installation that has configured nothing is exactly the one that needs
	// the screen, so every servable vendor is listed.
	if len(before.Providers) == 0 {
		t.Fatal("an unconfigured installation was shown no vendors at all")
	}
	for _, p := range before.Providers {
		if p.Configured {
			t.Errorf("%s reads as configured before anything was stored", p.Provider)
		}
		if p.EnvVar == "" {
			t.Errorf("%s names no environment variable, so an operator cannot see what seeded it", p.Provider)
		}
	}

	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/openai",
		apptest.AnyMap{"api_key": "sk-openai"}, nil, nil); code != 204 {
		t.Fatal("PUT failed")
	}
	var after struct {
		Providers []struct {
			Provider   string `json:"provider"`
			Configured bool   `json:"configured"`
		} `json:"providers"`
	}
	if code := e.Call(t, "GET", "/v1/ai/provider-keys", nil, nil, &after); code != 200 {
		t.Fatalf("GET after = %d", code)
	}
	// Tracked rather than only asserted inside the loop: a response that OMITS
	// openai altogether would satisfy every check below by never reaching them,
	// and "the vendor is missing from the list" is exactly the regression this
	// case is here to catch.
	sawOpenAI := false
	for _, p := range after.Providers {
		if p.Provider == "openai" {
			sawOpenAI = true
			if !p.Configured {
				t.Error("the vendor just configured does not read as configured")
			}
			continue
		}
		if p.Configured {
			t.Errorf("%s became configured by a write to another vendor", p.Provider)
		}
	}
	if !sawOpenAI {
		t.Error("the list omits the vendor just configured, so nothing above was checked against it")
	}
}

func TestRemovingOverHTTPIsIdempotent(t *testing.T) {
	e := setupProviderKeyApp(t)

	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/anthropic",
		apptest.AnyMap{"api_key": "sk-anthropic"}, nil, nil); code != 204 {
		t.Fatal("PUT failed")
	}
	if code := e.Call(t, "DELETE", "/v1/ai/provider-keys/anthropic", nil, nil, nil); code != 204 {
		t.Fatalf("first DELETE = %d, want 204", code)
	}
	// The caller asked for a state, and that state now holds — so a retry is
	// safe and says so, rather than 404ing a request that already succeeded.
	if code := e.Call(t, "DELETE", "/v1/ai/provider-keys/anthropic", nil, nil, nil); code != 204 {
		t.Errorf("second DELETE = %d, want 204 — removing what is gone is the state the caller asked for", code)
	}
}

func TestAVendorThatTakesNoKeyIsRefusedOverHTTP(t *testing.T) {
	e := setupProviderKeyApp(t)
	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/ollama",
		apptest.AnyMap{"api_key": "sk-local-needs-none"}, nil, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("PUT for a local provider = %d, want 422", code)
	}
	// An empty key is the caller's mistake too: removing a credential is
	// DELETE, not a write of nothing.
	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/gemini",
		apptest.AnyMap{"api_key": "   "}, nil, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("PUT with a blank key = %d, want 422", code)
	}
	// And an OMITTED key, which is a different path from a blank one. Marking
	// the field writeOnly is what keeps it out of every generated response
	// type, and the cost is that the generated request field became a pointer
	// with `omitempty` — so a body carrying no `api_key` at all decodes
	// cleanly and reaches the store. The store's refusal is therefore the only
	// thing standing between an absent field and a sealed zero-length key that
	// would read as configured and authenticate nothing.
	if code := e.Call(t, "PUT", "/v1/ai/provider-keys/gemini",
		apptest.AnyMap{}, nil, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("PUT with no api_key at all = %d, want 422", code)
	}
}
