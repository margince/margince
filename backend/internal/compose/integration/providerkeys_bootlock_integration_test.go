// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A provider key a boot source sets: read where it is, never copied into the
// vault, and locked against a change over HTTP.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const envProviderKey = "sk-env-secret-never-shown"

// keyEnviron sets the anthropic key and a vault root key. So the boot path has
// a vault it could seal an environment key into.
func keyEnviron() []string {
	rootKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	return []string{"ANTHROPIC_API_KEY=" + envProviderKey, keyvault.EnvRootKey + "=" + rootKey}
}

// installationCtx is the system on the installation's workspace, as the boot
// and the routing path read settings.
func installationCtx(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	ws := apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool)
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "test-provider-keys")
}

func secretsHeld(t *testing.T, vault keyvault.Vault) int {
	t.Helper()
	counted, ok := vault.(interface{ Count() int })
	if !ok {
		t.Fatal("the test vault cannot count what it holds")
	}
	return counted.Count()
}

func vaultRows(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM vault_secret`).Scan(&n); err != nil {
		t.Fatalf("counting sealed blobs: %v", err)
	}
	return n
}

// sealedEarlier records a key as the release before this one stored it: sealed
// in vault, its ref in the setting's row.
func sealedEarlier(ctx context.Context, t *testing.T, e *apptest.AppEnv, vault keyvault.Vault, s *config.Setting[string], key string) string {
	t.Helper()
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		t.Fatal("no workspace to seal under")
	}
	ref, err := vault.Put(ctx, ids.From[ids.WorkspaceKind](ws), []byte(key))
	if err != nil {
		t.Fatalf("sealing the earlier key: %v", err)
	}
	if err := compose.NewSettingsStore(e.Pool).SetRaw(ctx, s.Key(), json.RawMessage(`"`+string(ref)+`"`)); err != nil {
		t.Fatalf("recording the earlier key: %v", err)
	}
	return string(ref)
}

func TestAnEnvProviderKeyWinsAndIsNeverCopiedIntoTheVault(t *testing.T) {
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithEnv(t, keyEnviron(), compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)
	ctx := installationCtx(t, e)
	earlier := sealedEarlier(ctx, t, e, vault, ai.AnthropicKey, "sk-stored-before")
	cfg := compose.WithVault(compose.StoredConfig(e.Pool), vault, discard())

	key, origin, err := config.StoredValWithSource(ctx, cfg, ai.AnthropicKey)
	if err != nil || key != envProviderKey || origin.Name != "ANTHROPIC_API_KEY" {
		t.Fatalf("read %q from %+v (%v), want the environment's key", key, origin, err)
	}

	// The boot path that used to seal an environment key into the vault.
	if _, err := compose.ResolveRouting(context.Background(), e.Pool, "", compose.RoutingKeys(config.Bound()), discard()); err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if n := vaultRows(t, e); n != 0 {
		t.Errorf("%d blobs sealed in the boot's vault; an env key must not be sealed into the vault at boot", n)
	}
	if n := secretsHeld(t, vault); n != 1 {
		t.Errorf("the app's vault holds %d secrets, want only the 1 sealed before", n)
	}
	ref, stored, err := config.SealedRef(ctx, cfg, ai.AnthropicKey)
	if err != nil || !stored || ref != earlier {
		t.Errorf("the earlier sealed copy reads %q stored=%v (%v); it is ignored, not deleted", ref, stored, err)
	}

	var listed crmcontracts.AiProviderKeyList
	if status := e.Call(t, http.MethodGet, "/v1/ai/provider-keys", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("GET /ai/provider-keys → %d", status)
	}
	for _, p := range listed.Providers {
		if p.Provider == "anthropic" && !p.Configured {
			t.Error("anthropic reads unkeyed though the environment sets its key")
		}
	}
}

// A key a boot source sets refuses every write over HTTP, before the value
// sent is judged. It refuses a change and its own value, which would confirm a
// guess of it. It refuses a bad credential, both fields, and a removal.
func TestAProviderKeySetByEnvIsLockedAgainstAChange(t *testing.T) {
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithEnv(t, keyEnviron(), compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)

	put := func(body crmcontracts.AiProviderKeyInput) int {
		return e.Call(t, http.MethodPut, "/v1/ai/provider-keys/anthropic", body, nil, nil)
	}
	other, own, empty, file := "sk-another", envProviderKey, "", "{}"
	for name, body := range map[string]crmcontracts.AiProviderKeyInput{
		"another key":           {ApiKey: &other},
		"the boot key itself":   {ApiKey: &own},
		"an empty key":          {ApiKey: &empty},
		"both credential kinds": {ApiKey: &other, ServiceAccountJson: &file},
	} {
		if status := put(body); status != http.StatusConflict {
			t.Errorf("PUT %s over the locked key → %d, want 409", name, status)
		}
	}
	if status := e.Call(t, http.MethodDelete, "/v1/ai/provider-keys/anthropic", nil, nil, nil); status != http.StatusConflict {
		t.Errorf("DELETE of the locked key → %d, want 409", status)
	}
	if rows := settingRows(t, e, ai.AnthropicKey.Key()); rows != 0 {
		t.Errorf("the locked key wrote %d rows, want none", rows)
	}
	if n := secretsHeld(t, vault); n != 0 {
		t.Errorf("the refused writes sealed %d secrets, want none", n)
	}
	// A key no boot source sets is still the admin's to set.
	if status := e.Call(t, http.MethodPut, "/v1/ai/provider-keys/openai",
		crmcontracts.AiProviderKeyInput{ApiKey: &other}, nil, nil); status != http.StatusNoContent {
		t.Errorf("PUT of an unlocked key → %d, want 204", status)
	}
}

// The boot key appears in no error, no log line, no API body and no overview:
// only its name and origin do.
func TestASecretNeverAppearsInAnErrorALogOrTheOverview(t *testing.T) {
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithEnv(t, keyEnviron(), compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)
	ctx := installationCtx(t, e)
	seen := map[string]string{}

	var refusal, listed json.RawMessage
	other := "sk-another"
	e.Call(t, http.MethodPut, "/v1/ai/provider-keys/anthropic", crmcontracts.AiProviderKeyInput{ApiKey: &other}, nil, &refusal)
	e.Call(t, http.MethodGet, "/v1/ai/provider-keys", nil, nil, &listed)
	seen["the 409 body"], seen["the key list"] = string(refusal), string(listed)

	err := config.Save(ctx, compose.WithVault(compose.StoredConfig(e.Pool), vault, discard()), ai.AnthropicKey, "sk-another")
	var locked config.LockedError
	if !errors.As(err, &locked) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a save over the boot key = %v, want a LockedError", err)
	}
	seen["the lock error"] = err.Error()

	entries, err := compose.StoredConfig(e.Pool).StoredEntries(ctx)
	if err != nil {
		t.Fatalf("reading the overview: %v", err)
	}
	for _, entry := range entries {
		if entry.Key == ai.AnthropicKey.Key() && (entry.Value != nil || entry.Origin.Name != "ANTHROPIC_API_KEY") {
			t.Errorf("the overview shows %+v, want the name and origin and no value", entry)
		}
	}
	overview, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("encoding the overview: %v", err)
	}
	seen["the overview"] = string(overview)
	seen["the trace"] = strings.Join(config.Bound().TraceLines(), "\n")

	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sealedEarlier(ctx, t, e, vault, ai.GeminiKey, "sk-unopenable")
	if _, err := compose.ResolveRouting(context.Background(), e.Pool, "", compose.RoutingKeys(config.Bound()), log); err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	ai.KeyLookup(ctx, compose.StoredConfig(e.Pool), log)("GEMINI_API_KEY")
	seen["the log"] = logged.String()

	for where, text := range seen {
		if strings.Contains(text, envProviderKey) {
			t.Errorf("%s carries the boot key: %s", where, text)
		}
	}
	if !strings.Contains(seen["the log"], ai.GeminiKey.Key()) {
		t.Errorf("the unreadable key was not logged by name, so the check above read nothing: %s", seen["the log"])
	}
}

// Save on a secret setting seals the value and records only the ref, and
// StoredVal opens it. A rotation retires the replaced blob, and a removal
// retires the last one.
func TestASavedSecretIsSealedAndReadBack(t *testing.T) {
	vault := keyvault.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)
	ctx := installationCtx(t, e)
	cfg := compose.WithVault(compose.StoredConfig(e.Pool), vault, discard())

	if err := config.Save(ctx, cfg, ai.GeminiKey, "sk-first"); err != nil {
		t.Fatalf("saving the key: %v", err)
	}
	var row string
	if err := e.Pool.QueryRow(ctx, `SELECT value::text FROM setting WHERE key = $1`, ai.GeminiKey.Key()).Scan(&row); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if strings.Contains(row, "sk-first") || secretsHeld(t, vault) != 1 {
		t.Fatalf("the row holds %s with %d sealed secrets, want a ref to the one sealed key", row, secretsHeld(t, vault))
	}
	if key, err := config.StoredVal(ctx, cfg, ai.GeminiKey); err != nil || key != "sk-first" {
		t.Fatalf("read back %q (%v), want the saved key", key, err)
	}

	if err := config.Save(ctx, cfg, ai.GeminiKey, "sk-second"); err != nil {
		t.Fatalf("rotating the key: %v", err)
	}
	if key, err := config.StoredVal(ctx, cfg, ai.GeminiKey); err != nil || key != "sk-second" || secretsHeld(t, vault) != 1 {
		t.Fatalf("after a rotation read %q (%v) with %d sealed, want the new key and the old retired", key, err, secretsHeld(t, vault))
	}

	if err := config.RemoveSecret(ctx, cfg, ai.GeminiKey); err != nil {
		t.Fatalf("removing the key: %v", err)
	}
	if key, err := config.StoredVal(ctx, cfg, ai.GeminiKey); err != nil || key != "" || secretsHeld(t, vault) != 0 {
		t.Fatalf("after a removal read %q (%v) with %d sealed, want nothing", key, err, secretsHeld(t, vault))
	}
}
