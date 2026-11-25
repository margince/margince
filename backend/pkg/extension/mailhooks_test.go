// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A field added to MailView is content a hook reads, so the operator must be
// told of it too. Activity and Meeting say nothing about what the mail holds,
// and Lookup reads the unit's own tables.
func TestMailViewReadsNamesEveryContentField(t *testing.T) {
	notContent := []string{"Activity", "Meeting", "Lookup"}
	view := reflect.TypeFor[MailView]()
	var content []string
	for field := range view.Fields() {
		if name := field.Name; !slices.Contains(notContent, name) {
			content = append(content, strings.ToLower(name))
		}
	}
	if !slices.Equal(content, MailViewReads()) {
		t.Errorf("MailView carries %v, and MailViewReads names %v", content, MailViewReads())
	}
}
