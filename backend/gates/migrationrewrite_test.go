// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Applied migrations are immutable; the ratchet starts after the task ledger exists.
const migrationRewriteBaseline = "1792499838"

type rewriteToken struct {
	text   string
	quoted bool
}

func rewriteTokens(sql string) ([]rewriteToken, error) {
	var out []rewriteToken
	for i := 0; i < len(sql); {
		if n, closed := gatekit.SQLSpanAt(sql, i); n > 0 {
			if !closed {
				return nil, errors.New("unterminated SQL span")
			}
			i += n
			continue
		}
		c := sql[i]
		if c == '"' {
			var name strings.Builder
			i++
			closed := false
			for i < len(sql) {
				if sql[i] == '"' {
					i++
					if i == len(sql) || sql[i] != '"' {
						closed = true
						break
					}
				}
				name.WriteByte(sql[i])
				i++
			}
			if !closed {
				return nil, errors.New("unterminated SQL identifier")
			}
			out = append(out, rewriteToken{text: name.String(), quoted: true})
			continue
		}
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			start := i
			i++
			for i < len(sql) && (sql[i] == '_' || sql[i] == '$' || sql[i] >= 'a' && sql[i] <= 'z' || sql[i] >= 'A' && sql[i] <= 'Z' || sql[i] >= '0' && sql[i] <= '9') {
				i++
			}
			out = append(out, rewriteToken{text: strings.ToLower(sql[start:i])})
			continue
		}
		if c == '.' || c == ';' || c == '(' || c == ')' {
			out = append(out, rewriteToken{text: string(c)})
		}
		i++
	}
	return out, nil
}

func rewriteTable(tokens []rewriteToken, at int) string {
	if at < len(tokens) && tokens[at].text == "only" && !tokens[at].quoted {
		at++
	}
	if at >= len(tokens) || tokens[at].text == "(" {
		return "*"
	}
	var name strings.Builder
	name.WriteString(tokens[at].text)
	for at+2 < len(tokens) && tokens[at+1].text == "." {
		name.WriteString(".")
		name.WriteString(tokens[at+2].text)
		at += 2
	}
	return name.String()
}

func migrationRewrites(sql string) ([]string, error) {
	tokens, err := rewriteTokens(sql)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	start := 0
	triggerCreated := false
	for i, token := range tokens {
		if !token.quoted && token.text == ";" {
			if triggerCreated && i > start {
				found["*"] = true
			}
			triggerCreated = triggerCreated || createsTrigger(tokens[start:i])
			collectRewrites(tokens[start:i], found)
			start = i + 1
		}
	}
	if triggerCreated && start < len(tokens) {
		found["*"] = true
	}
	collectRewrites(tokens[start:], found)
	out := make([]string, 0, len(found))
	for table := range found {
		out = append(out, table)
	}
	slices.Sort(out)
	return out, nil
}

func collectRewrites(tokens []rewriteToken, found map[string]bool) {
	if len(tokens) == 0 || tokens[0].text == "grant" || tokens[0].text == "revoke" {
		return
	}
	insertTable := ""
	storedResult := createsStoredResult(tokens)
	for i, token := range tokens {
		if token.quoted {
			continue
		}
		switch token.text {
		case "do", "call", "truncate", "with", "values", "(", "copy", "refresh":
			// Procedural calls and bulk removal need review even when their target is indirect.
			if i == 0 {
				found["*"] = true
			}
		case "insert", "merge":
			if i+1 < len(tokens) && tokens[i+1].text == "into" {
				insertTable = rewriteTable(tokens, i+2)
				if token.text == "merge" {
					found[insertTable] = true
				}
			}
		case "select":
			if i == 0 || storedResult {
				found["*"] = true
			}
			if insertTable != "" {
				found[insertTable] = true
			}
		case "table":
			if storedResult && i > 0 && tokens[i-1].text == "as" {
				found["*"] = true
			}
		case "update":
			if table := updatedTable(tokens, i, insertTable); table != "" {
				found[table] = true
			}
		case "delete":
			if i+1 < len(tokens) && tokens[i+1].text == "from" {
				found[rewriteTable(tokens, i+2)] = true
			}
		}
	}
}

func updatedTable(tokens []rewriteToken, at int, insertTable string) string {
	if at > 0 && slices.Contains([]string{"for", "on", "before", "after", "or", "of"}, tokens[at-1].text) {
		return ""
	}
	if at > 2 && tokens[at-3].text == "for" && tokens[at-2].text == "no" && tokens[at-1].text == "key" {
		return ""
	}
	table := rewriteTable(tokens, at+1)
	if table == "set" && insertTable != "" {
		return insertTable
	}
	return table
}

type rewriteWaiver struct{ tables []string }

func rewriteWaivers(body string) (map[string]rewriteWaiver, error) {
	out := map[string]rewriteWaiver{}
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, "|", 3)
		if len(fields) != 3 {
			return nil, errors.New("expected migration filename | tables | bounded reason")
		}
		file, tables, reason := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
		if filepath.Base(file) != file || !strings.HasSuffix(file, ".sql") || tables == "" || len(strings.Fields(reason)) < 5 {
			return nil, fmt.Errorf("incomplete bounded rewrite waiver: %s", line)
		}
		if _, exists := out[file]; exists {
			return nil, fmt.Errorf("duplicate rewrite waiver: %s", file)
		}
		names := strings.Split(tables, ",")
		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}
		slices.Sort(names)
		if slices.Contains(names, "") || len(slices.Compact(slices.Clone(names))) != len(names) {
			return nil, fmt.Errorf("empty or duplicate waiver table: %s", file)
		}
		out[file] = rewriteWaiver{tables: names}
	}
	return out, nil
}

func TestCoreMigrationsKeepLargeCorrectionsInDataTasks(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("migrationrewrites.txt")
	if err != nil {
		t.Fatal(err)
	}
	waivers, err := rewriteWaivers(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("migrations/core/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("core migration census: %d files, %v", len(files), err)
	}
	findings, err := migrationRewriteErrors(files, waivers)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		t.Error(finding)
	}
}

func migrationRewriteErrors(files []string, waivers map[string]rewriteWaiver) ([]string, error) {
	var findings []string
	for _, file := range files {
		name := filepath.Base(file)
		version, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration has no version: %s", name)
		}
		if version <= migrationRewriteBaseline {
			continue
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		tables, err := migrationRewrites(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		waiver, waived := waivers[name]
		if waived {
			if !slices.Equal(tables, waiver.tables) {
				findings = append(findings, fmt.Sprintf("%s rewrite waiver is stale: declares %v, found %v", name, waiver.tables, tables))
			}
			delete(waivers, name)
		} else if len(tables) > 0 {
			findings = append(findings, fmt.Sprintf("%s rewrites %v: use a bounded data task, or declare why these tables are small in migrationrewrites.txt", name, tables))
		}
	}
	for file := range waivers {
		findings = append(findings, "stale migration rewrite waiver: "+file)
	}
	return findings, nil
}

func createsTrigger(tokens []rewriteToken) bool {
	if len(tokens) < 2 || tokens[0].text != "create" {
		return false
	}
	at := 1
	if len(tokens) > 3 && tokens[at].text == "or" && tokens[at+1].text == "replace" {
		at += 2
	}
	if tokens[at].text == "event" {
		at++
	}
	return at < len(tokens) && !tokens[at].quoted && tokens[at].text == "trigger"
}

func createsStoredResult(tokens []rewriteToken) bool {
	if len(tokens) < 2 || tokens[0].text != "create" {
		return false
	}
	at := 1
	for at < len(tokens) && slices.Contains([]string{"global", "local", "temp", "temporary", "unlogged"}, tokens[at].text) {
		at++
	}
	if at >= len(tokens) || tokens[at].quoted {
		return false
	}
	return tokens[at].text == "table" || tokens[at].text == "materialized" && at+1 < len(tokens) && tokens[at+1].text == "view"
}
