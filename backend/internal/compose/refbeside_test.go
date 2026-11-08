// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refBeside is the short ref a request shows for the record whose name or
// label is name, read off the request the way a model reads it. A scripted
// reply names a record by it, as a model's reply does; the request never
// shows the record's id.
func refBeside(t *testing.T, req model.Request, name string) string {
	t.Helper()
	shown := regexp.MustCompile(`"id":"([a-z]+[0-9]+)"[^{}]*?"(?:label|name|title)":"` + regexp.QuoteMeta(name) + `"`)
	var text strings.Builder
	for _, message := range req.Messages {
		text.WriteString(message.Content)
	}
	found := shown.FindStringSubmatch(text.String())
	if found == nil {
		t.Fatalf("the request shows no ref beside %q:\n%s", name, text.String())
		return ""
	}
	return found[1]
}
