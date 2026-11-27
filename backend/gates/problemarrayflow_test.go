// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"fmt"
	"testing"
)

type problemMockFlags struct{ own, field bool }

func (r *problemFileReader) readPassedFieldArrays() {
	tr := r.tree
	for open := range tr.tokens {
		if !tr.is(open, "{") {
			continue
		}
		problem := tr.parent[open]
		if problem < 0 || !tr.is(problem, "{") || !r.holdsMember(problem, "details", open) {
			continue
		}
		context := r.problemContext(problem, map[int]bool{})
		if !context.refusal {
			continue
		}
		for _, entry := range tr.entries(open) {
			if entry.key == "errors" && !entry.spread {
				r.readFieldArray(entry.value, context.code, entry.keyAt, 0)
			}
		}
	}
}

func (r *problemFileReader) readFieldArray(span tsSpan, status, at, depth int) bool {
	if depth > 10 || span.to <= span.from {
		return false
	}
	tr := r.tree
	if chain, ok := memberChain(tr, span); ok {
		if fn, index, parameter := r.paramOf(span.from, chain[0]); parameter && fn.name != "" {
			rest := tr.is(fn.params[index].from-1, "...")
			return r.registerHelper(fn.name, codeSource{index: index, fixedStatus: status, field: true, array: true, rest: rest, path: chain[1:]})
		}
	}
	if open, known := r.arrayAt(span); known {
		for _, element := range tr.split(open) {
			if tr.is(element.from, "...") {
				r.readFieldArray(tsSpan{element.from + 1, element.to}, status, at, depth+1)
				continue
			}
			r.readFieldArrayElement(element, status, at)
		}
		return false
	}
	if tr.is(span.from, "{") {
		r.readFieldArrayElement(span, status, at)
		return false
	}
	if span.to == span.from+1 && (tr.tokens[span.from].kind == tkString || tr.word(span.from) == "null" || tr.word(span.from) == "undefined") {
		return false
	}
	// An unreadable array is a census finding, even when its body code is readable.
	r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(at), field: true, computed: true, status: status})
	return false
}

func (r *problemFileReader) readFieldArrayElement(span tsSpan, status, at int) {
	read, known := r.resolve(span, []string{"code"}, 0)
	if !known {
		r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(at), field: true, computed: true, status: status})
		return
	}
	if read.helper != "" {
		read.source.field, read.source.fixedStatus = true, status
		r.registerHelper(read.helper, read.source)
		return
	}
	for _, literal := range read.literals {
		r.addFieldMocks([]int{literal}, status, literal, problemMockFlags{field: true})
	}
}

func (r *problemFileReader) readFieldArrayCall(call int, args []tsSpan, source codeSource) bool {
	if source.index >= len(args) {
		return false
	}
	changed := false
	end := source.index + 1
	if source.rest {
		end = len(args)
	}
	for _, span := range args[source.index:end] {
		if r.tree.is(span.from, "...") {
			span.from++
		}
		changed = r.readFieldArrayPath(span, source.path, source.fixedStatus, call, 0) || changed
	}
	return changed
}

func (r *problemFileReader) transportOfCall(call int) statusRead {
	tr := r.tree
	slot := tsSpan{call, tr.match[call+1] + 1}
	if tr.is(call-1, ".") {
		slot.from = call - 2
	}
	for parent := tr.parent[call]; parent >= 0; parent = tr.parent[parent] {
		if tr.is(parent, "(") {
			return r.sentAsArgument(parent, slot)
		}
		if tr.is(parent, "{") {
			return r.sentAsValue(parent, slot, map[int]bool{})
		}
	}
	return statusRead{}
}

func (r *problemFileReader) statusMismatch(at, body, transport int) {
	r.scan.faults = append(r.scan.faults, fmt.Sprintf("%s:%d: Problem body status %d differs from HTTP status %d", r.path, r.line(at), body, transport))
}

func (r *problemFileReader) checkBodyStatus(open int, transport statusRead) {
	own := r.ownStatus(open, map[int]bool{})
	if own.code > 0 && transport.code > 0 && own.code != transport.code {
		r.statusMismatch(open, own.code, transport.code)
	}
}

func TestEveryProblemMockBodyAgreesWithItsTransport(t *testing.T) {
	t.Parallel()
	_, scans := frontendProblemMocks(t)
	for _, scan := range scans {
		for _, fault := range scan.faults {
			t.Error(fault)
		}
	}
}

func TestProblemArraysFollowParametersRestAndSpread(t *testing.T) {
	for name, source := range map[string]string{
		"parameter": `function refuse(errors:Fault[]){return {status:422,code:"validation_error",details:{errors}}} refuse([{field:"name",code:"too_long"}]);`,
		"rest":      `function refuse(...faults:Fault[]){return {status:422,code:"validation_error",details:{errors:faults}}} refuse({field:"name",code:"too_long"});`,
		"spread":    `function refuse(errors:Fault[]){return {status:422,code:"validation_error",details:{errors}}} const first=[{field:"name",code:"too_long"}]; refuse([...first]);`,
	} {
		t.Run(name, func(t *testing.T) {
			tree, err := parseTS(source, name+".test.ts")
			if err != nil {
				t.Fatal(err)
			}
			scan := scanProblemFile(name+".test.ts", source, tree, map[string]bool{})
			for _, mock := range scan.mocks {
				if mock.field && mock.code == "too_long" {
					return
				}
			}
			t.Fatalf("passed array lost its field code: %+v", scan.mocks)
		})
	}
}

func TestProblemBodyStatusMismatchIsFoundThroughHelpers(t *testing.T) {
	for _, source := range []string{
		`Response.json({status:404,code:"unauthorized"},{status:401});`,
		`function refused(code:string){return {status:404,code}} Response.json(refused("unauthorized"),{status:401});`,
	} {
		tree, err := parseTS(source, "status.test.ts")
		if err != nil {
			t.Fatal(err)
		}
		scan := scanProblemFile("status.test.ts", source, tree, map[string]bool{})
		if len(scan.faults) == 0 {
			t.Fatalf("mismatched status escaped: %+v", scan.mocks)
		}
	}
}

func (r *problemFileReader) readFieldArrayPath(span tsSpan, path []string, status, at, depth int) bool {
	if len(path) == 0 {
		return r.readFieldArray(span, status, at, depth)
	}
	if depth > 10 {
		return false
	}
	tr := r.tree
	if tr.is(span.from, "{") {
		for _, entry := range tr.entries(span.from) {
			if entry.key == path[0] {
				return r.readFieldArrayPath(entry.value, path[1:], status, at, depth+1)
			}
		}
		return false
	}
	if chain, ok := memberChain(tr, span); ok {
		if fn, index, parameter := r.paramOf(span.from, chain[0]); parameter && fn.name != "" {
			return r.registerHelper(fn.name, codeSource{index: index, fixedStatus: status, field: true, array: true, path: append(chain[1:], path...)})
		}
		if open, known := r.objects[chain[0]]; known {
			return r.readFieldArrayPath(tsSpan{open, tr.match[open] + 1}, append(chain[1:], path...), status, at, depth+1)
		}
	}
	r.scan.mocks = append(r.scan.mocks, problemMock{path: r.path, line: r.line(at), field: true, computed: true, status: status})
	return false
}
