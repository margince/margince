// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The embedding lanes may only name entity types the embedding TABLE accepts,
// and the two maps that declare them must name the same set.
//
// Both obligations were stated in comments and held by nothing. An entity
// listed in the lanes but absent from `embedding.entity_type`'s CHECK is not a
// degraded lane, it is a write the database refuses — and the refusal surfaces
// far from here, as a job that fails forever: embed_drift_sweep re-selects
// whatever has no embedding row, so such a type is permanently pending, and
// each pass spends a real embedding call on it, is refused, and abandons the
// rest of that pass.
//
// Asserted transitively rather than by reading the migrations. enumBindings in
// gates/enumsync_test.go already pins `embedding.entity_type`'s CHECK to
// datasource.EntityType and fails on any inequality, so proving the lanes are
// a SUBSET of that Go vocabulary proves they are admitted by the CHECK, with
// no second parser of the schema to keep in step. Widening the CHECK alone
// cannot satisfy this: the other gate would fail on the same change.
func TestEveryEmbeddableEntityIsOneTheEmbeddingTableAccepts(t *testing.T) {
	t.Parallel()

	admitted := make(map[string]bool, len(datasource.EntityTypes()))
	for _, e := range datasource.EntityTypes() {
		admitted[string(e)] = true
	}
	if len(admitted) == 0 {
		t.Fatal("datasource declares no entity type, so this gate is asserting nothing")
	}

	for _, entityType := range slices.Sorted(maps.Keys(pendingSources)) {
		if !admitted[entityType] {
			t.Errorf("pendingSources names %q, which datasource.EntityTypes() does not — "+
				"embedding.entity_type's CHECK is pinned to that vocabulary, so every upsert for "+
				"this type is refused by the database and embed_drift_sweep retries it forever",
				entityType)
		}
	}
	for _, entityType := range slices.Sorted(maps.Keys(embedText)) {
		if !admitted[entityType] {
			t.Errorf("embedText names %q, which datasource.EntityTypes() does not — see above", entityType)
		}
	}
}

// The per-id lane and the set-form lane are two views of one vocabulary, and
// both files say so in prose. A type in one but not the other means the
// backlog count and the live indexer disagree about what is embeddable: an
// entity counted as pending that nothing embeds stays pending forever, and one
// embedded but never counted makes the re-embed cost preview too cheap.
func TestTheTwoEmbeddingLanesNameTheSameEntities(t *testing.T) {
	t.Parallel()

	pending := slices.Sorted(maps.Keys(pendingSources))
	perID := slices.Sorted(maps.Keys(embedText))
	if !slices.Equal(pending, perID) {
		t.Errorf("pendingSources and embedText have diverged:\n  pendingSources: %v\n  embedText:      %v",
			pending, perID)
	}
}
