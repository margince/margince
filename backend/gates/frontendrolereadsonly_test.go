// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A refused write names its cause in one sentence on every surface. The causes
// are a read seat, a role that only reads, and a role without one action. The record
// pages word each from the frontend catalog, and a move the server blocks
// carries the server's own words. This gate holds each pair equal in every
// language either side ships, so a refused button never says two things.

import (
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

// refusalPairs maps each frontend catalog key to the server's phrase for it.
var refusalPairs = map[string]langcopy.Phrase{
	"settings.seatRefused": momentaction.SeatReadsOnly(),
	"record.roleReadsOnly": momentaction.RoleReadsOnly(),
	"record.actionRefused": momentaction.ActionRefused(),
}

func TestTheServersRefusalsAreTheRecordPagesSentences(t *testing.T) {
	t.Parallel()
	langcopytest.Census(t, refusalPairs)
	languages := refusalPhraseLanguages()
	if catalogs := refusalCatalogLanguages(t); !slices.Equal(languages, catalogs) {
		t.Fatalf("the server writes refusals in %v and the frontend ships %v. Add the missing language on the other side", languages, catalogs)
	}
	for _, language := range languages {
		source, err := gatekit.ReadFile("../frontend/src/i18n/" + language + ".ts")
		if err != nil {
			t.Fatalf("reading the %s copy: %v", language, err)
		}
		for key, server := range refusalPairs {
			got := refusalCatalogEntry(t, source, language, key)
			want := langcopy.For(language).Say(server)
			if got != want {
				t.Errorf("%s: the record pages say %q for %s and a blocked move says %q. Change both together", language, got, key, want)
			}
		}
	}
}

// refusalPhraseLanguages reads the shipped languages off langcopy.Phrase's fields,
// so a language added there is a language this gate reads.
func refusalPhraseLanguages() []string {
	shape := reflect.TypeFor[langcopy.Phrase]()
	languages := make([]string, 0, shape.NumField())
	for i := range shape.NumField() {
		languages = append(languages, strings.ToLower(shape.Field(i).Name))
	}
	slices.Sort(languages)
	return languages
}

// refusalCatalogLanguages lists the frontend's language catalogs, one two-letter file each.
func refusalCatalogLanguages(t *testing.T) []string {
	t.Helper()
	entries, err := gatekit.ReadDir("../frontend/src/i18n")
	if err != nil {
		t.Fatalf("listing the frontend catalogs: %v", err)
	}
	var catalogs []string
	for _, entry := range entries {
		if refusalCatalogFile.MatchString(entry.Name()) {
			catalogs = append(catalogs, strings.TrimSuffix(entry.Name(), ".ts"))
		}
	}
	if len(catalogs) == 0 {
		t.Fatal("frontend/src/i18n holds no language catalog; the gate reads nothing")
	}
	slices.Sort(catalogs)
	return catalogs
}

var refusalCatalogFile = regexp.MustCompile(`^[a-z]{2}\.ts$`)

func refusalCatalogEntry(t *testing.T, source []byte, language, key string) string {
	t.Helper()
	entry := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*("(?:[^"\\]|\\.)*")`)
	match := entry.FindSubmatch(source)
	if match == nil {
		t.Fatalf("frontend/src/i18n/%s.ts has no %s; the gate reads nothing", language, key)
	}
	got, err := strconv.Unquote(string(match[1]))
	if err != nil {
		t.Fatalf("%s in %s.ts is not one string literal: %v", key, language, err)
	}
	return got
}
