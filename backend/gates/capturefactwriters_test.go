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
// gatekit:fixture the tables and columns storekit.CaptureFactsMemo stands for.
var captureFacts = []captureFact{
	{table: "capture_freemail_domain"},
	{table: "capture_owner_identity"},
	{table: "workspace_email_domain"},
	{table: "company_domain", columns: []string{"domain", "domain_folded", "archived_at", "company_id"}},
	{table: "capture_connection", columns: []string{"account_label", "mail_posture", "status", "archived_at", "user_id"}},
	{table: "app_user", columns: []string{"email", "status", "archived_at", "removed_at"}},
}

var captureFactsChanged = regexp.MustCompile(`\bCaptureFactsChanged(By)?\(`)

var (
	sqlLiteralEnd = regexp.MustCompile("[`\"]")
	setListEnd    = regexp.MustCompile(`(?i)\b(WHERE|RETURNING|FROM)\b`)
)

// changes reports whether body sends a statement that changes the fact.
func (f captureFact) changes(body string) bool {
	write := regexp.MustCompile(`(?i)\b(INSERT INTO|UPDATE|DELETE FROM)\s+` + f.table + `\b`)
	for _, at := range write.FindAllStringSubmatchIndex(body, -1) {
		verb := strings.ToUpper(body[at[2]:at[3]])
		statement := body[at[1]:]
		if end := sqlLiteralEnd.FindStringIndex(statement); end != nil {
			statement = statement[:end[0]]
		}
		switch {
		case verb != "UPDATE", f.columns == nil:
			return true
		default:
			if end := setListEnd.FindStringIndex(statement); end != nil {
				statement = statement[:end[0]]
			}
			for _, column := range f.columns {
				if regexp.MustCompile(`(?i)\b` + column + `\s*=`).MatchString(statement) {
					return true
				}
			}
		}
	}
	return false
}

func TestEveryWriterOfACaptureFactSaysItChanged(t *testing.T) {
	t.Parallel()
	writers := map[string]int{}
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || !strings.HasPrefix(where, "internal/") {
			continue
		}
		for _, finding := range judgeCaptureFactWriters(t, path, writers) {
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

// judgeCaptureFactWriters judges one file's functions. A file built only for
// the integration lane is test support: it seeds a database no capture reads yet.
func judgeCaptureFactWriters(t *testing.T, path string, writers map[string]int) []string {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "//go:build integration") {
		return nil
	}
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var findings []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		body := string(source[fn.Body.Pos()-file.FileStart : fn.Body.End()-file.FileStart])
		for _, fact := range captureFacts {
			if !fact.changes(body) {
				continue
			}
			writers[fact.table]++
			if !captureFactsChanged.MatchString(body) {
				findings = append(findings, filepath.ToSlash(path)+": "+fn.Name.Name+" writes "+fact.table+
					" and never calls storekit.CaptureFactsChanged")
			}
		}
	}
	return findings
}

func TestACaptureFactWriterThatSaysNothingIsJudged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "writer.go")
	source := "package p\n\nfunc archive() {\n\t_ = `UPDATE capture_connection SET status = 'disconnected' WHERE id = $1`\n}\n\n" +
		"func cursor() {\n\t_ = `UPDATE capture_connection SET cursor = $2 WHERE id = $1 AND status = 'connected'`\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	writers := map[string]int{}
	findings := judgeCaptureFactWriters(t, path, writers)
	if len(findings) != 1 || !strings.Contains(findings[0], "archive") {
		t.Fatalf("findings = %v, want the status writer alone", findings)
	}
}
