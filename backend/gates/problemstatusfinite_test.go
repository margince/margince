// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "strings"

func (r *problemFileReader) finiteStatus(span tsSpan, path []string, depth int) (codeRead, bool) {
	if depth > 10 {
		return codeRead{}, false
	}
	read, known := r.resolve(span, path, 0)
	if !known || read.helper == "" {
		return read, known
	}
	var result codeRead
	found := false
	tr := r.tree
	for at := range tr.tokens {
		if tr.word(at) != read.helper || !tr.is(at+1, "(") || tr.word(at-1) == "function" || tr.is(at-1, ".") {
			continue
		}
		args := tr.split(at + 1)
		var argument tsSpan
		if read.source.index < len(args) {
			argument = args[read.source.index]
		} else {
			for _, fn := range r.scan.funcs {
				if fn.name == read.helper {
					argument = fn.defaults[read.source.index]
					break
				}
			}
			if argument.to <= argument.from {
				continue
			}
		}
		next, ok := r.finiteStatus(argument, read.source.path, depth+1)
		if !ok {
			return codeRead{}, false
		}
		result.literals = append(result.literals, next.literals...)
		found = true
	}
	return result, found
}

func (r *problemFileReader) statusFitsCode(span tsSpan, number int) bool {
	key := strings.TrimSpace(r.flowConditionKey(span))
	for condition, yes := range r.codeConditions() {
		fields := strings.Fields(condition)
		for at, field := range fields {
			if field != "===" && field != "==" && field != "!==" && field != "!=" {
				continue
			}
			if strings.Join(fields[:at], " ") != key || at+2 != len(fields) {
				continue
			}
			// The compared constant is read from this same tree, never a server-status table.
			var wanted int
			for _, token := range r.tree.tokens {
				if token.text == fields[at+1] {
					if n, ok := unquotedNumber(token); ok {
						wanted = n
						break
					}
				}
			}
			if wanted == 0 {
				continue
			}
			matches := number == wanted
			if field == "!==" || field == "!=" {
				matches = !matches
			}
			if matches != yes {
				return false
			}
		}
	}
	return true
}

func (r *problemFileReader) nestedCodeConditions() map[string]bool {
	out := map[string]bool{}
	tr := r.tree
	code := r.flowCode
	if code < 0 {
		return out
	}
	for question := range tr.tokens {
		if !tr.is(question, "?") {
			continue
		}
		parent := tr.parent[question]
		from := question - 1
		for from > 0 {
			prior := from - 1
			if prior == parent || tr.is(prior, ",") || tr.is(prior, ":") || tr.is(prior, "=") || tr.word(prior) == "return" || tr.is(prior, ";") {
				break
			}
			if tr.match[prior] >= 0 && tr.match[prior] < prior {
				from = tr.match[prior]
				continue
			}
			if tr.parent[prior] != parent {
				break
			}
			from--
		}
		span := r.expressionEnd(from)
		if code < span.from || code >= span.to {
			continue
		}
		yes, no, ok := conditionalArms(tr, span)
		if !ok {
			continue
		}
		key := r.flowConditionKey(tsSpan{span.from, yes.from - 1})
		if code >= yes.from && code < yes.to {
			out[key] = true
		}
		if code >= no.from && code < no.to {
			out[key] = false
		}
	}
	return out
}

func (r *problemFileReader) keepStatusVariants(before int) {
	end := len(r.scan.mocks)
	for _, status := range r.statusVariants {
		for _, mock := range r.scan.mocks[before:end] {
			mock.status = status
			r.scan.mocks = append(r.scan.mocks, mock)
		}
	}
	r.statusVariants = nil
}

// callStatus judges the status a call passes a helper. One it cannot follow
// may be a refusal, so its code is judged.
func (r *problemFileReader) callStatus(args []tsSpan, param *codeSource) (status int, refusal bool, passed *codeRead) {
	if param == nil || param.index >= len(args) {
		return 0, true, nil
	}
	if _, _, fallback := fallbackSides(r.tree, args[param.index]); fallback {
		return 0, true, nil
	}
	read, ok := r.resolve(args[param.index], param.path, 0)
	switch {
	case ok && read.helper != "":
		return 0, true, &read
	case ok && len(read.literals) > 0:
		status, refusal = refusalOf(r.tree, read.literals)
		return status, refusal, nil
	}
	return 0, true, nil
}
