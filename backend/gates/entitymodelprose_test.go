// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// The words the entity model puts beside a column, and where each one comes from.
//
// Nothing here is prose kept for the page. Every sentence is derived from a
// committed, gated source — the contract's own description, the catalog's type
// and rules, or the house conventions below — because a page carrying its own
// hand-written column notes is a second description of the schema, and the two
// drift the first week nobody rereads them.
//
// Three sources, in falling order of authority:
//
//  1. HOUSE. The columns almost every table carries. They mean the same thing
//     everywhere, so they are written once here rather than 268 times in the
//     contract, and a reader who has learnt `version` on one page has learnt it
//     on all of them.
//  2. CONTRACT. api/crm.yaml's description for the matching field. Hand-written,
//     reviewed and already gated — the one place in this tree where somebody has
//     said what a field is FOR rather than what it holds.
//  3. DERIVED. What the catalog can prove: the type, whether it is required,
//     what it references, and the few CHECK shapes that describe one column on
//     their own.
//
// A CHECK only reaches a column's sentence when it mentions that column and no
// other. The rest are real conditional logic — "this is set only when that is" —
// and a paraphrase of one would be this file inventing a claim the schema does
// not make. Those are printed verbatim in the table's own rules section, where
// an engineer can read the condition instead of trusting a summary of it.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// houseColumns are the conventions the write backbone puts on nearly every
// table. Keyed by column name alone: that is what makes them conventions.
// gatekit:fixture the sentence each house column gets on every page
var houseColumns = map[string]string{
	"id":             "Primary key.",
	"created_at":     "When the row was created. Set once.",
	"updated_at":     "When the row last changed. Refreshed on every write.",
	"archived_at":    "Soft-delete marker. `NULL` means live, and nearly every read filters on it.",
	"version":        "Optimistic-concurrency counter. Every write bumps it, so an update built on a stale read is refused instead of overwriting.",
	"captured_by":    "Who or what wrote the row. Stamped by the server from the authenticated principal, never taken from the request body.",
	"source":         "Which internal channel the record arrived by.",
	"source_system":  "The outside system the record came from, when it came from one.",
	"raw":            "The unparsed upstream payload the row was built from, kept for replay and debugging.",
	"legal_hold":     "True while a hold is preserving this record: no retention sweep touches it, and an erasure against it is refused.",
	"correlation_id": "Ties this row to the one request that produced it.",
	"workspace_id":   "The workspace the row belongs to.",
	"search_tsv":     "Full-text search vector, maintained by the database.",
}

// The CHECK shapes a single column's sentence can be built from. Each is
// anchored on the whole definition so a fragment of a larger condition cannot
// match: a condition that happens to CONTAIN an enum test is not an enum.
var (
	checkEnum = regexp.MustCompile(
		`^CHECK \(\(?\(?[a-z0-9_]+ = ANY \(ARRAY\[(.*?)\]\)\)?\)?\)$`)
	checkNullableEnum = regexp.MustCompile(
		`^CHECK \(\(\([a-z0-9_]+ IS NULL\) OR \([a-z0-9_]+ = ANY \(ARRAY\[(.*?)\]\)\)\)\)$`)
	checkConstant = regexp.MustCompile(
		`^CHECK \(\(([a-z0-9_]+) = '([^']*)'::text\)\)$`)
	checkLength = regexp.MustCompile(
		`(?:char_length|length)\(([a-z0-9_]+)\) <= ([0-9]+)`)
	checkEnumValue = regexp.MustCompile(`'([^']*)'::[a-z ]+`)
	// wordBoundary finds a column named as an identifier rather than as part of
	// a longer one, which is what keeps `note` out of `note_author`'s rules.
	identifierChar = regexp.MustCompile(`[A-Za-z0-9_]`)
)

// enumsShown bounds an enum list. Past a handful the values stop telling a
// reader anything the constraint name did not, and the row gets unreadable.
const enumsShown = 6

// contractDescriptions maps `table.column` to the contract's description of the
// matching field.
//
// The component name IS the table name, snake-cased: the contract-first rule
// makes `FxRate` the shape of a `fx_rate` row. Wrappers — a request body, a list
// response — snake-case to a name no table has, so they drop out without a list
// here saying which ones to skip.
func contractDescriptions(t *testing.T, schema *emSchema) map[string]string {
	t.Helper()
	out := map[string]string{}
	schemas := crmYAMLSchemas(t)
	// Components in name order, and the first description of a field wins.
	// Two components can snake-case to one table — a read shape and a summary
	// of the same record — and a map walk would hand the page a different one
	// of them on every run.
	for _, component := range sortedKeys(schemas) {
		table, ok := schema.tables[snakeCase(component)]
		if !ok {
			continue
		}
		for _, property := range sortedKeys(schemas[component].Properties) {
			described := strings.TrimSpace(schemas[component].Properties[property].Description)
			key := table.name + "." + property
			if described == "" || out[key] != "" || !tableHasColumn(table, property) {
				continue
			}
			out[key] = firstSentence(described)
		}
	}
	return out
}

// columnSentence is what the page prints in a column's last cell.
func columnSentence(schema *emSchema, table *emTable, column emColumn, contract map[string]string) string {
	if house, ok := houseColumns[column.name]; ok {
		return house
	}
	if column.generated {
		return "Computed by the database. It cannot be written directly."
	}
	if described, ok := contract[table.name+"."+column.name]; ok {
		return described
	}
	if fk, ok := table.singleColumnFKs[column.name]; ok {
		return fmt.Sprintf("Points at `%s.%s` — %s.", fk.parent, fk.parentColumn, onDeleteMeaning(fk.onDelete))
	}
	if derived := fromOwnChecks(table, column); derived != "" {
		return derived
	}
	return plainSentence(column)
}

// fromOwnChecks reads the CHECKs that describe this column and no other.
func fromOwnChecks(table *emTable, column emColumn) string {
	for _, check := range table.checks {
		if !mentionsOnly(check.def, column.name, table) {
			continue
		}
		if values := enumValues(check.def); len(values) > 0 {
			return "One of " + joinEnum(values) + "."
		}
		if m := checkConstant.FindStringSubmatch(check.def); m != nil && m[1] == column.name {
			return fmt.Sprintf("Always `%s` — the column exists for the values it may hold later.", m[2])
		}
		if m := checkLength.FindStringSubmatch(check.def); m != nil && m[1] == column.name {
			return fmt.Sprintf("%s `%s`, at most %s characters.", requiredWord(column), column.dataType, m[2])
		}
	}
	return ""
}

func enumValues(def string) []string {
	m := checkEnum.FindStringSubmatch(def)
	if m == nil {
		m = checkNullableEnum.FindStringSubmatch(def)
	}
	if m == nil {
		return nil
	}
	var values []string
	for _, value := range checkEnumValue.FindAllStringSubmatch(m[1], -1) {
		values = append(values, value[1])
	}
	return values
}

func joinEnum(values []string) string {
	shown := values
	if len(shown) > enumsShown {
		shown = shown[:enumsShown]
	}
	quoted := make([]string, 0, len(shown))
	for _, value := range shown {
		quoted = append(quoted, "`"+value+"`")
	}
	joined := strings.Join(quoted, ", ")
	if len(values) > enumsShown {
		return fmt.Sprintf("%s and %d more", joined, len(values)-enumsShown)
	}
	return joined
}

// mentionsOnly reports whether a definition names this column of the table and
// none of its siblings.
func mentionsOnly(def, column string, table *emTable) bool {
	if !namesIdentifier(def, column) {
		return false
	}
	for _, other := range table.columns {
		if other.name != column && namesIdentifier(def, other.name) {
			return false
		}
	}
	return true
}

// namesIdentifier reports whether name appears in def as a whole identifier.
func namesIdentifier(def, name string) bool {
	for offset := 0; ; {
		at := strings.Index(def[offset:], name)
		if at < 0 {
			return false
		}
		at += offset
		before := at == 0 || !identifierChar.MatchString(string(def[at-1]))
		end := at + len(name)
		after := end == len(def) || !identifierChar.MatchString(string(def[end]))
		if before && after {
			return true
		}
		offset = at + 1
	}
}

func plainSentence(column emColumn) string {
	sentence := fmt.Sprintf("%s `%s`.", requiredWord(column), column.dataType)
	if column.def != "" {
		sentence = strings.TrimSuffix(sentence, ".") + fmt.Sprintf(", defaulting to `%s`.", column.def)
	}
	return sentence
}

func requiredWord(column emColumn) string {
	if column.notNull {
		return "Required"
	}
	return "Optional"
}

func onDeleteMeaning(action string) string {
	switch action {
	case "CASCADE":
		return "deleting the parent deletes this row"
	case "SET NULL":
		return "deleting the parent keeps this row and clears the link"
	case "SET DEFAULT":
		return "deleting the parent keeps this row and resets the link"
	default:
		return "the parent cannot be deleted while this row points at it"
	}
}

func tableHasColumn(table *emTable, name string) bool {
	for _, column := range table.columns {
		if column.name == name {
			return true
		}
	}
	return false
}

// snakeCase turns a contract component name into the table name it describes.
func snakeCase(name string) string {
	var out strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(r - 'A' + 'a')
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// sortedColumns is the order a page prints a table's columns in.
//
// The catalog is SORTED, so the order a migration declared the columns in is
// not in the file and cannot be recovered. Alphabetical would open every table
// with `archived_at`, so the order is chosen instead: the key, then the columns
// this table is about, then the house trailer every table shares. A reader
// comparing two tables then finds the part that differs at the top.
func sortedColumns(table *emTable) []emColumn {
	var key, own, house []emColumn
	for _, column := range table.columns {
		switch {
		case column.name == "id":
			key = append(key, column)
		case houseColumns[column.name] != "":
			house = append(house, column)
		default:
			own = append(own, column)
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].name < own[j].name })
	sort.Slice(house, func(i, j int) bool { return house[i].name < house[j].name })
	return append(append(key, own...), house...)
}
