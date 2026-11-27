// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "strconv"

func helperSourceKey(source codeSource) string {
	key := strconv.Itoa(source.index) + "/" + strconv.FormatBool(source.field) + "/" + strconv.Itoa(source.fixedStatus)
	for _, part := range source.path {
		key += "/" + part
	}
	if source.status != nil {
		key += "/status/" + strconv.Itoa(source.status.index)
		for _, part := range source.status.path {
			key += "/" + part
		}
	}
	return key
}

func (r *problemFileReader) registerHelper(name string, source codeSource) bool {
	key := helperSourceKey(source)
	for _, known := range r.scan.helpers[name] {
		if helperSourceKey(known) == key {
			return false
		}
	}
	r.scan.helpers[name] = append(r.scan.helpers[name], source)
	return true
}

// A table's status and code come from the same row, never a cross product.
func (r *problemFileReader) statusForCodeLiteral(args []tsSpan, source codeSource, code, fallback int) (int, bool) {
	if source.status == nil {
		return fallback, false
	}
	parameter := source.status
	if parameter.index >= len(args) {
		return fallback, false
	}
	return r.statusFromPath(args[parameter.index], parameter.path, code, fallback)
}

func (r *problemFileReader) statusFromSpan(span tsSpan, code, fallback int) (int, bool) {
	return r.statusFromPath(span, nil, code, fallback)
}

func (r *problemFileReader) statusFromPath(span tsSpan, path []string, code, fallback int) (int, bool) {
	if span.to <= span.from {
		return fallback, false
	}
	read, ok := r.resolve(span, path, 0)
	if !ok || read.helper != "" {
		return fallback, fallback == 0
	}
	var numbers []int
	for _, literal := range read.literals {
		if _, number := unquotedNumber(r.tree.tokens[literal]); number {
			numbers = append(numbers, literal)
		}
	}
	if len(numbers) == 1 {
		number, _ := unquotedNumber(r.tree.tokens[numbers[0]])
		return number, false
	}
	row := r.tableRowOf(code)
	for _, literal := range numbers {
		if row >= 0 && r.tableRowOf(literal) == row {
			number, _ := unquotedNumber(r.tree.tokens[literal])
			return number, false
		}
	}
	return 0, true
}

func (r *problemFileReader) tableRowOf(literal int) int {
	for parent := r.tree.parent[literal]; parent >= 0; parent = r.tree.parent[parent] {
		owner := r.tree.parent[parent]
		if owner >= 0 && r.tree.is(owner, "[") && (r.tree.is(parent, "[") || r.tree.is(parent, "{")) {
			return parent
		}
	}
	return -1
}

func (r *problemFileReader) indexTypedConstant(start int) {
	tr := r.tree
	for at := start + 3; at < len(tr.tokens); at++ {
		if tr.is(at, ";") {
			return
		}
		if tr.is(at, "=") {
			r.indexConstant(tr.word(start+1), at+1)
			return
		}
		if tr.match[at] > at {
			at = tr.match[at]
		}
	}
}
