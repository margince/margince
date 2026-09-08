// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// The record type is called company, and this is what stops the other word
// coming back.
//
// It never had to come back, because it never left: the product said company
// on every screen while the schema said organization, parts of the code said
// org, and the tool surface said account. A model asking for record_type
// "company" was refused by a registry that admitted only "organization", and a
// reader grepping for the company table found nothing. Nothing failed, because
// nothing was looking — which is why the fix is a test and not a sweep.
//
// WHAT THIS DOES NOT COVER, said rather than left to be assumed: the word
// account. It names this record type on a dozen tool verbs and forty interface
// strings, and it ALSO names a LinkedIn account, a channel Official Account,
// and the ordinary words accountable and accountability. Telling those apart is
// judgement per occurrence, so this test does not pretend to — a gate that
// covers most of a rule reads as covering all of it.
//
// The corpus comes from the index, so a new file is in scope automatically and
// an untracked scratch file is not.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// theOtherWord matches the record type's former names. `org` only where it is
// a whole identifier component: it is a substring of forget, Georgia, morgue
// and borgen, and a census that fired on those would be turned off.
var theOtherWord = regexp.MustCompile(
	`(?i)organi[sz]ation|(?:^|[^a-z0-9_.])orgs?(?:[^a-z0-9_]|$)`)

// notThisRecordType is the words that merely look like it: a meeting organizer,
// the ordinary adjective, the ordinary verb. Removed before the match rather
// than waived per site, because they are a property of English and not of any
// one file.
var notThisRecordType = regexp.MustCompile(
	`(?i)organi[sz]er|organi[sz]ational|organi[sz]e[sd]|organi[sz]ing|organi[sz]es`)

// hostname matches a .org TLD. There are 275 in this tree — a go.mod require,
// a disposable-email fixture list, schema.org in a comment — and in each one
// `org` sits between a dot and a slash or a quote exactly as an identifier
// component would.
var hostname = regexp.MustCompile(`[a-z0-9][a-z0-9.-]*\.orgs?\b`)

// exempt names a path and says why the word is right there. The reason is the
// point: a waiver nobody had to justify is how the second spelling survived
// review for a year.
var exempt = map[string]string{
	"backend/migrations/core": "shipped migrations are never edited — the SQL that built the " +
		"old names is the record of how the schema got here",
	"backend/migrations/testdata/rbac_baseline_era_defaults.json": "pinned byte for byte to the " +
		"baseline commit by rbacbaselineerafixture_test.go; it IS the matrix the server seeded then",
	"CHANGELOG.md": "entries say what they said when they were written",
	"sbom-schemas": "a vendored SPDX schema, not ours to rename",
	".github/workflows/release.yml": "a GitHub organisation — the account a repository belongs " +
		"to, not this record type",

	// Microsoft's authority alias: `organizations` is a literal path segment at
	// login.microsoftonline.com beside `common` and `consumers`, and renaming
	// it stops sign-in working.
	"backend/internal/compose/microsoftsignin.go":           "Microsoft's `organizations` authority alias",
	"backend/internal/compose/microsoftsignin_test.go":      "Microsoft's `organizations` authority alias",
	"backend/internal/compose/capturegraph.go":              "Microsoft's `common` authority, which admits any tenant",
	"backend/internal/modules/capture/connectorapp.go":      "Microsoft's `organizations` authority alias",
	"backend/internal/modules/capture/connectorapp_test.go": "Microsoft's `organizations` authority alias",
	"backend/internal/modules/capture/graph/client.go":      "Microsoft's `common` authority, which admits any tenant",
	"backend/cmd/api/config.go":                             "flag help for the Microsoft authority aliases",
	".env.example":                                          "the Microsoft authority aliases, in the operator's own file",
	"docs/reference/configuration.md":                       "the Microsoft authority aliases",

	"backend/internal/platform/licensecheck/host.go": "the licence payload's `org` key. The Go " +
		"name is ours; the key belongs to whoever signs the licence, and renaming it would stop " +
		"every licence already issued from decoding",
	"backend/internal/platform/licensecheck/licensecheck_test.go": "the licence payload's `org` key",

	"backend/internal/modules/people/companynamegate.go": "the .org TLD, in the stopword list that " +
		"stops a brand written as its own domain from reducing to its suffix",

	"backend/internal/modules/agents/tools_vocabulary.go": "a deliberately WRONG argument: " +
		"`{\"target\":\"organisation\"}` is the call the schema has to refuse",
	"backend/internal/modules/agents/tools_vocabulary_test.go": "the same deliberately wrong argument",

	"backend/internal/platform/webread/schemaorg.go": "schema.org's Organization type — its word " +
		"for this thing, published by the sites we read",
	"backend/internal/platform/webread/schemaorg_test.go":  "schema.org's Organization type",
	"backend/internal/platform/webread/logoimages_test.go": "schema.org's Organization type",

	"backend/internal/modules/people/vcard.go":                                "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/people/vcard_test.go":                           "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/people/vcardimport.go":                          "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/people/vcardsourceliveness_integration_test.go": "vCard's ORG property (RFC 6350)",
	"backend/internal/compose/vcardproposal_integration_test.go":              "vCard's ORG property (RFC 6350)",
	"extensions/vn/vn.go": "an organisation in the ordinary sense — the party holding data and " +
		"the party advertising need not be the same one",
	"extensions/vn/README.md": "the same sentence as vn.go",

	"e2e/llm/testdata": "recorded model output — what a model actually said on a run, which " +
		"editing would falsify",

	"backend/gates/companyvocabulary_test.go": "this file names the word in order to refuse it",
}

func TestTheRecordTypeIsCalledCompany(t *testing.T) {
	t.Parallel()

	read := 0
	for _, f := range trackedFiles(t) {
		if f.symlink || !readableAsText(f.path) {
			continue
		}
		if reason, ok := exemptionFor(f.path); ok {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%s is exempt with no reason — say why the word belongs there", f.path)
			}
			continue
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, f.path))
		if err != nil {
			t.Fatalf("reading %s: %v", f.path, err)
		}
		read++
		for i, line := range strings.Split(string(body), "\n") {
			if !namesTheOtherWord(line) {
				continue
			}
			t.Errorf("%s:%d calls the record type by its old name:\n\t%s\n"+
				"It is called company. If this one is something else — a .org host, a meeting "+
				"organizer, Microsoft's authority alias — add the path to exempt, with the reason.",
				f.path, i+1, strings.TrimSpace(line))
		}
	}

	// Under-recognition is the one way this can fail without failing: read a
	// smaller tree, report PASS, and no assertion notices.
	if read < 1000 {
		t.Fatalf("the census read %d files, too few to be this tree — a scan that comes up "+
			"short reports the clean result it never looked for", read)
	}
}

// namesTheOtherWord reports whether a line calls this record type by a former
// name, once the words that merely resemble it are taken out.
func namesTheOtherWord(line string) bool {
	line = hostname.ReplaceAllString(line, "")
	line = notThisRecordType.ReplaceAllString(line, "")
	return theOtherWord.MatchString(line)
}

func exemptionFor(path string) (string, bool) {
	for prefix, reason := range exempt {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return reason, true
		}
	}
	return "", false
}

// readableAsText keeps binaries and lockfiles out: a .png cannot say the word,
// and a lockfile says whatever the registry named the package.
func readableAsText(path string) bool {
	switch filepath.Ext(path) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2",
		".pdf", ".zip", ".gz", ".sum", ".lock", ".svg", ".pen":
		return false
	}
	return filepath.Base(path) != "pnpm-lock.yaml"
}
