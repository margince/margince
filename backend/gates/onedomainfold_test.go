// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// One domain fold. owndomain.Fold is how a mail domain is compared, and every
// other reader asks it (mailidentity.NormalizeDomain, freemail's normalize).
// Five folds once answered the same question differently. A unicode and a
// punycode spelling of one domain matched in one check and not the next.
// A file that imports golang.org/x/net/idna is writing its own fold, so it
// must be the fold itself or say here why it is not one.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const idnaImportPath = "golang.org/x/net/idna"

var idnaReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/platform/owndomain/owndomain.go":   "is the fold itself, and the Spellings that a SQL comparison against stored header addresses needs because Postgres cannot fold IDNA. Cost: none, every other domain fold delegates here",
	"internal/modules/capture/owndomainstore.go": "validates an own domain before it is stored and refuses one IDNA cannot read, where Fold keeps it. It stores the same ASCII form Fold gives a readable domain. Cost: a second IDNA call, kept so a typo is refused rather than stored as protection that never fires",
	"internal/modules/capture/strangermail.go":   "adds the IDNA spelling of a sender for the stranger lookup and the Unicode spelling for the stored-counterparty repair. It is a comparison, not display. Routing it through mailidentity.AddressSpellings waits on one IDNA-folded correspondence lock (#9083), because widening the lookup first lets a punycode inbound and a Unicode outbound miss each other's lock. Cost: a second spelling helper until then",
	"internal/platform/freemail/displayname.go":  "renders a domain for display with idna.ToUnicode; it never compares. Cost: none for matching, a display-only conversion",
})

func TestOnlyTheSharedFoldReadsIDNA(t *testing.T) {
	t.Parallel()
	defer idnaReadersAdmitted.AssertAllMatched(t)
	for file, imports := range goImports(t, "internal") {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, path := range imports {
			if path != idnaImportPath {
				continue
			}
			rel := filepath.ToSlash(file)
			if idnaReadersAdmitted.Waived(t, rel) {
				continue
			}
			t.Errorf("%s imports %s: fold domains through owndomain.Fold (or mailidentity.NormalizeDomain / FoldAddress) instead of a fold of its own", rel, idnaImportPath)
		}
	}
}

// lockKeyOwner is the one file that may spell the correspondence lock key.
const lockKeyOwner = "internal/platform/mailidentity/mailidentity.go"

// TestOnlyMailIdentitySpellsTheCorrespondenceLockKey censuses the callers of
// correspondence.LockIdentity, which is the raw half of that key.
//
// The lock serializes a verdict that archives a sender's mail against the
// insert proving the workspace writes to them. correspondence is tier 0 and
// cannot fold IDNA. Its key alone gave two spellings of one domain two keys,
// so the pair serialized on nothing while looking like code that did.
func TestOnlyMailIdentitySpellsTheCorrespondenceLockKey(t *testing.T) {
	t.Parallel()
	const call = "correspondence.LockIdentity("
	owned, judged := 0, 0
	var findings []string
	for _, root := range []string{"internal", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return walkErr
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from the trusted source tree
			if readErr != nil {
				return readErr
			}
			judged++
			rel := filepath.ToSlash(path)
			if !strings.Contains(string(source), call) {
				return nil
			}
			if rel == lockKeyOwner {
				owned++
				return nil
			}
			findings = append(findings, rel)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// A walk that read nothing passes as a clean tree does.
	if judged < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", judged)
	}
	if owned == 0 {
		t.Errorf("%s no longer spells the key, so this census guards nothing", lockKeyOwner)
	}
	if len(findings) > 0 {
		t.Errorf("%d file(s) spell the correspondence lock key themselves: %s\n\n"+
			"Take it from mailidentity.CorrespondenceLockKey, which folds the address first. "+
			"A key that differs by a spelling takes a second lock and serializes nothing.",
			len(findings), strings.Join(findings, ", "))
	}
}

// foldsADomainInSQL matches a statement that normalizes a domain with SQL
// lower() instead of the shared fold.
//
// Two halves, because the mistake has two spellings. lower() wrapped around a
// domain column, which also costs the index on it. And a domain column compared
// to a lower() of the caller's value. Neither matches domain_folded, which its
// writers fold.
var foldsADomainInSQL = regexp.MustCompile(`(?is)lower\(\s*[a-z_]*\.?domain|\bdomain\s*=\s*lower\(`)

// sqlFoldsAdmitted are the statements that may still lower() a domain, by name
// and with the reason, because no fold reaches them.
var sqlFoldsAdmitted = gatekit.Waive(map[string]string{
	"internal/compose/company360/contacts.go": "compares a stored provider claim's company_domain against the company's own domains, both sides stored, inside a correlated subquery over every claim. Postgres cannot fold IDNA and no Go value passes through, so folding it needs a folded column on both sides, which is a schema change rather than a predicate change.",
})

// TestNoSQLStatementFoldsADomainWithLower censuses the SQL that compares a
// domain, which TestOnlyTheSharedFoldReadsIDNA cannot see.
//
// That gate reads imports. So a statement folding a domain with lower() passes
// it, while reading a Unicode and a punycode spelling of one domain as two.
// owndomain.Spellings is how a comparison reaches both stored spellings.
func TestNoSQLStatementFoldsADomainWithLower(t *testing.T) {
	t.Parallel()
	defer sqlFoldsAdmitted.AssertAllMatched(t)

	var findings []string
	judged := 0
	for _, root := range []string{"internal", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_gen.go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			judged++
			rel := filepath.ToSlash(path)
			for _, statement := range gatekit.SQLStatementsIn(t, path, readSource(t, path)) {
				if !foldsADomainInSQL.MatchString(statement) {
					continue
				}
				if sqlFoldsAdmitted.Waived(t, rel) {
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
	// A walk that read nothing passes as a clean tree does.
	if judged < 500 {
		t.Fatalf("the census read only %d Go files, so it covered almost nothing", judged)
	}
	if len(findings) > 0 {
		t.Errorf("%d statement(s) fold a domain with SQL lower():\n\t%s\n\n"+
			"Compare against the column the fold writes, or pass owndomain.Spellings so both "+
			"spellings of one domain are read. lower() answers them as two domains, and "+
			"wrapping the column in it also costs the index.",
			len(findings), strings.Join(findings, "\n\t"))
	}
}
