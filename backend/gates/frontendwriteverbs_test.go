// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser hides a moment verb a seat may not take by asking whether the
// verb writes an activity. The server's `writesAnActivity` answers the same
// question when it withholds the verb. The frontend table is a declared mirror
// of that map, and this compares the two both ways, value by value.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendWriteVerbs = "../frontend/src/lib/writeverbs.ts"

// tsWriteVerbEntry is one whole `kind: true` entry, bare or quoted. Each entry
// must match in full, so an entry this cannot read fails rather than vanishes.
var tsWriteVerbEntry = regexp.MustCompile(`^["']?([a-z_]+)["']?\s*:\s*(true|false)$`)

func TestTheFrontendMomentVerbTableMirrorsTheServers(t *testing.T) {
	t.Parallel()
	inTS := frontendMomentVerbWrites(t)
	inGo := serverMomentVerbWrites(t)
	for kind, writes := range inGo {
		got, present := inTS[kind]
		switch {
		case !present:
			t.Errorf("%s: the server classifies %q (writes=%v) and MOMENT_VERB_WRITES does not, "+
				"so the browser treats it as a write and hides it from a seat the server offers it to",
				frontendWriteVerbs, kind, writes)
		case got != writes:
			t.Errorf("%s: %q writes=%v in the browser and writes=%v on the server; one verb needs one answer",
				frontendWriteVerbs, kind, got, writes)
		}
	}
	for kind := range inTS {
		if _, present := inGo[kind]; !present {
			t.Errorf("%s classifies %q, which the server's writesAnActivity does not: a stale entry",
				frontendWriteVerbs, kind)
		}
	}
}

func serverMomentVerbWrites(t *testing.T) map[string]bool {
	t.Helper()
	for _, vocabulary := range offeredVerbVocabularies {
		if vocabulary.mapName == "writesAnActivity" {
			classified, _ := vocabulary.classification(t)
			return classified
		}
	}
	t.Fatal("offeredVerbVocabularies no longer names writesAnActivity, the map the frontend table mirrors")
	return nil
}

// frontendMomentVerbWrites reads every entry of the MOMENT_VERB_WRITES literal.
func frontendMomentVerbWrites(t *testing.T) map[string]bool {
	t.Helper()
	source, err := gatekit.ReadFile(frontendWriteVerbs)
	if err != nil {
		t.Fatalf("reading the frontend write-verb table: %v", err)
	}
	const opening = "MOMENT_VERB_WRITES = {"
	start := indexAfter(string(source), opening)
	if start < 0 {
		t.Fatalf("%s no longer declares %q; this gate reads the table from that literal", frontendWriteVerbs, opening)
	}
	end := strings.Index(string(source)[start:], "} as const")
	if end < 0 {
		t.Fatalf("%s: the MOMENT_VERB_WRITES literal is unterminated", frontendWriteVerbs)
	}
	body := gatekit.TSWithoutComments(string(source)[start : start+end])
	entries := map[string]bool{}
	for entry := range strings.SplitSeq(body, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		match := tsWriteVerbEntry.FindStringSubmatch(entry)
		if match == nil {
			t.Fatalf("%s: MOMENT_VERB_WRITES entry %q is not `kind: true|false`; this gate reads only that shape",
				frontendWriteVerbs, entry)
		}
		entries[match[1]] = match[2] == "true"
	}
	if len(entries) == 0 {
		t.Fatalf("no entries read from %s: a gate that reads nothing agrees with everything", frontendWriteVerbs)
	}
	return entries
}
