// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

// What a lead is called decides two things at once: whether the promotion will
// take it at all, and which person the ladder then matches. The cases that
// matter are the ones where "has a name" and "has a full_name" part company —
// a pointer to an empty string is a full_name and is not a name.
func TestLeadIdentityName(t *testing.T) {
	email := func(s string) *openapi_types.Email {
		e := openapi_types.Email(s)
		return &e
	}
	for _, tc := range []struct {
		name string
		lead crmcontracts.Lead
		want string
	}{
		{
			name: "a name is the name",
			lead: crmcontracts.Lead{FullName: ptr("Vera Lindqvist")},
			want: "Vera Lindqvist",
		},
		{
			name: "the email names a lead that has no name of its own",
			lead: crmcontracts.Lead{Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "a name wins over an email",
			lead: crmcontracts.Lead{FullName: ptr("Vera Lindqvist"), Email: email("vera@nordwind.example")},
			want: "Vera Lindqvist",
		},
		{
			name: "a present-but-empty full_name is not a name",
			lead: crmcontracts.Lead{FullName: ptr(""), Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "padding is not a name either",
			lead: crmcontracts.Lead{FullName: ptr("   "), Email: email("vera@nordwind.example")},
			want: "vera@nordwind.example",
		},
		{
			name: "a name keeps no padding into the ladder",
			lead: crmcontracts.Lead{FullName: ptr("  Vera Lindqvist  ")},
			want: "Vera Lindqvist",
		},
		{
			name: "nothing names a lead with neither",
			lead: crmcontracts.Lead{},
			want: "",
		},
		{
			// The case the promotion guard has to refuse: a full_name that
			// exists, names nobody, and has no address standing behind it.
			name: "an empty full_name and no email names nobody",
			lead: crmcontracts.Lead{FullName: ptr("")},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := leadIdentityName(tc.lead); got != tc.want {
				t.Errorf("leadIdentityName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// The rule has a second spelling that no AST gate can see. A query naming a
// lead falls back the same way in SQL, and there `COALESCE(full_name, email)`
// is the exact defect `FullName != nil` was in Go: it answers the empty string
// for a full_name that is present and empty, so one surface calls the lead
// nothing while the promotion of the same lead calls it by its address.
//
// The census reads every string literal in the module rather than the ones
// that look like queries — deciding what looks like a query is where a census
// goes blind — and then decides on ONE coalesce's own argument list. A literal
// merely mentioning both columns is not this rule: a SELECT naming the name and
// the address as separate columns, an INSERT listing both, and a coalesce over
// a domain extracted from the address all do that, and none of them falls back.
func TestEverySQLFallbackToAnEmailGuardsTheEmptyName(t *testing.T) {
	const (
		nameColumn  = "full_name"
		emailColumn = "email"
		// The guard that makes the SQL agree with leadIdentityName: an empty
		// name, once trimmed, is NULL, so the fallback proceeds past it.
		guard = "nullif(btrim(full_name), '')"
	)
	fallbacks := 0
	for _, lit := range moduleSQLLiterals(t) {
		for _, args := range coalesceArguments(lit.text) {
			// The fallback is name THEN address, in one argument list. The
			// other order is a different rule and this gate does not hold it.
			name := strings.Index(args, nameColumn)
			if name < 0 || !strings.Contains(args[name:], emailColumn) {
				continue
			}
			fallbacks++
			if strings.Contains(args, guard) {
				continue
			}
			t.Errorf("a query at %s falls back from %s to %s as coalesce(%s) without %s.\n\n"+
				"COALESCE stops at a present-but-empty name and answers \"\", so this surface "+
				"calls a lead nothing while leadIdentityName calls the same lead by its "+
				"address. Guard the name.",
				lit.where, nameColumn, emailColumn, strings.Join(strings.Fields(args), " "), guard)
		}
	}
	if fallbacks == 0 {
		t.Fatalf("no query in this module falls back from %s to %s, so this gate judged nothing "+
			"— the rule has moved out of SQL and this test no longer describes the module",
			nameColumn, emailColumn)
	}
}

// coalesceArguments returns the argument text of every coalesce(...) in one SQL
// literal, lowercased, with line comments stripped first — a `--` can carry an
// unbalanced parenthesis, and counting it would end the argument list early and
// hide what follows.
func coalesceArguments(sql string) []string {
	var stripped strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if cut := strings.Index(line, "--"); cut >= 0 {
			line = line[:cut]
		}
		stripped.WriteString(line)
		stripped.WriteByte('\n')
	}
	text := strings.ToLower(stripped.String())

	var args []string
	for at := 0; ; {
		open := strings.Index(text[at:], "coalesce(")
		if open < 0 {
			return args
		}
		open += at + len("coalesce(")
		depth, end := 1, open
		for ; end < len(text) && depth > 0; end++ {
			switch text[end] {
			case '(':
				depth++
			case ')':
				depth--
			}
		}
		if depth != 0 {
			// Unbalanced: the literal is a fragment concatenated with another.
			// Judging half an argument list would answer about a call that is
			// not there, so take what there is and say nothing about the rest.
			args = append(args, text[open:])
			return args
		}
		args = append(args, text[open:end-1])
		at = end
	}
}
