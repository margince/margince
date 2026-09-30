// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The inbox line the sink writes when a captured lead collides with one it
// already holds.
//
// An approval's summary is shared-record text: stored once and read by every
// seat, so it follows the installation's base language, resolved when the
// merge is staged. Rows already stored keep the words they were written in.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// summaryCopy is one language's set. Every field is required, and the census
// (summarycopy_test.go) refuses a partial set.
type summaryCopy struct {
	// duplicateLead names the colliding capture. %s/%s is its natural key,
	// source system then source id; both are identifiers, untranslated.
	duplicateLead string
}

// summaryByLang is the census, keyed by textlang.Lang so the test can walk
// textlang.Shipped and ask this map directly.
var summaryByLang = map[textlang.Lang]summaryCopy{
	textlang.English: {
		duplicateLead: "Captured %s/%s duplicates an existing lead",
	},
	textlang.German: {
		duplicateLead: "Der erfasste Datensatz %s/%s ist ein Duplikat eines bestehenden Leads",
	},
	textlang.Vietnamese: {
		duplicateLead: "Bản ghi thu thập %s/%s trùng với một lead hiện có",
	},
}

// summaryIn answers the set for the installation's base language, and English
// for a language this table has not learned: the language comes off a settings
// row, and a sentence in English beats no sentence.
func summaryIn(ctx context.Context, language baselanguage.Resolver) summaryCopy {
	if said, ok := summaryByLang[language.Resolve(ctx)]; ok {
		return said
	}
	return summaryByLang[textlang.English]
}

// duplicateLeadSummary is the dedupe merge proposal's inbox line for the
// capture that collided.
func (s *Sink) duplicateLeadSummary(ctx context.Context, key connector.NaturalKey) string {
	return fmt.Sprintf(summaryIn(ctx, s.language).duplicateLead, key.SourceSystem, key.SourceID)
}

// WithBaseLanguage returns a copy that writes its staged summaries in the
// installation's base language. A sink built without one writes English.
func (s *Sink) WithBaseLanguage(language baselanguage.Resolver) *Sink {
	c := *s
	c.language = language
	return &c
}
