// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"errors"
	"strings"
	"testing"
)

func TestTheAuthorPairIsTheImportersToStateAndNobodyElses(t *testing.T) {
	ptr := func(s string) *string { return &s }
	hubspot := ptr("mirror:hubspot")

	for _, c := range []struct {
		name         string
		hasID        bool
		authorName   *string
		sourceSystem *string
		importer     bool
		wantField    string
		wantCode     string
	}{
		{
			name: "a client naming a seat", hasID: true, sourceSystem: hubspot,
			wantField: "source_author_id", wantCode: "reserved_source_author",
		},
		{
			name: "a client naming a name", authorName: ptr("Anna"), sourceSystem: hubspot,
			wantField: "source_author_name", wantCode: "reserved_source_author",
		},
		{
			name: "a blank name names nobody", authorName: ptr("  "), sourceSystem: hubspot, importer: true,
			wantField: "source_author_name", wantCode: "source_author_name_invalid",
		},
		{
			name: "a name past the bound", authorName: ptr(strings.Repeat("é", SourceAuthorNameMax+1)),
			sourceSystem: hubspot, importer: true,
			wantField: "source_author_name", wantCode: "source_author_name_invalid",
		},
		{
			name: "an author on a record from nowhere", hasID: true, importer: true,
			wantField: "source_author_id", wantCode: "source_author_needs_a_source",
		},
		{
			name: "an author on an empty source", authorName: ptr("Anna"), sourceSystem: ptr(" "), importer: true,
			wantField: "source_author_name", wantCode: "source_author_needs_a_source",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := AdmitSourceAuthor(AuthorClaim{HasAuthorID: c.hasID, AuthorName: c.authorName, SourceSystem: c.sourceSystem}, c.importer)
			var refused *AuthorError
			if !errors.As(err, &refused) {
				t.Fatalf("want an AuthorError, got %v", err)
			}
			if refused.Field != c.wantField || refused.Code != c.wantCode {
				t.Errorf("refused (%s, %s), want (%s, %s)", refused.Field, refused.Code, c.wantField, c.wantCode)
			}
		})
	}
}

func TestTheImporterMayStateEitherSpellingOrBoth(t *testing.T) {
	ptr := func(s string) *string { return &s }
	hubspot := ptr("mirror:hubspot")

	for _, c := range []struct {
		name       string
		hasID      bool
		authorName *string
		wantName   string
	}{
		{name: "a seat", hasID: true},
		{name: "a name, trimmed", authorName: ptr("  Anna Müller "), wantName: "Anna Müller"},
		{name: "both", hasID: true, authorName: ptr("Anna Müller"), wantName: "Anna Müller"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := AdmitSourceAuthor(AuthorClaim{HasAuthorID: c.hasID, AuthorName: c.authorName, SourceSystem: hubspot}, true)
			if err != nil {
				t.Fatalf("admitted importer refused: %v", err)
			}
			if got != c.wantName {
				t.Errorf("name = %q, want %q", got, c.wantName)
			}
		})
	}
}

// No author at all is the ordinary create, and it asks nothing of the caller.
func TestNoAuthorIsNotAClaim(t *testing.T) {
	got, err := AdmitSourceAuthor(AuthorClaim{}, false)
	if err != nil || got != "" {
		t.Errorf("a create without an author was refused or given one: %v, %v", got, err)
	}
}
