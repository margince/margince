// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mcpe2e stamps the verdicts of the MCP end-to-end lane (e2e/llm) with
// what they measured, and says which verdicts no longer describe the product.
// The lane, `make e2e-llm-report` and the stale-verdict workflow all compute
// the stamp here. It reads committed files only, so a pull request learns for free which
// verdicts it makes stale.
package mcpe2e

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// scenarioKey is one `key:` line of a scenario file. A standard YAML decoder
// refuses these files: a double-quoted `\b` is an unknown escape there. So they
// are read the way e2e/llm/check.py reads them.
var scenarioKey = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):(.*)$`)

// scenario is what the stamp needs of one file: its name, whether a model
// judges it, and every tool one of its checks names.
type scenario struct {
	name   string
	judged bool
	tools  []string
	body   []byte
	lists  map[string][]string
}

// toolKeys are the checks whose entries name a tool. Each says how check.py
// splits an entry into its any-of group, and what ends the tool's name. A
// must_not_call_with entry is one regex, where `|` already means "or".
var toolKeys = map[string]struct{ anyOf, nameEnd string }{
	"must_call":           {"|", ""},
	"may_call":            {"|", ""},
	"must_call_with":      {"|", "."},
	"must_not_call_with":  {"", "~"},
	"must_call_matching":  {" || ", "~"},
	"counts_when_refused": {"", ""},
}

func readScenario(path string) (scenario, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- a committed scenario file, never a request's
	if err != nil {
		return scenario{}, err
	}
	reader := scenarioReader{lists: map[string][]string{}}
	lines := bufio.NewScanner(bytes.NewReader(body))
	lines.Buffer(make([]byte, 0, 64*1024), len(body)+1)
	for lines.Scan() {
		reader.line(strings.TrimRight(lines.Text(), " \t"))
	}
	if err := lines.Err(); err != nil {
		return scenario{}, fmt.Errorf("%s: %w", path, err)
	}
	if reader.name == "" {
		return scenario{}, fmt.Errorf("%s names no scenario", path)
	}
	return scenario{
		name: reader.name, body: body, judged: len(reader.lists["judge"]) > 0,
		tools: scenarioTools(reader.lists), lists: reader.lists,
	}, nil
}

// RequiredCalls names, per scenario, every call a check requires a run to make.
// Each alternative counts, since a run may make any of them. A must_call_with
// alternative keeps its tool.argument=value. A must_call_matching one keeps its
// tool, plus tool.argument=value for each "argument": "value" in its pattern.
// So an action pinned only by a pattern is still seen.
func RequiredCalls(dir string) (map[string][]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, path := range paths {
		sc, err := readScenario(path)
		if err != nil {
			return nil, err
		}
		for _, key := range []string{"must_call", "must_call_with", "must_call_matching"} {
			shape := toolKeys[key]
			for _, entry := range sc.lists[key] {
				for alternative := range strings.SplitSeq(entry, shape.anyOf) {
					out[sc.name] = append(out[sc.name], requiredCall(strings.TrimSpace(alternative), shape.nameEnd)...)
				}
			}
		}
	}
	return out, nil
}

// patternMember is one "argument": "value" a must_call_matching pattern spells.
var patternMember = regexp.MustCompile(`"([a-z_]+)":\s*"([a-z_]+)"`)

func requiredCall(alternative, nameEnd string) []string {
	if alternative == "" {
		return nil
	}
	if nameEnd != "~" {
		return []string{alternative}
	}
	tool, pattern, _ := strings.Cut(alternative, nameEnd)
	calls := []string{tool}
	for _, member := range patternMember.FindAllStringSubmatch(pattern, -1) {
		calls = append(calls, tool+"."+member[1]+"="+member[2])
	}
	return calls
}

// scenarioReader is the state of one pass over a scenario file: the key a
// list item belongs to, and whether a `|` block is still running.
type scenarioReader struct {
	name, current string
	inBlock       bool
	lists         map[string][]string
}

func (r *scenarioReader) line(line string) {
	if r.inBlock && (line == "" || strings.HasPrefix(line, "  ")) {
		return
	}
	r.inBlock = false
	if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}
	if item, ok := strings.CutPrefix(line, "  - "); ok && r.current != "" {
		r.lists[r.current] = append(r.lists[r.current], unquote(strings.TrimSpace(item)))
		return
	}
	key := scenarioKey.FindStringSubmatch(line)
	if key == nil {
		r.current = ""
		return
	}
	r.current = key[1]
	value := strings.TrimSpace(key[2])
	switch {
	case value == "|":
		r.inBlock = true
	case r.current == "name":
		r.name = unquote(value)
	case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
		for item := range strings.SplitSeq(strings.Trim(value, "[]"), ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				r.lists[r.current] = append(r.lists[r.current], unquote(trimmed))
			}
		}
	}
}

func scenarioTools(lists map[string][]string) []string {
	var out []string
	for key, shape := range toolKeys {
		for _, entry := range lists[key] {
			group := []string{entry}
			if shape.anyOf != "" {
				group = strings.Split(entry, shape.anyOf)
			}
			for _, alternative := range group {
				name := strings.TrimSpace(alternative)
				if shape.nameEnd != "" {
					name, _, _ = strings.Cut(name, shape.nameEnd)
				}
				if name != "" {
					out = append(out, name)
				}
			}
		}
	}
	return out
}

func unquote(text string) string {
	if len(text) >= 2 && (text[0] == '"' || text[0] == '\'') && text[len(text)-1] == text[0] {
		return text[1 : len(text)-1]
	}
	return text
}
