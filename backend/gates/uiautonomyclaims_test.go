// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A direct human instruction can authorise a send without another in-app
// approval. Static send tiers describe that path; unattended runs wait at
// auth.TierFor. This copy check catches promises of an extra confirmation.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// These phrases wrongly turn routine updates into an approval queue. Each
// shipped language needs its own patterns; phrasing cannot be derived.
var absoluteApprovalClaims = map[string][]*regexp.Regexp{
	"en": {
		regexp.MustCompile(`every (field )?change (waits|requires approval)`),
		regexp.MustCompile(`nothing is written until you (approve|accept)`),
		regexp.MustCompile(`agents only draft[,]? people send`),
		regexp.MustCompile(`granting the permission is the approval`),
	},
	"de": {
		regexp.MustCompile(`jede änderung (wartet|braucht eine freigabe)`),
		regexp.MustCompile(`vorher wird nichts geschrieben`),
		regexp.MustCompile(`nur menschen (senden|verschicken)`),
		regexp.MustCompile(`mit der berechtigung ist die freigabe bereits erteilt`),
	},
	"vi": {
		regexp.MustCompile(`mọi thay đổi.{0,30}chờ.{0,20}duyệt`),
		regexp.MustCompile(`chưa duyệt[,]? chưa lưu`),
		regexp.MustCompile(`chỉ bạn được bấm gửi`),
		regexp.MustCompile(`cấp quyền chính là phê duyệt`),
	},
}

// shippedLocales names every locale the app ships, read from the directory
// rather than from the map below.
//
// This is the half that keeps the gate from failing short. The claim patterns
// are per-LANGUAGE and cannot be derived — somebody has to write down how the
// promise is spelled in French before French can be checked — so a hand-written
// map is unavoidable here. What is avoidable is that map going quietly short: a
// new locale file with no patterns beside it would be a whole language this gate
// reads nothing of, and it would report PASS. So the directory decides who must
// be covered, and an uncovered locale FAILS rather than being skipped.
func shippedLocales(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the locale directory: %v", err)
	}
	// The machinery that lives beside the catalogs, named as the exclusions they
	// are rather than filtered by the SHAPE of a locale code. A length test for
	// "two characters" silently drops pt-BR and zh-Hans: a whole language read
	// by nothing, while the gate reports PASS. That is the same
	// under-recognition this file exists to refuse, one level up.
	machinery := map[string]bool{
		"index.ts": true, "publiclocale.ts": true, "locale.ts": true,
		// A register of catalogue KEYS, not a catalogue: it ships no copy a
		// user reads, and asking it for a phrasing of "nothing sends without
		// your approval" would be asking a list of identifiers to speak.
		"plural-debt.ts":       true,
		"catalogs.ts":          true, // Loads catalogs; contains no user copy.
		"bootstrapcopy_gen.ts": true, // Generated projection of en.ts, which this gate reads.
	}

	var locales []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".ts") || strings.Contains(name, ".test.") || machinery[name] {
			continue
		}
		locales = append(locales, strings.TrimSuffix(name, ".ts"))
	}
	if len(locales) == 0 {
		t.Fatal("no locale files found — this gate is reading a tree shape that is gone")
	}
	return locales
}

// shippedCopy drops whole-line comments, leaving what a user can actually read.
//
// A locale file explains its own copy, and this gate went off on a comment
// describing the very claim it had just removed. Left alone, that teaches the
// next author to weaken the patterns to quiet it — trading a real matcher for a
// tidy log — so the fix belongs on the CORPUS instead: judge shipped strings,
// never the prose about them.
//
// Only lines that BEGIN with the marker go, so a "https://…" inside a string
// keeps its slashes. A trailing comment after a string survives, and is
// harmless: it is one line of prose on a line that also carries real copy.
func shippedCopy(doc string) string {
	var kept []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func TestNoLocaleRequiresApprovalForEveryUpdate(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot, "frontend", "src", "i18n")
	for _, locale := range shippedLocales(t, dir) {
		claims, covered := absoluteApprovalClaims[locale]
		if !covered {
			t.Errorf("frontend/src/i18n/%s.ts ships to users but this gate knows no %s phrasing of the incorrect autonomy claims, so it reads none of that language. Add the shapes to absoluteApprovalClaims — an uncovered locale is where this census goes short", locale, locale)
			continue
		}

		raw, err := gatekit.ReadFile(filepath.Join(dir, locale+".ts"))
		if err != nil {
			t.Fatalf("reading the %s locale: %v", locale, err)
		}
		body := strings.ToLower(shippedCopy(string(raw)))

		for _, claim := range claims {
			if loc := claim.FindStringIndex(body); loc != nil {
				t.Errorf("frontend/src/i18n/%s.ts promises %q, but routine updates run automatically and a direct instruction authorises a send", locale, strings.TrimSpace(body[loc[0]:loc[1]]))
			}
		}
	}
}
