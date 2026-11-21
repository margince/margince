// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every writer of a fact capture remembers says so in its own transaction.
//
// A capture transaction remembers the consumer-mail list, the own domains, the
// members' addresses, a seat's identities and a mailbox's sharing setting
// (storekit.CaptureFactsMemo). A writer that does not call
// storekit.CaptureFactsChanged leaves the rest of its transaction deciding on
// the old answer. The writers are found from the SQL they send, in every
// module, so a new one cannot be left off a list.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// captureFact is one table capture remembers. An UPDATE changes the fact only
// when its SET list names one of columns; nil columns means any write does.
type captureFact struct {
	table   string
	columns []string
}

// captureFacts are the tables behind what a capture transaction remembers. The
// own company's domains change only in company_domain: archiving a company
// archives its domains in the same function, and no code moves is_anchor.
var captureFacts = []captureFact{
	{table: "capture_freemail_domain"},
	{table: "capture_owner_identity"},
	{table: "workspace_email_domain"},
	{table: "company_domain", columns: []string{"domain", "domain_folded", "archived_at", "company_id"}},
	{table: "capture_connection", columns: []string{"account_label", "mail_posture", "status", "archived_at", "user_id"}},
	{table: "app_user", columns: []string{"email", "status", "archived_at", "removed_at"}},
}

var captureFactsChanged = regexp.MustCompile(`\b(CaptureFactsChanged(By)?|OwnCompanyDomainsChanged)\(`)

var (
	sqlLiteralEnd = regexp.MustCompile("[`\"]")
	setListEnd    = regexp.MustCompile(`(?i)\b(WHERE|RETURNING|FROM)\b`)
)

// changes reports whether body sends a statement that changes the fact. A SET
// list the literal leaves open for a concatenated column counts as changing it.
func (f captureFact) changes(body string) bool {
	write := regexp.MustCompile(`(?i)\b(INSERT INTO|UPDATE|DELETE FROM)\s+` + f.table + `\b`)
	for _, at := range write.FindAllStringSubmatchIndex(body, -1) {
		verb := strings.ToUpper(body[at[2]:at[3]])
		if verb != "UPDATE" || f.columns == nil {
			return true
		}
		statement, assembled := body[at[1]:], false
		if end := sqlLiteralEnd.FindStringIndex(statement); end != nil {
			assembled = strings.HasPrefix(strings.TrimSpace(statement[end[1]:]), "+")
			statement = statement[:end[0]]
		}
		if end := setListEnd.FindStringIndex(statement); end != nil {
			statement, assembled = statement[:end[0]], false
		}
		if assembled {
			return true
		}
		for _, column := range f.columns {
			if regexp.MustCompile(`(?i)\b` + column + `\s*=`).MatchString(statement) {
				return true
			}
		}
	}
	return false
}

func TestEveryWriterOfACaptureFactSaysItChanged(t *testing.T) {
	t.Parallel()
	packages := map[string][]string{}
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || !strings.HasPrefix(where, "internal/") {
			continue
		}
		packages[filepath.Dir(path)] = append(packages[filepath.Dir(path)], path)
	}
	writers := map[string]int{}
	for _, paths := range packages {
		for _, finding := range judgeCaptureFactWriters(t, paths, writers) {
			t.Error(finding)
		}
	}
	// A census that finds no writer of a fact read nothing, and would pass for that.
	for _, fact := range captureFacts {
		if writers[fact.table] == 0 {
			t.Errorf("found no writer of %s; the census reads the wrong files", fact.table)
		}
	}
}

// factSource is one parsed file of a package and its text.
type factSource struct {
	path   string
	file   *ast.File
	source string
}

// judgeCaptureFactWriters judges one package's functions. SQL held in a
// package-level declaration, such as an archive shape, is judged at every
// function that names it. A file built only for the integration lane is test
// support: it seeds a database no capture reads yet.
func judgeCaptureFactWriters(t *testing.T, paths []string, writers map[string]int) []string {
	t.Helper()
	var files []factSource
	for _, path := range paths {
		source, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "//go:build integration") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, factSource{path: path, file: file, source: string(source)})
	}
	shapes := packageFactShapes(files)
	var findings []string
	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			body := f.source[fn.Body.Pos()-f.file.FileStart : fn.Body.End()-f.file.FileStart]
			for _, fact := range captureFacts {
				if !fact.changes(body) && !namesShapeOf(body, shapes[fact.table]) {
					continue
				}
				writers[fact.table]++
				if !captureFactsChanged.MatchString(body) {
					findings = append(findings, filepath.ToSlash(f.path)+": "+fn.Name.Name+" writes "+fact.table+
						" and never calls storekit.CaptureFactsChanged")
				}
			}
		}
	}
	return findings
}

// packageFactShapes names each package-level var or const whose SQL changes a
// fact, by the fact's table.
func packageFactShapes(files []factSource) map[string][]string {
	shapes := map[string][]string{}
	for _, f := range files {
		for _, decl := range f.file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.VAR && gen.Tok != token.CONST) {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				text := f.source[value.Pos()-f.file.FileStart : value.End()-f.file.FileStart]
				for _, fact := range captureFacts {
					if !fact.changes(text) {
						continue
					}
					for _, name := range value.Names {
						shapes[fact.table] = append(shapes[fact.table], name.Name)
					}
				}
			}
		}
	}
	return shapes
}

func namesShapeOf(body string, names []string) bool {
	for _, name := range names {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(body) {
			return true
		}
	}
	return false
}

func judgeFixture(t *testing.T, source string) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "writer.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return judgeCaptureFactWriters(t, []string{path}, map[string]int{})
}

func TestACaptureFactWriterThatSaysNothingIsJudged(t *testing.T) {
	t.Parallel()
	findings := judgeFixture(t, "package p\n\nfunc archive() {\n\t_ = `UPDATE capture_connection SET status = 'disconnected' WHERE id = $1`\n}\n\n"+
		"func cursor() {\n\t_ = `UPDATE capture_connection SET cursor = $2 WHERE id = $1 AND status = 'connected'`\n}\n")
	if len(findings) != 1 || !strings.Contains(findings[0], "archive") {
		t.Fatalf("findings = %v, want the status writer alone", findings)
	}
}

func TestAFactWriteInAPackageShapeIsJudgedWhereItRuns(t *testing.T) {
	t.Parallel()
	findings := judgeFixture(t, "package p\n\nvar restore = shape{Statement: `UPDATE company_domain SET archived_at = NULL WHERE id = $1`}\n\n"+
		"func Restore() { run(restore) }\n\nfunc Unrelated() {}\n")
	if len(findings) != 1 || !strings.Contains(findings[0], "Restore writes company_domain") {
		t.Fatalf("findings = %v, want the function that runs the shape", findings)
	}
}

func TestAnAssembledSetListIsJudged(t *testing.T) {
	t.Parallel()
	findings := judgeFixture(t, "package p\n\nfunc set(column string) {\n\t_ = `UPDATE app_user SET `+column+` = $2 WHERE id = $1`\n}\n")
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want the assembled update judged", findings)
	}
}
