// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The custom-field types are one list on both sides of the wire. The server
// creates a field of a type in fieldcatalog.Types(); the admin screen offers the
// types in CF_TYPES. A type the server takes and the screen leaves out can only
// be created through the API. A type the screen offers and the server refuses
// is a create that fails every time.
//
// Both sides are read: the Go list from the port every backend consumer
// follows, the TypeScript array from the file the screen reads. It fails in
// both directions, and on order, since the picker shows them in that order.

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const frontendCustomFieldTypes = "../frontend/src/lib/customfields.logic.ts"

var tsTypeEntry = regexp.MustCompile(`["']([a-z]+)["']`)

func TestTheFrontendCustomFieldTypesMatchTheGoList(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendCustomFieldTypes)
	if err != nil {
		t.Fatalf("reading the frontend type list: %v", err)
	}
	const marker = "CF_TYPES = ["
	start := indexAfter(string(source), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares %s as an array literal; this gate reads a shape that is gone",
			frontendCustomFieldTypes, marker)
	}
	end := indexAfter(string(source)[start:], "]")
	if end < 0 {
		t.Fatalf("%s's type literal is unterminated", frontendCustomFieldTypes)
	}
	literal := tsHostComment.ReplaceAllString(string(source)[start:start+end], " ")
	var inTS []string
	for _, m := range tsTypeEntry.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no types parsed out of the frontend array; a gate that reads nothing agrees with everything")
	}
	if inGo := fieldcatalog.Types(); !slices.Equal(inGo, inTS) {
		t.Errorf("the custom-field type lists have drifted.\n  %s CF_TYPES: %v\n  fieldcatalog.Types(): %v\n"+
			"Change both in one commit.", frontendCustomFieldTypes, inTS, inGo)
	}
}
