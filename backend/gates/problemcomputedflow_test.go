// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "strings"

type codeRead struct {
	literals []int
	helper   string
	source   codeSource
}

// calculatedCode follows finite local expressions and status-only functions.
// Parameter values are resolved before binding, so a same-named caller cannot loop.
func (r *problemFileReader) calculatedCode(span tsSpan, path []string, depth int) (codeRead, bool) {
	tr := r.tree
	if tr.is(span.from, "(") && tr.match[span.from] == span.to-1 {
		return r.resolve(tsSpan{span.from + 1, span.to - 1}, path, depth+1)
	}
	if yes, no, ok := conditionalArms(tr, span); ok {
		condition := tsSpan{span.from, yes.from - 1}
		if value, known := r.flowCondition(condition); known {
			if value {
				return r.resolve(yes, path, depth+1)
			}
			return r.resolve(no, path, depth+1)
		}
		a, aok := r.resolve(yes, path, depth+1)
		b, bok := r.resolve(no, path, depth+1)
		return codeRead{literals: append(a.literals, b.literals...)}, aok && bok && a.helper == "" && b.helper == ""
	}
	if tr.word(span.from) == "" || !tr.is(span.from+1, "(") || tr.match[span.from+1] != span.to-1 {
		return codeRead{}, false
	}
	for _, fn := range r.scan.funcs {
		if fn.name != tr.word(span.from) {
			continue
		}
		args := tr.split(span.from + 1)
		bound := map[string]codeRead{}
		for i, param := range fn.params {
			if param.to != param.from+1 {
				return codeRead{}, false
			}
			value, ok := fn.defaults[i]
			if i < len(args) {
				value, ok = args[i], true
			}
			if !ok {
				return codeRead{}, false
			}
			read, known := r.resolve(value, nil, depth+1)
			if !known || read.helper != "" {
				return codeRead{}, false
			}
			for _, literal := range read.literals {
				if literal == r.flowCode {
					read.literals = []int{literal}
					break
				}
			}
			bound[tr.word(param.from)] = read
		}
		old := r.flowValues
		r.flowValues = bound
		result, known, _ := r.flowBody(fn.body, path, depth+1)
		r.flowValues = old
		return result, known && len(result.literals) > 0
	}
	return codeRead{}, false
}

func (r *problemFileReader) flowBody(body tsSpan, path []string, depth int) (codeRead, bool, bool) {
	if depth > 12 {
		return codeRead{}, false, false
	}
	tr := r.tree
	if !tr.is(body.from, "{") {
		if tr.word(body.from) == "return" {
			body.from++
			if tr.is(body.to-1, ";") {
				body.to--
			}
		}
		read, known := r.resolve(body, path, depth+1)
		return read, known, true
	}
	var result codeRead
	known := true
	for at := body.from + 1; at < body.to-1; at++ {
		switch tr.word(at) {
		case "switch", "try", "for", "while", "do", "throw":
			// Opaque control flow must remain in the unresolved census.
			return result, false, false
		}

		if tr.word(at) == "if" && tr.is(at+1, "(") {
			read, ok, terminated, after := r.flowIf(at, body.to-1, path, depth)
			result.literals = append(result.literals, read.literals...)
			known = known && ok
			if terminated {
				return result, known, true
			}
			at = after - 1
			continue
		}
		if tr.word(at) == "return" {
			value := r.expressionEnd(at + 1)
			if value.to > body.to-1 {
				value.to = body.to - 1
			}
			read, ok := r.resolve(value, path, depth+1)
			result.literals = append(result.literals, read.literals...)
			return result, known && ok && read.helper == "", true
		}
		if tr.match[at] > at {
			at = tr.match[at]
		}
	}
	return result, known && len(result.literals) > 0, false
}

func (r *problemFileReader) flowStatement(from, limit int) tsSpan {
	if r.tree.is(from, "{") {
		return tsSpan{from, r.tree.match[from] + 1}
	}
	span := r.expressionEnd(from)
	if span.to > limit {
		span.to = limit
	}
	if r.tree.is(span.to, ";") {
		span.to++
	}
	return span
}

func (r *problemFileReader) flowCondition(span tsSpan) (bool, bool) {
	tr := r.tree
	if span.to == span.from+1 {
		if tr.word(span.from) == "true" {
			return true, true
		}
		if tr.word(span.from) == "false" {
			return false, true
		}
	}
	if tr.is(span.from, "!") {
		value, known := r.flowCondition(tsSpan{span.from + 1, span.to})
		return !value, known
	}
	key := r.flowConditionKey(span)
	if value, known := r.codeConditions()[key]; known {
		return value, true
	}
	for at := span.from; at < span.to; at++ {
		if tr.match[at] > at {
			at = tr.match[at]
			continue
		}
		equal := tr.is(at, "===") || tr.is(at, "==")
		unequal := tr.is(at, "!==") || tr.is(at, "!=")
		if !equal && !unequal {
			continue
		}
		left, lok := r.resolve(tsSpan{span.from, at}, nil, 8)
		right, rok := r.resolve(tsSpan{at + 1, span.to}, nil, 8)
		if !lok || !rok || len(left.literals) != 1 || len(right.literals) != 1 {
			return false, false
		}
		matches := tr.tokens[left.literals[0]].text == tr.tokens[right.literals[0]].text
		if unequal {
			matches = !matches
		}
		return matches, true
	}
	return false, false
}

func (r *problemFileReader) flowConditionKey(span tsSpan) string {
	var text strings.Builder
	for at := span.from; at < span.to; at++ {
		text.WriteString(r.tree.tokens[at].text)
		text.WriteByte(' ')
	}
	return text.String()
}

// A code literal fixes the arm of the same condition used to calculate status.
func (r *problemFileReader) codeConditions() map[string]bool {
	out := r.nestedCodeConditions()
	tr := r.tree
	code := r.flowCode
	if code < 0 {
		return out
	}
	for at := range tr.tokens {
		if tr.is(at, "=") || tr.word(at) == "return" {
			span := r.expressionEnd(at + 1)
			if code < span.from || code >= span.to {
				continue
			}
			yes, no, known := conditionalArms(tr, span)
			if !known {
				continue
			}
			if code >= yes.from && code < yes.to {
				out[r.flowConditionKey(tsSpan{span.from, yes.from - 1})] = true
			}
			if code >= no.from && code < no.to {
				out[r.flowConditionKey(tsSpan{span.from, yes.from - 1})] = false
			}
		}
	}
	return out
}

func (r *problemFileReader) localCalculatedValue(at int, name string) (tsSpan, bool) {
	tr := r.tree
	best := -1
	var value tsSpan
	for i := range at {
		if tr.word(i) != name || !tr.is(i+1, "=") {
			continue
		}
		if tr.word(i-1) != "const" && tr.word(i-1) != "let" && tr.word(i-1) != "var" {
			continue
		}
		scope := tr.parent[i]
		inside := scope < 0
		for parent := tr.parent[at]; parent >= 0; parent = tr.parent[parent] {
			if parent == scope {
				inside = true
				break
			}
		}
		if inside && i > best {
			best = i
			value = r.expressionEnd(i + 2)
		}
	}
	return value, best >= 0
}

func (r *problemFileReader) flowIf(at, limit int, path []string, depth int) (codeRead, bool, bool, int) {
	tr := r.tree
	var result codeRead
	known := true
	end := tr.match[at+1]
	condition, decided := r.flowCondition(tsSpan{at + 2, end})
	start := end + 1
	branch := r.flowStatement(start, limit)
	after := branch.to
	var otherwise tsSpan
	if tr.word(after) == "else" {
		otherwise = r.flowStatement(after+1, limit)
		after = otherwise.to
	}
	terminated := false
	if !decided || condition {
		read, ok, ends := r.flowBody(branch, path, depth+1)
		result.literals = append(result.literals, read.literals...)
		known = known && ok
		terminated = ends
	}
	if otherwise.to > otherwise.from && (!decided || !condition) {
		read, ok, ends := r.flowBody(otherwise, path, depth+1)
		result.literals = append(result.literals, read.literals...)
		known = known && ok
		if !decided {
			terminated = terminated && ends
		} else {
			terminated = ends
		}
	} else if !decided || !condition {
		terminated = false
	}
	return result, known, terminated, after
}
