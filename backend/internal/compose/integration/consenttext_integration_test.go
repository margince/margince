// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation's own wording, published by the bootstrap that seeds the
// purpose catalog beside it.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// TestBootstrapPublishesTheInstallationsWording is the wiring check.
//
// A table nothing writes is a table that answers "which wording was this
// contact shown" with silence — and silence reads as "no wording was
// published", which is an answer and a wrong one. The previous item in this
// plan shipped exactly that shape: a writer with no production caller, tests
// green, found only in review.
func TestBootstrapPublishesTheInstallationsWording(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var published int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_text_version WHERE published_at IS NOT NULL`).Scan(&published); err != nil {
		t.Fatal(err)
	}
	if published < 2 {
		t.Fatalf("bootstrap published %d wordings, want at least the confirm-details and "+
			"double-opt-in templates: nothing calls the publisher, so a proof row naming a "+
			"version would resolve to nothing", published)
	}

	// Every published row carries the hash that identifies its text — the
	// column a later reader compares a proof row's own copy against.
	var unhashed int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM consent_text_version
		 WHERE published_at IS NOT NULL AND (content_hash IS NULL OR content_hash = '')`).Scan(&unhashed); err != nil {
		t.Fatal(err)
	}
	if unhashed != 0 {
		t.Errorf("%d published wording(s) carry no content hash, so nothing can prove a proof "+
			"row's copy still matches what was published", unhashed)
	}
}
