// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mcpe2e stamps the verdicts of the MCP end-to-end lane (e2e/llm) with
// what they measured, and says which committed verdicts no longer describe the
// product. The lane writes the stamp, the coverage page and `make
// e2e-llm-report` read it, and all three compute it here, from committed files
// only, so a pull request can be told for free which verdicts it makes stale.
package mcpe2e

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// scenarioKey is one `key:` line of a scenario file. The files are a small
// subset of YAML that a standard decoder refuses (a double-quoted `\b` is an
// unknown escape there), so they are read the way e2e/llm/check.py reads them.
var scenarioKey = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*):(.*)$`)

// scenario is what the stamp needs of one file: its name, whether a model
// judges it, and every tool one of its checks names.
type scenario struct {
	name   string
	judged bool
	tools  []string
	body   []byte
}

// toolKeys are the checks whose entries name a tool: how check.py splits an
// entry into its any-of group, and what ends the tool's name inside one. A
// must_not_call_with entry is one regex, where `|` already means "or".
var toolKeys = map[string]struct{ anyOf, nameEnd string }{
	"must_call":          {"|", ""},
	"may_call":           {"|", ""},
	"must_call_with":     {"|", "."},
	"must_not_call_with": {"", "~"},
	"must_call_matching": {" || ", "~"},
}

func readScenario(path string) (scenario, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return scenario{}, err
	}
	lists := map[string][]string{}
	sc := scenario{body: body}
	var current string
	inBlock := false
	lines := bufio.NewScanner(bytes.NewReader(body))
	lines.Buffer(make([]byte, 0, 64*1024), len(body)+1)
	for lines.Scan() {
		line := strings.TrimRight(lines.Text(), " \t")
		if inBlock && (line == "" || strings.HasPrefix(line, "  ")) {
			continue
		}
		inBlock = false
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if item, ok := strings.CutPrefix(line, "  - "); ok && current != "" {
			lists[current] = append(lists[current], unquote(strings.TrimSpace(item)))
			continue
		}
		key := scenarioKey.FindStringSubmatch(line)
		if key == nil {
			current = ""
			continue
		}
		current = key[1]
		value := strings.TrimSpace(key[2])
		switch {
		case value == "|":
			inBlock = true
		case current == "name":
			sc.name = unquote(value)
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			for item := range strings.SplitSeq(strings.Trim(value, "[]"), ",") {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					lists[current] = append(lists[current], unquote(trimmed))
				}
			}
		}
	}
	if err := lines.Err(); err != nil {
		return scenario{}, fmt.Errorf("%s: %w", path, err)
	}
	if sc.name == "" {
		return scenario{}, fmt.Errorf("%s names no scenario", path)
	}
	sc.judged = len(lists["judge"]) > 0
	sc.tools = scenarioTools(lists)
	return sc, nil
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
