// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// A domain maps to at most one organization (data-model §4.2), so "is this
// domain taken?" is one question — and answering it discloses something either
// way. That the domain is taken reveals an organization exists with it; naming
// WHICH organization reveals a record the caller may not be allowed to read.
//
// So the answer carries a disclosure rule: `ExistingID` is filled only when
// `auth.VisibleTo` says the caller could have read that row anyway, and the 409
// stands without it otherwise. That rule is the subject of this census.
//
// A SECOND COPY OF AN EXISTENCE-HIDING RULE IS WHERE A LEAK APPEARS, because
// nothing fails when one copy stops asking. The copy keeps answering; it just
// answers with more than it should, on a path nobody re-read. There is no error
// and no failing test — the client simply receives an id it was not entitled
// to, in a field that is supposed to be sometimes-absent.
//
// The question is asked from doors that look unrelated — creating a company,
// editing its domains, saving its profile website — and each must answer the
// same collision the same way. A door that reports it without naming the
// holder leaves the reader nothing to act on.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// claimProbeOwner is the file that may ask the question. Keyed to the FILE, not
// the package: a second file in `people` writing its own probe is the same
// defect wearing the right import path.
const claimProbeOwner = "internal/modules/people/organization_domains.go"

// readsDomainClaim matches a statement that reads organization_domain to learn
// WHO holds a domain — the shape of the probe, not every read of the table.
//
// Two halves make it this question, and each rules out a real neighbour:
//
//   - `organization_id` in the PROJECTION — a read of `domain`/`is_primary` for
//     an org's own list is not asking who owns anything, and an existence probe
//     that selects 1 discloses no org at all.
//   - a filter ON THE DOMAIN — the bulk read that fetches every domain row for
//     a SET of orgs projects `organization_id` too, but it is answering "what
//     does this org own", the opposite direction, and it filters on the org.
//
// Both halves are needed. The projection alone matched that bulk read, in the
// owner's own file, where the census trusts what it finds.
var readsDomainClaim = regexp.MustCompile(
	`(?is)SELECT\s+[^;]*\borganization_id\b[^;]*\bFROM\s+organization_domain\b[^;]*\bdomain\s*=`)

// resolvers read the same table and project the same column while asking a
// DIFFERENT question: "which organization does this domain resolve to", answered
// to the system so it can attach or score a record — never to a caller as a
// refusal. Nothing about their answer reaches a client, so no disclosure rule
// governs them.
//
// Ratified by name rather than excluded by a narrower pattern, because there is
// no syntactic difference between the two questions: any pattern narrow enough
// to miss these would also miss a real second refusal probe, and it would miss
// it silently. A reason a reviewer can disagree with is worth more than a
// regular expression nobody can see the edge of.
var domainResolvers = gatekit.Waive(map[string]string{
	"internal/modules/people/dedupeorg.go": "exactOrgByDomain is dedupe tier 1: it resolves which live org a candidate's domains already belong to so the record LANDS on it. The id goes to the merge, never to a caller, so there is no 409 and nothing to gate the disclosure of.",
	"internal/modules/signals/resolver.go": "the signal resolver scores which organizations a domain points at, joining organization to skip anchors. Its ids become ranked candidates inside the resolution, not an answer any client receives.",
})

func TestEveryDomainClaimAnswersThroughOneProbe(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is a ratification for a site that has
	// moved or been fixed, and leaving it in place quietly re-exempts whatever
	// takes its name next.
	defer domainResolvers.AssertAllMatched(t)

	var findings []string
	judged, owned := 0, 0
	for _, root := range []string{".", "../extensions", "../fixtures"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "node_modules" || name == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			rel := filepath.ToSlash(path)
			if filepath.Base(path) == "domainclaimprobe_test.go" {
				return nil
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted source tree
			if readErr != nil {
				return readErr
			}
			judged++
			for _, statement := range gatekit.SQLStatementsIn(t, path, string(source)) {
				if !readsDomainClaim.MatchString(statement) {
					continue
				}
				if rel == claimProbeOwner {
					owned++
					continue
				}
				if domainResolvers.Waived(t, rel) {
					continue
				}
				findings = append(findings, rel+": "+gatekit.FirstLineOf(statement))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// A walk that reads nothing passes exactly like a clean tree.
	if judged < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", judged)
	}
	// EXACTLY one, in the owner too. Skipping the owner file wholesale would
	// let a second probe live beside the first — the same defect, at the one
	// address the census trusts. Zero means the statement has moved and the
	// census is guarding an empty file.
	if owned != 1 {
		t.Errorf("%s holds %d statements asking who owns a domain, want exactly 1 — "+
			"the owner may hold THE probe, not a second one beside it", claimProbeOwner, owned)
	}
	if len(findings) > 0 {
		t.Errorf("%d statement(s) ask who owns a domain outside %s.\n\n"+
			"The answer carries a disclosure rule — the owning org's id is returned only when "+
			"auth.VisibleTo admits it — and a second copy of an existence-hiding rule leaks "+
			"silently when one copy stops asking. Call claimedDomainOwner, which returns the "+
			"typed 409 with the id already gated:\n\n\t%s",
			len(findings), claimProbeOwner, strings.Join(findings, "\n\t"))
	}
}

// TestTheDomainClaimCensusSeesWhatItClaimsTo plants what the census must catch
// and what it must not.
//
// A gate asserting a shape is ABSENT passes identically over a clean tree and
// over a detector that has stopped detecting.
func TestTheDomainClaimCensusSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	caught := []string{
		"SELECT organization_id FROM organization_domain WHERE domain = lower($1)",
		// Wrapped across lines, which is how it is actually written.
		"SELECT organization_id\n\t\t\t  FROM organization_domain\n\t\t\t WHERE domain = lower($1)",
		// Aliased and joined — still this question.
		"SELECT od.organization_id, od.is_primary FROM organization_domain od WHERE od.domain = $1",
		// Case is not the subject.
		"select ORGANIZATION_ID from Organization_Domain where domain = $1",
		// A set of candidate domains is the same question asked in bulk.
		"SELECT organization_id FROM organization_domain WHERE domain = ANY($1) AND archived_at IS NULL",
	}
	// Assembled with `+`, which no single fragment matches. Read from the AST
	// rather than written out here, so the flattening itself is what is proven.
	concatenated := `package p
var q = "SELECT organization_id " +
	"FROM organization_domain " +
	"WHERE domain = lower($1)"`
	joined := gatekit.SQLStatementsIn(t, "concat.go", concatenated)
	if len(joined) != 1 || !readsDomainClaim.MatchString(joined[0]) {
		t.Errorf("a probe assembled with `+` is not read as one statement: %q", joined)
	}
	// The same probe in INTERPRETED quotes, where the whitespace is an escape.
	// Source text keeps the backslash, so a pattern asking for `\s` matches
	// nothing unless the literal is decoded first.
	escaped := "package p\nvar q = \"SELECT organization_id\\nFROM organization_domain\\nWHERE domain = lower($1)\""
	decoded := gatekit.SQLStatementsIn(t, "escaped.go", escaped)
	if len(decoded) != 1 || !readsDomainClaim.MatchString(decoded[0]) {
		t.Errorf("a probe whose whitespace is escaped is not decoded: %q", decoded)
	}
	for _, statement := range caught {
		if !readsDomainClaim.MatchString(statement) {
			t.Errorf("the census does not see a probe it must:\n\t%s", statement)
		}
	}
	missed := []string{
		// An org reading its OWN domains asks a different question, and every
		// domains surface does it — reporting these would bury the finding.
		"SELECT domain, is_primary FROM organization_domain WHERE organization_id = $1",
		// A bare existence probe discloses nothing about which org.
		"SELECT 1 FROM organization_domain WHERE domain = lower($1)",
		// A WRITE is not a probe; the unique index answers those.
		"INSERT INTO organization_domain (organization_id, domain) VALUES ($1, $2)",
		// A different table whose name contains this one's.
		"SELECT organization_id FROM organization_domain_history WHERE domain = $1",
		// The opposite direction: what does this SET of orgs own? It projects
		// organization_id and reads the table, and it filters on the org.
		"SELECT organization_id, id, domain, is_primary FROM organization_domain WHERE organization_id = ANY($1) AND archived_at IS NULL",
	}
	for _, statement := range missed {
		if readsDomainClaim.MatchString(statement) {
			t.Errorf("the census reports something that is not this probe:\n\t%s", statement)
		}
	}
}
