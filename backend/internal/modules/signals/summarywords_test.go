// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestOnlyAReaderWhoChoseALanguageGetsTheSummaryReworded(t *testing.T) {
	words := SummaryWords(func(lang textlang.Lang, _ string, _ json.RawMessage) (string, bool) {
		return "worded in " + string(lang), true
	})
	refusing := SummaryWords(func(textlang.Lang, string, json.RawMessage) (string, bool) { return "", false })
	reading := func(locale string) context.Context {
		return principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Locale: locale})
	}
	parts := json.RawMessage(`{"days":14}`)
	for name, c := range map[string]struct {
		words  SummaryWords
		ctx    context.Context
		params json.RawMessage
		want   string
	}{
		"a reader who chose German":            {words, reading("de"), parts, "worded in de"},
		"a reader who chose no language":       {words, reading(""), parts, "stored"},
		"a reader whose choice is not shipped": {words, reading("fr"), parts, "stored"},
		"no reader at all":                     {words, context.Background(), parts, "stored"},
		"a row that kept no parts":             {words, reading("de"), nil, "stored"},
		"no words wired":                       {nil, reading("de"), parts, "stored"},
		"words that refuse the parts":          {refusing, reading("de"), parts, "stored"},
	} {
		if got := c.words.ForReader(c.ctx, "ghosted_thread", c.params, "stored"); got != c.want {
			t.Errorf("%s reads %q, want %q", name, got, c.want)
		}
	}
}
