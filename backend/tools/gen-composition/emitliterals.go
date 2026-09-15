// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Rendering the composed verb, schema and job set into Go source literals.
// Split from emit.go, which owns the surrounding file/module assembly; these
// answer one question — how a declared value becomes emitted Go — for the
// three shapes that need it.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// tsString renders a Go string as a TypeScript string literal through
// encoding/json rather than strconv.Quote: JSON's escaping is a strict subset
// of what a TS double-quoted literal accepts (and it escapes <, > and & as
// \uXXXX), whereas Go's own quoting has forms — raw non-ASCII runes carried
// through verbatim — that would put a unit author's prose into the emitted
// source unescaped. A string that cannot be marshalled does not exist: every
// input here has already round-tripped through the merged YAML document.
func tsString(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		panic("gen-composition: a string from the merged contract is not JSON-encodable: " + err.Error())
	}
	return string(encoded)
}

// writeVerbLiterals emits the composed verb set as a Go slice literal. Fields
// are written unconditionally rather than omitted when zero: the emitted
// source is what a reviewer reads to see what an installation publishes, and a
// missing line would have to be read as "absent" or as "the zero value" with
// nothing on the page to say which.
func writeVerbLiterals(b *strings.Builder, verbs []declaredVerb) {
	if len(verbs) == 0 {
		b.WriteString("\treturn nil\n}\n")
		return
	}
	b.WriteString("\treturn []extension.Verb{\n")
	for _, d := range verbs {
		v := d.verb
		b.WriteString("\t\t{\n")
		fmt.Fprintf(b, "\t\t\tUnit:           %q,\n", string(v.Unit))
		fmt.Fprintf(b, "\t\t\tContract:       %q,\n", v.Contract)
		fmt.Fprintf(b, "\t\t\tOperationID:    %q,\n", v.OperationID)
		fmt.Fprintf(b, "\t\t\tRoute:          %q,\n", v.Route)
		fmt.Fprintf(b, "\t\t\tMethod:         %q,\n", v.Method)
		fmt.Fprintf(b, "\t\t\tTool:           %q,\n", v.Tool)
		fmt.Fprintf(b, "\t\t\tTitle:          %q,\n", v.Title)
		fmt.Fprintf(b, "\t\t\tDescription:    %q,\n", v.Description)
		fmt.Fprintf(b, "\t\t\tVersion:        %q,\n", v.Version)
		fmt.Fprintf(b, "\t\t\tTier:           %q,\n", string(v.Tier))
		fmt.Fprintf(b, "\t\t\tRequestedScope: %q,\n", string(v.RequestedScope))
		writeSchemaLiteral(b, "InputSchema", v.InputSchema)
		writeSchemaLiteral(b, "OutputSchema", v.OutputSchema)
		fmt.Fprintf(b, "\t\t\tRbacObject:     %q,\n", v.RbacObject)
		if !v.Subject.IsZero() {
			// Emitted only when declared, so the generated literal reads the
			// way the fragment does: a tier that never stages carries no
			// subject, and a zero struct written out would look like one that
			// was declared empty.
			fmt.Fprintf(b, "\t\t\tSubject:        extension.Subject{Arg: %q, Table: %q},\n", v.Subject.Arg, v.Subject.Table)
		}
		fmt.Fprintf(b, "\t\t\tRbacAction:     %q,\n", string(v.RbacAction))
		if v.HumanOnly {
			// Emitted only when true, matching Subject's own convention just
			// above: the zero value is every verb as it behaves today, and a
			// literal `false` on every one of them would bury the one field
			// that actually says something.
			b.WriteString("\t\t\tHumanOnly:      true,\n")
		}
		b.WriteString("\t\t},\n")
	}
	b.WriteString("\t}\n}\n")
}

// writeSchemaLiteral emits a json.RawMessage field. A nil schema is emitted as
// `nil`, not as an empty literal: the served adapter defaults a missing input
// schema to `{"type":"object"}`, and `json.RawMessage("")` is not that — it is
// a value every JSON decoder rejects.
func writeSchemaLiteral(b *strings.Builder, field string, raw json.RawMessage) {
	if raw == nil {
		fmt.Fprintf(b, "\t\t\t%s:%s nil,\n", field, strings.Repeat(" ", len("RequestedScope")-len(field)))
		return
	}
	fmt.Fprintf(b, "\t\t\t%s:%s json.RawMessage(%q),\n", field, strings.Repeat(" ", len("RequestedScope")-len(field)), string(raw))
}

// writeJobLiterals emits the composed job set as a Go slice literal.
//
// Durations go through mustDuration carrying the contract's own spelling, so
// the emitted file reads `mustDuration("6h0m0s")` rather than a nanosecond
// count. See extensionsGenDurationHelper.
//
// Fields are written unconditionally rather than omitted when zero, for the
// reason the verb literals are: the emitted source is what a reviewer reads to
// see what an installation schedules, and a missing line would have to be read
// as "absent" or as "the zero value" with nothing on the page to say which.
func writeJobLiterals(b *strings.Builder, decls []extension.JobDeclaration) {
	if len(decls) == 0 {
		b.WriteString("\treturn nil\n}\n")
		return
	}
	b.WriteString("\treturn []extension.JobDeclaration{\n")
	for _, d := range decls {
		b.WriteString("\t\t{\n")
		fmt.Fprintf(b, "\t\t\tUnit:              %q,\n", string(d.Unit))
		fmt.Fprintf(b, "\t\t\tJob:               %q,\n", d.Job)
		fmt.Fprintf(b, "\t\t\tQueue:             %q,\n", d.Queue)
		fmt.Fprintf(b, "\t\t\tCadence:           mustDuration(%q),\n", d.Cadence.String())
		fmt.Fprintf(b, "\t\t\tDispatcherTimeout: mustDuration(%q),\n", d.DispatcherTimeout.String())
		fmt.Fprintf(b, "\t\t\tTimeout:           mustDuration(%q),\n", d.Timeout.String())
		fmt.Fprintf(b, "\t\t\tMaxAttempts:       %d,\n", d.MaxAttempts)
		fmt.Fprintf(b, "\t\t\tTier:              %q,\n", string(d.Tier))
		fmt.Fprintf(b, "\t\t\tRequestedScope:    %q,\n", string(d.RequestedScope))
		b.WriteString("\t\t},\n")
	}
	b.WriteString("\t}\n}\n")
}

// anySchema reports whether the emitted literals will mention
// json.RawMessage, which decides whether the generated file imports
// encoding/json at all.
func anySchema(verbs []declaredVerb) bool {
	for _, d := range verbs {
		if d.verb.InputSchema != nil || d.verb.OutputSchema != nil {
			return true
		}
	}
	return false
}
