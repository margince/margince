// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/tools/internal/oasnode"
)

// permissionNames are the passport permissions as the app names them, keyed by
// the scope x-mcp-tool declares.
var permissionNames = map[string]string{
	"read": "Read records", "draft": "Draft messages", "write": "Change records",
	"send": "Send messages", "enrich": "Buy contact data",
}

// tierSentences say what the agent gate does with a mutating call at each tier.
var tierSentences = map[string]string{
	"auto_execute":          "Runs at once.",
	"confirmation_required": "Waits for a human to approve.",
	"dynamic":               "May wait for a human to approve, depending on the change.",
}

var modelSentences = map[string]string{
	"always": "Holds the request open while an AI model answers.",
	"on-miss": "May hold the request open while an AI model answers, " +
		"when no stored answer exists yet.",
}

// aside is one parenthetical, which clean drops when it holds a developer note.
var aside = regexp.MustCompile(`\s*\([^()]*\)`)

// developerMarkers are the forms of build notes the contract's prose carries
// for its own developers. They are decision, backlog and requirement labels,
// specification paths and sections, migration numbers and storage detail. A
// description holding one is not for an agent. The label forms are the ones
// backend/gates/followablecitations_test.go refuses in a touched line.
var developerMarkers = regexp.MustCompile(strings.Join([]string{
	`\bADR-\d+`, `features/`, `data-model`, `interfaces\.md`, `(^|[^\w/.-])spec/`, `§`,
	`\b(B-|S-)?EP?\d{2}(\.\d+[a-z]?)*\b`, `\bUAT-PLAN-\d\b`, `\bOP-\d{1,2}\b`,
	`\b[A-Z]{2,}(-[A-Z]+)+-[A-Z]?\d+[a-z]?\b`, `\b[A-Z]{4,}-\d{1,2}\b`, `\bmigrations? \d{4}\b`, `founder decision`,
	`\bIS (NOT )?NULL\b`, `\btsvector\b`, `\b(SELECT|INSERT|UPDATE|DELETE) .*\b(FROM|INTO|SET|WHERE)\b`,
}, "|"))

// prose cleans the contract's descriptions for an agent. tables are the
// storage table names, which no wire description has reason to carry.
type prose struct{ tables *regexp.Regexp }

func newProse(tables []string) prose {
	if len(tables) == 0 {
		return prose{}
	}
	return prose{tables: regexp.MustCompile(`\b(` + strings.Join(tables, "|") + `)\b`)}
}

// tierMarks are the contract's shorthand for the two tiers, which an agent
// reads the sentences describeOperation writes for instead.
var tierMarks = strings.NewReplacer(" 🟢", "", " 🟡", "", "🟢", "", "🟡", "")

func (p prose) isDeveloperNote(text string) bool {
	return developerMarkers.MatchString(text) || (p.tables != nil && p.tables.MatchString(text))
}

// clean trims text to its first paragraph without asides that are developer
// notes. It returns "" when what is left is a developer note too.
func (p prose) clean(node *yaml.Node) string {
	text := strings.TrimSpace(node.Value)
	breakAt := "\n\n"
	if node.Style == yaml.FoldedStyle {
		breakAt = "\n"
	}
	text, _, _ = strings.Cut(text, breakAt)
	text = aside.ReplaceAllStringFunc(tierMarks.Replace(text), func(parenthetical string) string {
		if p.isDeveloperNote(parenthetical) {
			return ""
		}
		return parenthetical
	})
	text = strings.TrimSpace(text)
	if p.isDeveloperNote(text) {
		return ""
	}
	return text
}

// describeOperation rewrites an operation's summary and description: the
// first clean paragraph, else the summary, then what the agent gate will do
// with the call.
func (p prose) describeOperation(op operation) {
	summary := ""
	if node, ok := oasnode.Lookup(op.node, "summary"); ok {
		summary = p.clean(node)
		setText(op.node, "summary", summary)
	}
	text := summary
	if node, ok := oasnode.Lookup(op.node, "description"); ok {
		if cleaned := p.clean(node); cleaned != "" {
			text = cleaned
		}
	}
	if summary == "" && text != "" {
		setText(op.node, "summary", firstSentence(text))
	}
	var sentences []string
	if text != "" {
		sentences = append(sentences, text)
	}
	if op.mutating() {
		if name := permissionNames[op.scope]; name != "" {
			sentences = append(sentences, "Needs the passport permission "+name+".")
		}
		sentences = append(sentences, tierSentences[op.tier])
	}
	if sentence := modelSentences[op.waitsOnModel]; sentence != "" {
		sentences = append(sentences, sentence)
	}
	setText(op.node, "description", strings.Join(sentences, "\n\n"))
}

func (op operation) mutating() bool {
	switch op.method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// sanitize cleans every description and summary under node, and drops an
// example that carries a developer note. Keys directly under properties are
// field names, so a field called description or example is left alone.
func (p prose) sanitize(node *yaml.Node, namesOnly bool) {
	if node.Kind == yaml.MappingNode && !namesOnly {
		kept := node.Content[:0]
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			switch {
			case (key.Value == "description" || key.Value == "summary") && value.Kind == yaml.ScalarNode:
				value.Value = p.clean(value)
				if value.Value == "" {
					continue
				}
			case (key.Value == "example" || key.Value == "examples") && p.carriesNote(value):
				continue
			}
			kept = append(kept, key, value)
		}
		node.Content = kept
	}
	for i, child := range node.Content {
		isNames := node.Kind == yaml.MappingNode && i%2 == 1 && !namesOnly &&
			node.Content[i-1].Value == "properties"
		p.sanitize(child, isNames)
	}
}

func (p prose) carriesNote(node *yaml.Node) bool {
	if node.Kind == yaml.ScalarNode {
		return p.isDeveloperNote(node.Value)
	}
	for _, child := range node.Content {
		if p.carriesNote(child) {
			return true
		}
	}
	return false
}

// firstSentence stands in for a summary that was a developer note.
func firstSentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if sentence, _, found := strings.Cut(text, ". "); found {
		return sentence + "."
	}
	return text
}

// setText sets a scalar under key, or removes the key when text is empty.
func setText(node *yaml.Node, key, text string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key {
			continue
		}
		if text == "" {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
		node.Content[i+1] = textNode(text)
		return
	}
	if text != "" {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, textNode(text))
	}
}

func textNode(text string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: text}
	if strings.Contains(text, "\n") {
		node.Style = yaml.LiteralStyle
	}
	return node
}
