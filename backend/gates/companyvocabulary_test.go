// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// The record type is called company, and this is what stops the other word
// coming back.
//
// It never had to come back, because it never left: the product said company
// on every screen while the schema said company, parts of the code said
// company, and the tool surface said account. A model asking for record_type
// "company" was refused by a registry that admitted only "company", and a
// reader grepping for the company table found nothing. Nothing failed, because
// nothing was looking — which is why the fix is a test and not a sweep.
//
// WHAT THIS DOES NOT COVER, said rather than left to be assumed. First, a
// dotted name inside quotes: `"license.holder.org"` is a translation key and
// `"example.org"` is a host, and no amount of pattern reads one as the other —
// both are a dotted label between two quotes. The .org exclusion takes both, so
// a key ending in the old word survives it, and the same is true of a map whose
// keys are hosts. Second, the word account. It names this record type on a dozen tool verbs and forty interface
// strings, and it ALSO names a LinkedIn account, a channel Official Account,
// and the ordinary words accountable and accountability. Telling those apart is
// judgement per occurrence, so this test does not pretend to — a gate that
// covers most of a rule reads as covering all of it.
//
// The corpus comes from the index, so a new file is in scope automatically and
// an untracked scratch file is not.

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theOtherWord matches the record type's former names. `org` only where it is
// a whole identifier component: it is a substring of forget, Georgia, morgue
// and borgen, and a census that fired on those would be turned off.
//
// The camelCase HUMP is spelled out rather than folded into a case-insensitive
// class, and that is the case this test was written without: `orgID` and
// `partnerOrgID` are the commonest form the abbreviation takes, and an earlier
// version of this pattern read a boundary as "not a letter" and so matched
// neither. It reported a clean tree over a planted one.
var theOtherWord = regexp.MustCompile(
	`[Oo]rgani[sz]ation|ORGANI[SZ]ATION` +
		`|(?:^|[^A-Za-z0-9])orgs?(?:[^A-Za-z0-9]|$)` +
		`|(?:^|[^A-Za-z0-9])orgs?[A-Z0-9]` +
		`|Orgs?(?:[^A-Za-z0-9]|$)|Orgs?[A-Z0-9]` +
		`|(?:^|[^A-Za-z0-9])ORGS?(?:[^A-Za-z0-9]|$)`)

// notThisRecordType is the words that merely look like it: a meeting organizer,
// the ordinary adjective, the ordinary verb, and REORGANISATION — restructuring
// a sales pipeline, which is a thing this product's prose says and has nothing
// to do with the record type. Removed before the match rather than waived per
// site, because they are a property of English and not of any one file.
//
// reorgani[sz]ation comes FIRST: the alternation is ordered, and a bare
// organi[sz]ation branch would eat the tail of the longer word and leave `re`
// behind for the next reader to puzzle over.
var notThisRecordType = regexp.MustCompile(
	`(?i)reorgani[sz]ations?|organi[sz]er|organi[sz]ational|organi[sz]e[sd]|organi[sz]ing|organi[sz]es`)

// hostname matches a .org TLD. There are 275 in this tree — a go.mod require,
// a disposable-email fixture list, schema.org in a comment — and in each one
// `org` sits between a dot and a slash or a quote exactly as an identifier
// component would.
//
// What it must NOT swallow is the dotted form of the word itself: `deal.org`,
// `holder.org`, `{ ...org }`, `o.org_id`, `"license.holder.org"`. An earlier
// version matched any label before `.org` and so read every one of those as a
// host — including two SQL statements that selected a column their own alias
// list no longer declared. So the host must START where a host starts (a
// quote, whitespace, a slash, an @) and END where a TLD ends: a full stop
// closing the sentence, or a two-letter country below it (`mail.org.uk`), but
// never `_` and never a longer label — `opts.orgs.map` is a field and a call,
// not a domain. The word predicate no longer treats a preceding dot as a
// boundary it can ignore.
var hostname = regexp.MustCompile(
	"(?:^|[\\s\"'`(<,=|/@\\[])(?:[a-z0-9][a-z0-9.\\\\-]*)?\\\\?\\.orgs?(?:\\.[a-z]{2}(?:[^A-Za-z0-9_]|$)|\\.(?:\\s|$)|[^A-Za-z0-9_.]|$)")

// exempt names a path and says why the word is right there. The reason is the
// point: a waiver nobody had to justify is how the second spelling survived
// review for a year.
var exempt = gatekit.Waive(map[string]string{
	"backend/migrations/core": "shipped migrations are never edited — the SQL that built the " +
		"old names is the record of how the schema got here",
	"backend/migrations/custom": "the same, one namespace over — and the migration that MOVES a\n\t\tstored value has to name the value it is moving",
	"backend/migrations/testdata/rbac_baseline_era_defaults.json": "pinned byte for byte to the " +
		"baseline commit by rbacbaselineerafixture_test.go; it IS the matrix the server seeded then",
	"CHANGELOG.md": "entries say what they said when they were written",
	"sbom-schemas": "a vendored SPDX schema, not ours to rename",
	".github/workflows/release.yml": "a GitHub organisation — the account a repository belongs " +
		"to, not this record type",
	"sonar-project.properties": "`sonar.organization` is the scanner's own mandatory property, " +
		"naming the SonarCloud organisation this project is filed under. Renaming it does not " +
		"fail the scan's quality gate — it stops the scan running at all, which is a different " +
		"colour of red and took a CI round to read",

	// Microsoft's authority alias: `companies` is a literal path segment at
	// login.microsoftonline.com beside `common` and `consumers`, and renaming
	// it stops sign-in working.
	"backend/internal/compose/microsoftsignin.go":           "Microsoft's `companies` authority alias",
	"backend/internal/compose/microsoftsignin_test.go":      "Microsoft's `companies` authority alias",
	"backend/internal/compose/capturegraph.go":              "Microsoft's `common` authority, which admits any tenant",
	"backend/internal/modules/capture/connectorapp.go":      "Microsoft's `companies` authority alias",
	"backend/internal/modules/capture/connectorapp_test.go": "Microsoft's `companies` authority alias",
	"backend/internal/modules/capture/graph/client.go":      "Microsoft's `common` authority, which admits any tenant",
	"backend/cmd/api/config.go":                             "flag help for the Microsoft authority aliases",
	".env.example":                                          "the Microsoft authority aliases, in the operator's own file",
	"docs/reference/configuration.md":                       "the Microsoft authority aliases",

	"backend/internal/platform/licensecheck/host.go": "the licence payload's `company` key. The Go " +
		"name is ours; the key belongs to whoever signs the licence, and renaming it would stop " +
		"every licence already issued from decoding",
	"backend/internal/platform/licensecheck/licensecheck_test.go": "the licence payload's `company` key",

	"backend/internal/modules/contacts/companynamegate.go": "the .org TLD, in the stopword list that " +
		"stops a brand written as its own domain from reducing to its suffix",

	"backend/internal/modules/agents/tools_vocabulary.go": "a deliberately WRONG argument: " +
		"`{\"target\":\"organisation\"}` is the call the schema has to refuse",
	"backend/internal/modules/agents/tools_vocabulary_test.go": "the same deliberately wrong argument",

	"backend/internal/platform/webread/schemaorg.go": "schema.org's Company type — its word " +
		"for this thing, published by the sites we read",
	"backend/internal/platform/webread/schemaorg_test.go":  "schema.org's Company type",
	"backend/internal/platform/webread/logoimages_test.go": "schema.org's Company type",

	"backend/internal/modules/contacts/vcard.go":                                "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/contacts/vcard_test.go":                           "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/contacts/vcardimport.go":                          "vCard's ORG property (RFC 6350)",
	"backend/internal/modules/contacts/vcardsourceliveness_integration_test.go": "vCard's ORG property (RFC 6350)",
	"backend/internal/compose/vcardproposal_integration_test.go":                "vCard's ORG property (RFC 6350)",
	"extensions/vn/vn.go": "an organisation in the ordinary sense — the party holding data and " +
		"the party advertising need not be the same one",
	"extensions/vn/README.md": "the same sentence as vn.go",

	"e2e/llm/testdata": "recorded model output — what a model actually said on a run, which " +
		"editing would falsify",
	"e2e/llm/scenarios/case2-business-card.yaml": "one judged CRITERION quotes a sentence a " +
		"model might write. The offline judge is keyed on sha256(criterion + answer), so " +
		"rewording a criterion by one word orphans every verdict recorded against it — four " +
		"of them here — and they can only be recomputed against a live model. The word is " +
		"illustrative; the cache is not",

	"backend/gates/rlsclaimsprose_test.go": "its waiver keys quote shipped migrations verbatim, " +
		"and a shipped migration is never edited — the quote has to keep the word the SQL says",

	"backend/internal/compose/auditlegacytype.go": "the word audit rows written before the\n		rename still carry. `trg_audit_no_mutate` refuses an UPDATE on audit_log, so those rows\n		keep it forever and the two reads that filter the trail by record type have to match it —\n		the file exists to say so once instead of twice",

	"docs/reference/record-vocabulary.md": "it states the rule both record nouns hold, which it " +
		"cannot do without naming the word each one retires",

	"backend/gates/companyvocabulary_test.go": "this file names the word in order to refuse it",
})

func TestTheRecordTypeIsCalledCompany(t *testing.T) {
	t.Parallel()

	read := 0
	for _, f := range trackedFiles(t) {
		if f.symlink || !readableAsText(f.path) {
			continue
		}
		if exemptionCovers(t, f.path) {
			continue
		}
		// The PATH says the word as loudly as the contents do, and a census
		// that read only the contents let graphorgreach.go, csvorgwriter.go
		// and linkedinorgplace.go through — three files whose bodies had been
		// renamed and whose names had not.
		if pathNamesTheOtherWord(f.path) {
			t.Errorf("%s is NAMED for the record type's old name. It is called company.", f.path)
		}
		body, err := os.ReadFile(filepath.Join(repoRoot, f.path))
		if err != nil {
			t.Fatalf("reading %s: %v", f.path, err)
		}
		if binary(body) {
			continue
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

// TestTheWordIsSeenAfterADot plants the dotted forms, because the exclusion
// that keeps a .org TLD quiet is the one place this census can go silently
// permissive: widen it by one character and `deal.org`, `{ ...org }` and
// `o.org_id` all read as hosts, which is exactly how two SQL statements came to
// select a column their own alias list no longer declared.
func TestTheWordIsSeenAfterADot(t *testing.T) {
	t.Parallel()

	for _, line := range []string{
		`  if (!deal.org) {`,
		`      <Avatar name={deal.org} src={deal.companyLogoUrl} />`,
		`    const notMine = { ...org, owner_id: "u-2" };`,
		`		 WHERE c.org_count = 1`,
		`	return sprintf("SELECT DISTINCT l.activity_id, o.org_id AS company_id")`,
		`  "contact.enriched.field.org_name": "Company",`,
		`  const options = opts.orgs.map((company) => ({`,
	} {
		if !namesTheOtherWord(line) {
			t.Errorf("the census reads past the old word in:\n\t%s", line)
		}
	}

	for _, host := range []string{
		`	if strings.Contains(public.String(), "openstreetmap.org") {`,
		`		"email":        "subject@example.org",`,
		`// A schema.org block names it in so many words.`,
		`var webTLDs = []string{".de", ".com", ".net", ".org", ".co"}`,
		`GOVULNCHECK ?= $(call resolve-gate-tool,govulncheck,golang\.org/x/vuln)`,
		`	expect(screen.queryByText(/mail\.example\.org/)).not.toBeInTheDocument();`,
		`golang.org/x/tools v0.38.0 // indirect`,
		`// endpoint could produce the state. Every reorganisation of a sales process`,
		`// client and this suite would reach api.telegram.org.`,
		`mail.org.uk`,
		`2ch.orgs.hk`,
	} {
		if namesTheOtherWord(host) {
			t.Errorf("the census reads a .org host as the old word in:\n\t%s", host)
		}
	}
}

// pathNamesTheOtherWord reports whether a path segment is named for the record
// type's old name. It cannot reuse the line predicate: that one leans on a
// boundary — a hump, a punctuation mark, a space — and a Go file name has
// none. `csvorgwriter.go` is a single lowercase run, and it is exactly the
// shape that walked past the contents-only census with its body already
// renamed. So the path check matches the bare letters and takes the English
// out first.
func pathNamesTheOtherWord(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		segment = strings.ToLower(segment)
		if strings.HasSuffix(segment, ".org") {
			continue
		}
		if strings.Contains(spellingsThatAreNotTheWord.ReplaceAllString(segment, ""), "org") {
			return true
		}
	}
	return false
}

// spellingsThatAreNotTheWord is the English that merely spells those three
// letters in the middle of a name: forged, forget, anchorguard, schema.org.
// Removed before the match for the reason notThisRecordType is — they are a
// property of the language, not of any one file — and held to that by
// TestThePathCheckSeesAConcatenatedName, which plants both a real name and an
// innocent one so a list that grew until it excused everything fails.
var spellingsThatAreNotTheWord = regexp.MustCompile(
	`forge|forget|forgive|anchorguard|anchorgate|schemaorg|morgue|gorge|georgia|borg`)

// TestThePathCheckSeesAConcatenatedName plants the case the path check exists
// for. Under-recognition is silent: a predicate that stopped matching would
// leave the census reporting a clean tree, and no assertion above would notice.
func TestThePathCheckSeesAConcatenatedName(t *testing.T) {
	t.Parallel()

	for _, named := range []string{
		"backend/internal/compose/csvorgwriter.go",
		"backend/internal/modules/search/graphorgreach.go",
		"backend/internal/modules/contacts/linkedinorgplace.go",
		"backend/internal/orgs/doc.go",
		"backend/internal/modules/organization/doc.go",
	} {
		if !pathNamesTheOtherWord(named) {
			t.Errorf("%s is named for the old word and the path check does not see it", named)
		}
	}

	for _, innocent := range []string{
		"backend/internal/modules/contacts/anchorguard.go",
		"backend/internal/modules/contacts/projectanchorgate_test.go",
		"backend/internal/compose/aicert/corpus/capture_counterparty_verdict/forged_fence_01.yaml",
		"backend/internal/modules/ai/structured_forget_test.go",
		"backend/internal/platform/webread/schemaorg.go",
		"backend/internal/modules/contacts/company.go",
	} {
		if pathNamesTheOtherWord(innocent) {
			t.Errorf("%s is ordinary English and the path check reads it as the old word", innocent)
		}
	}
}

// namesTheOtherWord reports whether a line calls this record type by a former
// name, once the words that merely resemble it are taken out.
func namesTheOtherWord(line string) bool {
	line = hostname.ReplaceAllString(line, "")
	line = notThisRecordType.ReplaceAllString(line, "")
	return theOtherWord.MatchString(line)
}

// exemptionCovers reports whether path, or a directory above it, is ratified.
// The waiver set holds each reason to a standard and reports the entries that
// stopped matching, so a path that moves does not leave a standing permission.
func exemptionCovers(t *testing.T, path string) bool {
	for _, prefix := range exempt.Subjects() {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return exempt.Waived(t, prefix)
		}
	}
	return false
}

// readableAsText keeps lockfiles out — a lockfile says whatever the registry
// named the package, which is not this tree's vocabulary to choose.
func readableAsText(path string) bool {
	switch filepath.Ext(path) {
	case ".sum", ".lock":
		return false
	}
	return filepath.Base(path) != "pnpm-lock.yaml"
}

// binary reports content no reader reads as prose. Decided by the BYTES rather
// than by a list of extensions, because the list is the thing that goes short:
// an .mp4 under docs/evidence matched the pattern inside its compressed stream
// and was reported as a finding, and every extension nobody thought of would
// have done the same.
func binary(body []byte) bool {
	return bytes.IndexByte(body, 0) >= 0 || !utf8.Valid(body)
}
