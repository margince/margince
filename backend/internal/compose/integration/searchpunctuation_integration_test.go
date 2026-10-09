// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A last word that is only punctuation lexes to nothing. It is a stray key in
// the search box, so it must never fail the request: it adds no prefix, and the
// words before it still search.
func TestSearchIgnoresALastWordMadeOfPunctuation(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	created, err := e.Contacts.CreateContact(admin, contacts.CreateContactInput{FullName: "Acme Rivera", Source: "manual"})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	store := search.NewStore(e.DB())

	for _, query := range []string{"%", "(", "Acme (", "Acme %", "Acme ..."} {
		page, err := store.Search(admin, search.Input{Query: query})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		finishedWords := query != "%" && query != "("
		if got := hasHit(page, ids.UUID(created.Id)); got != finishedWords {
			t.Errorf("search %q found the contact = %v, want %v", query, got, finishedWords)
		}
	}
}
