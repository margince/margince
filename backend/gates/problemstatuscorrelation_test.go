// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"strconv"
	"strings"
)

func helperSourceKey(source codeSource) string {
	parts := []string{strconv.Itoa(source.index), strconv.FormatBool(source.field), strconv.Itoa(source.fixedStatus), strconv.FormatBool(source.array), strconv.FormatBool(source.rest)}
	parts = append(parts, source.path...)
	if source.status != nil {
		parts = append(parts, "status", strconv.Itoa(source.status.index))
		parts = append(parts, source.status.path...)
	}
	return strings.Join(parts, "/")
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
	r.statusVariants = nil
	old := r.flowCode
	r.flowCode = code
	read, ok := r.finiteStatus(span, path, 0)
	defer func() { r.flowCode = old }()
	if !ok || read.helper != "" {
		return fallback, fallback == 0
	}
	var numbers []int
	for _, literal := range read.literals {
		if number, known := unquotedNumber(r.tree.tokens[literal]); known && refusalStatus(number) && r.statusFitsCode(span, number) {
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
	unique := numbers[:0]
	seen := map[int]bool{}
	for _, literal := range numbers {
		number, _ := unquotedNumber(r.tree.tokens[literal])
		if !seen[number] {
			seen[number] = true
			unique = append(unique, literal)
		}
	}
	numbers = unique
	if len(numbers) == 1 {
		n, _ := unquotedNumber(r.tree.tokens[numbers[0]])
		return n, false
	}
	if len(numbers) > 1 {
		for _, literal := range numbers[1:] {
			n, _ := unquotedNumber(r.tree.tokens[literal])
			r.statusVariants = append(r.statusVariants, n)
		}
		n, _ := unquotedNumber(r.tree.tokens[numbers[0]])
		return n, false
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
