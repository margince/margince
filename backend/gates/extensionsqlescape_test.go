// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The constructs a unit's SQL must not spell, because each one lets a statement
// leave the ext_runtime role, or run SQL this gate cannot read.
//
// The runtime role is a guard against mistakes, not a wall. One statement can
// switch back to the login role and run a query string. The check after the
// statement does not see it. A first-party unit never needs either, so the
// lint refuses both where the source spells them. A separate login per unit
// would make it a wall (#628).

import (
	"fmt"
	"regexp"
	"strings"
)

// sqlEscape is one construct and the reason a unit's SQL may not use it.
type sqlEscape struct {
	pattern *regexp.Regexp
	reason  string
}

// statementStart anchors a pattern to the opening of a statement, so a word in
// a sentence is not read as a command.
const statementStart = `(?:^|;)\s*`

// queryStringFunctions are the pg_catalog functions that run a query, or read
// a table, named by a string the gate cannot judge.
var queryStringFunctions = []string{
	"query_to_xml", "query_to_xmlschema", "query_to_xml_and_xmlschema",
	"cursor_to_xml", "cursor_to_xmlschema",
	"table_to_xml", "table_to_xmlschema", "table_to_xml_and_xmlschema",
	"schema_to_xml", "schema_to_xmlschema", "schema_to_xml_and_xmlschema",
	"database_to_xml", "database_to_xmlschema", "database_to_xml_and_xmlschema",
	"ts_stat", "ts_rewrite",
}

var escapeConstructs = []sqlEscape{
	{
		pattern: regexp.MustCompile(statementStart + `(?:set\s+(?:local\s+|session\s+)?(?:role|session\s+authorization)\b|reset\s+(?:role|session\s+authorization)\b|reset\s+all\s*(?:;|$))`),
		reason:  "SQL changes the database role (SET ROLE, RESET ROLE, SET SESSION AUTHORIZATION, RESET ALL)",
	},
	{
		pattern: regexp.MustCompile(statementStart + `do\s+(?:\$|'|e'|language\b)`),
		reason:  "SQL runs a DO block, which is code the database runs and this gate cannot judge",
	},
	{
		pattern: regexp.MustCompile(statementStart + `(?:execute|prepare)\s`),
		reason:  "SQL runs a prepared or dynamic statement (EXECUTE, PREPARE)",
	},
	{
		pattern: regexp.MustCompile(statementStart + `create\s+(?:or\s+replace\s+)?(?:function|procedure)\b`),
		reason:  "SQL creates a function or a procedure, which runs code this gate cannot judge",
	},
	{
		pattern: regexp.MustCompile(`\b(?:pg_catalog\.)?(?:` + strings.Join(queryStringFunctions, "|") + `)\s*\(`),
		reason:  "SQL calls a function that runs a query named by a string",
	},
}

// setConfigCall finds set_config and what its first argument opens with.
var setConfigCall = regexp.MustCompile(`\b(?:pg_catalog\.)?set_config\s*\(\s*('[^']*'|\S?)`)

// sqlEscapes returns the reason for each construct text spells. It reads the
// raw folded string, literals included, because set_config names its setting
// in one.
func sqlEscapes(text string) []string {
	normalised := strings.ToLower(strings.Join(strings.Fields(sqlComments.ReplaceAllString(text, " ")), " "))
	var out []string
	for _, escape := range escapeConstructs {
		if escape.pattern.MatchString(normalised) {
			out = append(out, escape.reason+": a unit's SQL runs as ext_runtime on its own tables and has no use for it")
		}
	}
	for _, call := range setConfigCall.FindAllStringSubmatch(normalised, -1) {
		switch setting := strings.Trim(call[1], "'"); {
		case !strings.HasPrefix(call[1], "'"):
			out = append(out, "set_config names its setting through something this gate cannot read: spell the setting as a literal, so the SQL says it does not change the role")
		case setting == "role" || setting == "session_authorization":
			out = append(out, fmt.Sprintf("set_config(%s, …) changes the database role: a unit's SQL runs as ext_runtime and must not leave it", call[1]))
		}
	}
	return out
}
