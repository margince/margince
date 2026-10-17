// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Provider.Search pages one record type. Every refusal below is decided
// before a module is asked, so a Provider with no modules wired answers them.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestSearchRefusesAQueryThatDoesNotNameExactlyOneType(t *testing.T) {
	for name, types := range map[string][]datasource.EntityType{
		"none": nil,
		"two":  {datasource.EntityContact, datasource.EntityCompany},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&Provider{}).Search(context.Background(), datasource.SearchQuery{Text: "x", EntityTypes: types})
			if err == nil {
				t.Fatalf("a query naming %d types was answered — Search pages one type, "+
					"and a silent sweep would serve records nobody asked for", len(types))
			}
		})
	}
}

func TestSearchRefusesATypeNoModuleSearches(t *testing.T) {
	for _, et := range []datasource.EntityType{datasource.EntityRelationship, "nonsense"} {
		_, err := (&Provider{}).Search(context.Background(), datasource.SearchQuery{EntityTypes: []datasource.EntityType{et}})
		if _, ok := errors.AsType[*datasource.UnsupportedEntityError](err); !ok {
			t.Errorf("searching %q answered %v, want the unsupported-entity refusal", et, err)
		}
	}
}

// A cursor carries the type it was minted for. Presented with another type it
// would resume a keyset from a different table, so it is refused rather than
// read.
func TestSearchRefusesACursorMintedForAnotherType(t *testing.T) {
	token, err := storekit.EncodeSweepCursor(storekit.SweepCursor{Stream: string(datasource.EntityPartner), Inner: "x"})
	if err != nil {
		t.Fatalf("minting the cursor: %v", err)
	}
	_, err = (&Provider{}).Search(context.Background(), datasource.SearchQuery{
		EntityTypes: []datasource.EntityType{datasource.EntityContact}, Cursor: token,
	})
	if _, ok := errors.AsType[*storekit.MalformedCursorError](err); !ok {
		t.Fatalf("a partner cursor presented on a contact search answered %v, want the malformed-cursor refusal", err)
	}
}
