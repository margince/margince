// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package techprofile

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// rulesJSON is the fingerprint ruleset, vendored rather than fetched.
//
// It is HAND-CURATED and deliberately small, biased toward the German SMB
// market this product sells into. The obvious alternative — Wappalyzer's
// dataset — is not available to us: the project's open-source dataset was
// withdrawn and every maintained fork is GPL, which this BUSL-licensed tree
// cannot carry. A short honest list we own beats a large one we may not ship.
//
//go:embed data/rules.json
var rulesJSON []byte

// pinnedRuleCount is what the embedded file holds. A rule accidentally deleted
// while editing the JSON would otherwise shrink the ruleset silently, and a
// fingerprint that recognizes less looks exactly like a company that runs less.
const pinnedRuleCount = 34

// Rule is one technology and the markers that prove it.
//
// A rule matches when ANY of its markers matches. The markers are all
// substring tests over things a page states about itself, lowercased on both
// sides — a technology announcing itself is not trying to hide, so a substring
// is the right strength.
type Rule struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Generator matches inside <meta name="generator">.
	Generator []string `json:"generator"`
	// Headers maps a lowercased response-header name to the values that count
	// as a match. An empty string among the values means the header's PRESENCE
	// is the match, whatever it says.
	Headers map[string][]string `json:"headers"`
	// Cookies matches the names of cookies the response set. Values are never
	// consulted: a cookie's name identifies the software, its value is a
	// visitor's session.
	Cookies []string `json:"cookies"`
	// Scripts matches inside the src of a <script> the page loads.
	Scripts []string `json:"scripts"`
	// Body matches inside the raw HTML, for markers that live in markup no
	// tag-level harvest reaches.
	Body []string `json:"body"`
}

var (
	rulesOnce sync.Once
	rules     []Rule
	rulesErr  error
)

// Rules returns the embedded ruleset, parsed once.
func Rules() ([]Rule, error) {
	rulesOnce.Do(func() {
		if err := json.Unmarshal(rulesJSON, &rules); err != nil {
			rulesErr = fmt.Errorf("techprofile: reading the embedded fingerprint rules: %w", err)
			return
		}
		if len(rules) != pinnedRuleCount {
			rulesErr = fmt.Errorf(
				"techprofile: the embedded fingerprint rules hold %d entries, pinned at %d",
				len(rules), pinnedRuleCount)
		}
	})
	return rules, rulesErr
}

// Evidence is what a page declared, in the shape the rules are matched
// against. It mirrors webread.Fingerprint without importing it, so this
// package stays free of the fetcher and testable from a literal.
type Evidence struct {
	Headers     http.Header
	CookieNames []string
	ScriptSrcs  []string
	Generator   string
	Body        string
}

// Technologies reads the technologies a page declares.
//
// The result is sorted by key so two reads of an unchanged page produce the
// same order, which is what lets a caller compare them without sorting first.
func Technologies(evidence Evidence) ([]Signal, error) {
	ruleset, err := Rules()
	if err != nil {
		return nil, err
	}
	lowered := lower(evidence)
	var found []Signal
	for _, rule := range ruleset {
		if marker, ok := rule.match(lowered); ok {
			found = append(found, Signal{
				Field: FieldTechnology, Key: rule.Key,
				Label: rule.Label, Evidence: marker,
			})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Key < found[j].Key })
	return found, nil
}

// loweredEvidence is the evidence with every haystack lowercased once, rather
// than once per rule.
type loweredEvidence struct {
	headers   map[string]string
	cookies   []string
	scripts   []string
	generator string
	body      string
}

func lower(evidence Evidence) loweredEvidence {
	headers := make(map[string]string, len(evidence.Headers))
	for name, values := range evidence.Headers {
		headers[strings.ToLower(name)] = strings.ToLower(strings.Join(values, " "))
	}
	return loweredEvidence{
		headers:   headers,
		cookies:   lowerAll(evidence.CookieNames),
		scripts:   lowerAll(evidence.ScriptSrcs),
		generator: strings.ToLower(evidence.Generator),
		body:      strings.ToLower(evidence.Body),
	}
}

func lowerAll(values []string) []string {
	lowered := make([]string, 0, len(values))
	for _, value := range values {
		lowered = append(lowered, strings.ToLower(value))
	}
	return lowered
}

// match reports whether the rule fires, and names the marker that fired it so
// the stored evidence says what was actually seen rather than only which rule
// won.
func (r Rule) match(evidence loweredEvidence) (string, bool) {
	for _, want := range r.Generator {
		if want != "" && strings.Contains(evidence.generator, strings.ToLower(want)) {
			return "meta generator: " + evidence.generator, true
		}
	}
	for name, wanted := range r.Headers {
		got, present := evidence.headers[strings.ToLower(name)]
		if !present {
			continue
		}
		for _, want := range wanted {
			// An empty wanted value means presence alone is the match.
			if want == "" || strings.Contains(got, strings.ToLower(want)) {
				return strings.ToLower(name) + ": " + truncate(got), true
			}
		}
	}
	if marker, ok := containsAny(evidence.cookies, r.Cookies); ok {
		return "Cookie: " + marker, true
	}
	if marker, ok := containsAny(evidence.scripts, r.Scripts); ok {
		return "Script: " + truncate(marker), true
	}
	for _, want := range r.Body {
		if want != "" && strings.Contains(evidence.body, strings.ToLower(want)) {
			return "Markup: " + want, true
		}
	}
	return "", false
}

// containsAny reports the first haystack entry containing any needle.
func containsAny(haystacks, needles []string) (string, bool) {
	for _, haystack := range haystacks {
		for _, needle := range needles {
			if needle != "" && strings.Contains(haystack, strings.ToLower(needle)) {
				return haystack, true
			}
		}
	}
	return "", false
}
