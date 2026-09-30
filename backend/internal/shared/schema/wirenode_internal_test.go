// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package schema

import (
	"reflect"
	"slices"
	"testing"
)

// Node is what ValidateJSON decodes a schema into and wireNode is what Must
// writes, so a keyword one carries and the other lacks is enforced by the
// validator and never sent to the model — or sent and never read back.
func TestTheWireNodeCarriesEveryNodeKeyword(t *testing.T) {
	node, wire := jsonTags(reflect.TypeFor[Node]()), jsonTags(reflect.TypeFor[wireNode]())
	if len(node) == 0 {
		t.Fatal("Node declares no tagged field, so nothing was compared")
	}
	if !slices.Equal(node, wire) {
		t.Errorf("Node's keywords are %v and wireNode's are %v — the same list, in the same order", node, wire)
	}
}

// jsonTags is each exported field's JSON tag, in declaration order.
func jsonTags(t reflect.Type) []string {
	var tags []string
	for field := range t.Fields() {
		if field.IsExported() {
			tags = append(tags, field.Tag.Get("json"))
		}
	}
	return tags
}
