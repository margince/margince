// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Bracket structure over a TypeScript token stream. A block or a type literal
// reads as an object too, and matches `key: value` only where it holds one.

import "fmt"

type tsTree struct {
	tokens   []tsToken
	comments []tsNote
	match    []int
	parent   []int
}

func tsCloserOf(open string) string {
	switch open {
	case "(":
		return ")"
	case "[":
		return "]"
	case "{":
		return "}"
	}
	return ""
}

func buildTSTree(tokens []tsToken) (*tsTree, error) {
	tree := &tsTree{tokens: tokens, match: make([]int, len(tokens)), parent: make([]int, len(tokens))}
	var open []int
	for i, token := range tokens {
		tree.match[i] = -1
		tree.parent[i] = -1
		if len(open) > 0 {
			tree.parent[i] = open[len(open)-1]
		}
		if token.kind != tkPunct {
			continue
		}
		if tsCloserOf(token.text) != "" {
			open = append(open, i)
			continue
		}
		if token.text != ")" && token.text != "]" && token.text != "}" {
			continue
		}
		if len(open) == 0 || tsCloserOf(tokens[open[len(open)-1]].text) != token.text {
			return nil, fmt.Errorf("a %q at offset %d closes nothing it matches", token.text, token.pos)
		}
		opener := open[len(open)-1]
		open = open[:len(open)-1]
		tree.match[opener], tree.match[i] = i, opener
		tree.parent[i] = tree.parent[opener]
	}
	if len(open) > 0 {
		return nil, fmt.Errorf("a %q at offset %d never closes", tokens[open[len(open)-1]].text, tokens[open[len(open)-1]].pos)
	}
	return tree, nil
}

func (tr *tsTree) is(i int, text string) bool {
	return i >= 0 && i < len(tr.tokens) && tr.tokens[i].kind == tkPunct && tr.tokens[i].text == text
}

func (tr *tsTree) word(i int) string {
	if i >= 0 && i < len(tr.tokens) && tr.tokens[i].kind == tkIdent {
		return tr.tokens[i].text
	}
	return ""
}

// tsSpan is a half-open range of token indices.
type tsSpan struct{ from, to int }

// split cuts the inside of the bracket at open into its depth-0 parts, at
// commas and, for a brace, semicolons.
func (tr *tsTree) split(open int) []tsSpan {
	var parts []tsSpan
	start := open + 1
	for i := open + 1; i < tr.match[open]; i++ {
		if tr.match[i] > i {
			i = tr.match[i]
			continue
		}
		if tr.is(i, ",") || tr.is(open, "{") && tr.is(i, ";") {
			if i > start {
				parts = append(parts, tsSpan{start, i})
			}
			start = i + 1
		}
	}
	if tr.match[open] > start {
		parts = append(parts, tsSpan{start, tr.match[open]})
	}
	return parts
}

// tsEntry is one part of a brace read as an object literal.
type tsEntry struct {
	key       string
	keyAt     int
	value     tsSpan
	spread    bool
	shorthand bool
}

func (tr *tsTree) entries(open int) []tsEntry {
	var out []tsEntry
	for _, part := range tr.split(open) {
		first := tr.tokens[part.from]
		switch {
		case tr.is(part.from, "..."):
			out = append(out, tsEntry{keyAt: part.from, value: tsSpan{part.from + 1, part.to}, spread: true})
		case first.kind != tkIdent && first.kind != tkString && first.kind != tkNumber:
		case part.to == part.from+1 && first.kind == tkIdent:
			out = append(out, tsEntry{key: first.text, keyAt: part.from, value: tsSpan{part.from, part.to}, shorthand: true})
		case tr.is(part.from+1, ":"):
			out = append(out, tsEntry{key: first.text, keyAt: part.from, value: tsSpan{part.from + 2, part.to}})
		case tr.is(part.from+1, "?") && tr.is(part.from+2, ":"):
			out = append(out, tsEntry{key: first.text, keyAt: part.from, value: tsSpan{part.from + 3, part.to}})
		}
	}
	return out
}

// literals answers the literal tokens a value can be: one, cast with `as` or
// `satisfies`, or an arm of a conditional. true and false answer none.
func (tr *tsTree) literals(value tsSpan) ([]int, bool) {
	if value.to <= value.from {
		return nil, false
	}
	first := tr.tokens[value.from]
	single := value.to == value.from+1 || tr.word(value.from+1) == "as" || tr.word(value.from+1) == "satisfies"
	switch {
	case single && (first.kind == tkString || first.kind == tkTemplate):
		return []int{value.from}, true
	case single && first.kind == tkNumber && first.text != "<jsx/>" && first.text != "/regex/":
		return []int{value.from}, true
	case single && (first.text == "true" || first.text == "false"):
		return nil, true
	}
	yes, no, ok := conditionalArms(tr, value)
	if !ok {
		return nil, false
	}
	yesLiterals, yesOK := tr.literals(yes)
	noLiterals, noOK := tr.literals(no)
	return append(yesLiterals, noLiterals...), yesOK && noOK
}

// conditionalArms splits `cond ? yes : no` at its own operators: a `?` inside
// an arm waits for its own `:` before the outer one is reached.
func conditionalArms(tr *tsTree, value tsSpan) (yes, no tsSpan, ok bool) {
	question, open := -1, 0
	for i := value.from; i < value.to; i++ {
		if tr.match[i] > i {
			i = tr.match[i]
			continue
		}
		switch {
		case tr.is(i, "?"):
			if question < 0 {
				question = i
			}
			open++
		case tr.is(i, ":") && open > 0:
			open--
			if open == 0 {
				return tsSpan{question + 1, i}, tsSpan{i + 1, value.to}, true
			}
		}
	}
	return tsSpan{}, tsSpan{}, false
}
