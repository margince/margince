// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag tools' record_type enum, held against the store's own vocabulary.
// The agents module once carried its own copy of this list and it drifted:
// the store admitted project tags and the tool schema could not name one, so
// the web app could do what the assistant had to refuse. The schema now
// derives from the seam; this gate is what fails if anyone reintroduces a
// literal.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/collections"
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
