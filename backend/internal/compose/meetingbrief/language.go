// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The language the brief is written in.
//
// It is the READER's, not the record's: a brief about a German conversation
// read by an English-speaking rep should be English, and the same brief read by
// a German rep should be German. The client sends what it is rendering in, on
// Accept-Language, because that is the one place that knows.
//
// It travels on the context rather than through six call signatures: only the
// model prompt consumes it, the deterministic floor is English templates and
// ignores it, and threading a display preference through the assembly would put
// it in front of every reader of that code for the benefit of one line.

import (
	"context"
	"net/http"
	"strings"
)

// The tags this product renders in. An Accept-Language naming anything else
// gets the default rather than a brief in a language nothing else on the screen
// speaks: a German brief inside an English page is worse than an English one.
var supportedLanguages = map[string]string{"en": "en", "de": "de", "vi": "vi"}

// defaultLanguage is what an absent or unrecognised header means.
const defaultLanguage = "en"

type languageKey struct{}

// WithReaderLanguage carries the reader's language for this request.
func WithReaderLanguage(ctx context.Context, language string) context.Context {
	return context.WithValue(ctx, languageKey{}, language)
}

// ReaderLanguage returns the language this brief should be written in.
func ReaderLanguage(ctx context.Context) string {
	if language, ok := ctx.Value(languageKey{}).(string); ok && language != "" {
		return language
	}
	return defaultLanguage
}

// languageOf reads Accept-Language, keeping only a tag this product renders.
//
// It reads the FIRST tag and does not weigh q-values: the client sends exactly
// what it is rendering in, one tag, so parsing a preference list would be
// machinery for a case this product's own client never produces. A browser's
// own multi-tag header still resolves — its first tag is its preference.
func languageOf(r *http.Request) string {
	header := r.Header.Get("Accept-Language")
	if header == "" {
		return defaultLanguage
	}
	first, _, _ := strings.Cut(header, ",")
	first, _, _ = strings.Cut(strings.TrimSpace(first), ";")
	// A region subtag names the same language: de-AT reads German.
	base, _, _ := strings.Cut(strings.TrimSpace(first), "-")
	if language, ok := supportedLanguages[strings.ToLower(base)]; ok {
		return language
	}
	return defaultLanguage
}
