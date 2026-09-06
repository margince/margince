// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// How long an in-flight authentication may be held waiting on somebody else's
// server.
//
// Two outbound metadata reads gate one: the consent metadata a consent page
// waits on, and the signing keys a sign-in waits on. Both are small-JSON reads
// from an identity provider. They differed by 6x — 5s and 30s — with no reason
// recorded for the difference, which is how a third arrives at a fourth number.
//
// The keys are the tighter case, not the looser one, and the caching is why:
// refreshes coalesce, so one slow response holds EVERY sign-in arriving during
// it rather than one. A bound set for patience with a single caller is an
// outage for all of them.
//
// Read from the source because the two constants live in modules that cannot
// import each other. Equality rather than a ceiling: two numbers that may drift
// apart within a bound drift apart, and the reason they should not is the same
// on both sides.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// authWait names each bound and where it is declared.
var authWait = map[string]struct{ file, constant string }{
	"the consent-metadata fetch": {"backend/internal/modules/identity/oauth_cimd.go", "cimdFetchTimeout"},
	"the signing-key fetch":      {"backend/internal/compose/oidcverify.go", "jwksFetchTimeout"},
}

func TestEveryFetchAnAuthenticationWaitsOnHasTheSameBound(t *testing.T) {
	t.Parallel()

	found := map[string]string{}
	for name, where := range authWait {
		raw, err := os.ReadFile(filepath.Join(repoRoot, where.file))
		if err != nil {
			t.Fatalf("reading %s: %v", where.file, err)
		}
		pattern := regexp.MustCompile(regexp.QuoteMeta(where.constant) + `\s*=\s*([^\n/]+)`)
		match := pattern.FindStringSubmatch(string(raw))
		if match == nil {
			t.Fatalf("%s no longer declares %s — this gate is comparing a bound that is gone, "+
				"which is how a census comes to certify nothing", where.file, where.constant)
		}
		found[name] = trimSpace(match[1])
	}

	var first, firstName string
	for name, value := range found {
		if first == "" {
			first, firstName = value, name
			continue
		}
		if value != first {
			t.Errorf("%s waits %s and %s waits %s. Both are small-JSON reads an authentication is "+
				"held on, and the signing-key one is the SHARED wait — its refreshes coalesce, so a "+
				"slow provider holds every sign-in arriving during it. If one of these genuinely "+
				"needs a different number, the reason belongs at both sites and this gate should "+
				"say what it is.", firstName, first, name, value)
		}
	}
}

// trimSpace drops surrounding whitespace and a trailing comma.
func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == ',') {
		s = s[:len(s)-1]
	}
	return s
}
