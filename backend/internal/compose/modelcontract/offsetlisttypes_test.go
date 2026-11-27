// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package modelcontract_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// list_records tells a model which lists take an offset. The search seam
// decides it from compose.OffsetListTypes, so the two must name the same types.
func TestTheListRecordsOffsetCopyNamesTheTypesSearchPagesByOffset(t *testing.T) {
	specs := servedSpecs()
	at := slices.IndexFunc(specs, func(spec mcp.ToolSpec) bool { return spec.Name == "list_records" })
	if at < 0 {
		t.Fatal("the registry serves no list_records tool")
	}
	var input struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(specs[at].InputSchema, &input); err != nil {
		t.Fatalf("reading the list_records input schema: %v", err)
	}
	offsetCopy := input.Properties["offset"].Description
	sentence, found := strings.CutSuffix(offsetCopy, " lists only.")
	if !found {
		t.Fatalf("the offset copy %q does not end by naming its lists; teach this test its wording", offsetCopy)
	}
	named := strings.FieldsFunc(strings.ToLower(sentence[strings.LastIndex(sentence, ". ")+2:]), func(r rune) bool {
		return r == ',' || r == ' '
	})
	named = slices.DeleteFunc(named, func(word string) bool { return word == "and" })
	var paged []string
	for _, et := range compose.OffsetListTypes() {
		paged = append(paged, string(et))
	}
	slices.Sort(named)
	slices.Sort(paged)
	if !slices.Equal(named, paged) {
		t.Errorf("list_records says %v take an offset, and search pages %v by offset", named, paged)
	}
}
