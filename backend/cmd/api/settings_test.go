// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
)

// apiSettings are what the api's registry holds.
func apiSettings(t *testing.T) []config.Any {
	t.Helper()
	reg, err := compose.ProgramRegistry(config.RoleAPI)
	if err != nil {
		t.Fatal(err)
	}
	return reg.All()
}

// secretByEnv marks every variable this role reads.
func secretByEnv(t *testing.T) map[string]bool {
	t.Helper()
	secret := map[string]bool{}
	for _, s := range apiSettings(t) {
		secret[s.Env()] = s.Secret()
	}
	return secret
}

func TestTheCredentialsThisRoleReadsAreMarkedSecret(t *testing.T) {
	t.Parallel()
	// Named explicitly because nothing mechanical can tell a path from a
	// password; this catches a credential wrongly declared public.
	secret := secretByEnv(t)
	for _, name := range []string{
		"MARGINCE_DSN", "MARGINCE_SCHEMA_DSN", "MARGINCE_REDIS_PASSWORD",
		"MARGINCE_GMAIL_CLIENT_SECRET", "MARGINCE_GMAIL_PUSH_TOKEN",
		"MARGINCE_GRAPH_CLIENT_SECRET", "MARGINCE_GRAPH_PUSH_TOKEN",
		"MARGINCE_CONNECTOR_STATE_KEY", "MARGINCE_WEBHOOK_KEY", "MARGINCE_METRICS_TOKEN",
		"MARGINCE_KEYVAULT_ROOT_KEY", "MARGINCE_LICENSE",
		"MARGINCE_BLOBSTORE_ACCESS_KEY", "MARGINCE_BLOBSTORE_SECRET_KEY",
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY",
	} {
		if !secret[name] {
			t.Errorf("%s authenticates something and is not marked Secret", name)
		}
	}
	// A value that authenticates nobody stays visible, because an operator
	// debugging a boot wants to read it.
	for _, name := range []string{
		"MARGINCE_LOG_LEVEL", "MARGINCE_PUBLIC_BASE_URL", "MARGINCE_METRICS_ACCESS",
		"MARGINCE_TRUSTED_PROXIES", "MARGINCE_VAT_CHECK_BASE_URL", "MARGINCE_GEOCODE_BASE_URL", "MARGINCE_GRAPH_TENANT",
	} {
		if secret[name] {
			t.Errorf("%s is marked Secret but authenticates nothing; hiding it only makes a boot harder to debug", name)
		}
	}
}

func TestAMisspelledVariableReachesTheBootReport(t *testing.T) {
	t.Parallel()
	// Assembled rather than written whole: the documentation gate reads a
	// quoted MARGINCE_* literal as a variable somebody must document.
	const misspelled = "MARGINCE_" + "REDDIS"
	_, loaded, err := loadAPIConfig([]string{"--dsn", "postgres://user@localhost:5432/db"}, []string{
		misspelled + "=localhost:6379",
		"MARGINCE_DSN=postgres://user@localhost:5432/db",
		// Read by a package this role wires, and by the worker only.
		"MARGINCE_KEYVAULT_ROOT_KEY=stands-in-for-a-key",
		"MARGINCE_OBSERVE_ADDR=127.0.0.1:9101",
	})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !slices.Contains(loaded.Warnings(), misspelled+" (did you mean "+ps.Redis.Env()+"?)") {
		t.Errorf("warnings = %v; the misspelling never reached the boot report with the name it meant", loaded.Warnings())
	}
	for _, read := range []string{"MARGINCE_DSN", "MARGINCE_KEYVAULT_ROOT_KEY", "MARGINCE_OBSERVE_ADDR"} {
		if slices.ContainsFunc(loaded.Warnings(), func(w string) bool { return strings.HasPrefix(w, read+" ") || w == read }) {
			t.Errorf("%s is read by a program and was reported as unread", read)
		}
	}
}
