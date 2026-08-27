// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag tools' record_type enum, held to the collections vocabulary: this
// gate fails if the schema stops deriving from the seam (a reintroduced
// literal). Whether that vocabulary is itself COMPLETE against the taggable
// CHECK is the integration lane's claim
// (TestEveryEnumOverTheRecordVocabularyMatchesTheCheckConstraint), not this
// one's — both sides of the comparison here read TaggableEntityTypes.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
)

func TestTagToolSchemasAdmitEveryTaggableRecordType(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	want := collections.TaggableEntityTypes()
	checked := 0
	for _, spec := range registry.Specs() {
		if spec.Name != "apply_tag" && spec.Name != "remove_tag" {
			continue
		}
		checked++
		var shape struct {
			Properties struct {
				RecordType struct {
					Enum []string `json:"enum"`
				} `json:"record_type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &shape); err != nil {
			t.Fatalf("%s: its input schema does not decode: %v", spec.Name, err)
		}
		if !reflect.DeepEqual(shape.Properties.RecordType.Enum, want) {
			t.Errorf("%s admits %v; the store's vocabulary is %v — the schema must derive from the seam, not a literal",
				spec.Name, shape.Properties.RecordType.Enum, want)
		}
	}
	if checked != 2 {
		t.Fatalf("found %d of the 2 tag tools on the composed surface; this gate no longer reaches them", checked)
	}
}
