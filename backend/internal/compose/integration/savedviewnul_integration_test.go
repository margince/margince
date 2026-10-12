// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A NUL inside a view's query reaches Postgres as a jsonb escape it cannot
// store. It is the caller's text, so it must read as an invalid value and not
// as a server fault.
func TestASavedViewQueryHoldingANULIsTheCallersInvalidValue(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep := e.As(e.Rep1, nil, collectionsPerms())

	_, err := store.CreateSavedView(rep, collections.CreateSavedViewInput{
		Resource: "deals", Name: "NUL in the query", Query: map[string]any{"a": "\x00"},
	})
	if err == nil {
		t.Fatal("a query holding a NUL was stored")
	}
	if !storekit.IsInvalidValueForType(err) {
		t.Fatalf("a NUL in the query = %v, want an invalid value the caller sent", err)
	}
}
