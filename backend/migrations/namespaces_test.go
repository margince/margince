// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations_test

// Loading the embedded migration namespaces, once, for every test in this
// package that reads or replays them. Core comes without its baseline: these
// tests walk the history version by version.

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func namespaces(t *testing.T) (core, custom dbmigrate.Namespace) {
	t.Helper()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading the core namespace: %v", err)
	}
	custom, err = migrations.Custom()
	if err != nil {
		t.Fatalf("loading the custom namespace: %v", err)
	}
	return core, custom
}
