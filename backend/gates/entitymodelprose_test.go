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
//  1. CONTRACT. api/crm.yaml's description for the matching field. Hand-written,
//     reviewed and already gated — the one place in this tree where somebody has
//     said what a field is FOR rather than what it holds.
//  2. DERIVED. What the catalog can prove about THIS column: what it references,
//     and the few CHECK shapes that describe one column on their own.
//  3. HOUSE. The columns almost every table carries. They mean the same thing
//     everywhere, so they are written once here rather than 275 times.
//
// House comes LAST, and only when the column wears the convention's type,
// because it is the one source keyed by name alone — it cannot tell two columns
// apart. Let it win and it overwrites the specific with the generic:
// `consent_text_version.version` is TEXT, the published version of a consent
// document, and the write backbone's counter sentence on it is not vague but
// false. `capture_owner_identity.source` is one of user/provider/delivered_to,
// which is not "which internal channel the record arrived by" either.
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
// houseColumn is a convention: the sentence, and the types the convention wears.
// The types are what stop the name alone speaking for a column that merely
// shares it.
type houseColumn struct {
	dataTypes []string
	sentence  string
}

var houseColumns = map[string]houseColumn{
	"id":             {[]string{"uuid"}, "Primary key."},
	"created_at":     {[]string{timestamptz}, "When the row was created. Set once."},
	"updated_at":     {[]string{timestamptz}, "When the row last changed. Refreshed on every write."},
	"archived_at":    {[]string{timestamptz}, "Soft-delete marker. `NULL` means live, and nearly every read filters on it."},
	"version":        {[]string{"bigint", "integer"}, "Optimistic-concurrency counter. Every write bumps it, so an update built on a stale read is refused instead of overwriting."},
	"captured_by":    {[]string{"text"}, "Who or what wrote the row. Stamped by the server from the authenticated principal, never taken from the request body."},
	"source":         {[]string{"text"}, "Which internal channel the record arrived by."},
	"source_system":  {[]string{"text"}, "The outside system the record came from, when it came from one."},
	"raw":            {[]string{"jsonb"}, "The unparsed upstream payload the row was built from, kept for replay and debugging."},
	"legal_hold":     {[]string{"boolean"}, "True while a hold is preserving this record: no retention sweep touches it, and an erasure against it is refused."},
	"correlation_id": {[]string{"uuid"}, "Ties this row to the one request that produced it."},
	"workspace_id":   {[]string{"uuid"}, "The workspace the row belongs to."},
	"search_tsv":     {[]string{"tsvector"}, "Full-text search vector, maintained by the database."},
}

// timestamptz is spelled out once because Postgres prints it long and four
// entries above would otherwise each carry the same mouthful.
const timestamptz = "timestamp with time zone"

// houseSentence is the convention's sentence for this column, or "" when the
// column only shares a name with it.
func houseSentence(column emColumn) string {
	convention, named := houseColumns[column.name]
	if !named {
		return ""
	}
	for _, dataType := range convention.dataTypes {
		if column.dataType == dataType {
			return convention.sentence
		}
	}
	return ""
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
// makes `FxRate` the shape of a `fx_rate` row. A list response or a create body
// snake-cases to a name no table has, so it drops out without a list here
// saying which ones to skip.
//
// The exception is the write shape that is the ONLY description of a column:
// `WorklistPinRequest` says what a worklist pin's `source` is and nothing named
// `WorklistPin` exists. Those are read in a second pass, so a component that
// names the table outright always wins, and only against a column the table
// actually has — a wrapper whose fields are not that table's columns still
// contributes nothing.
func contractDescriptions(t *testing.T, schema *emSchema) map[string]string {
	t.Helper()
	out := map[string]string{}
	schemas := crmYAMLSchemas(t)
	for _, wrappers := range []bool{false, true} {
		// Components in name order, and the first description of a field wins.
		// Two components can snake-case to one table — a read shape and a
		// summary of the same record — and a map walk would hand the page a
		// different one of them on every run.
		for _, component := range sortedKeys(schemas) {
			table, ok := schema.tables[tableNameFor(component, wrappers)]
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
	}
	return out
}

// columnSentence is what the page prints in a column's last cell.
func columnSentence(schema *emSchema, table *emTable, column emColumn, contract map[string]string) string {
	if column.generated {
		return "Computed by the database. It cannot be written directly."
	}
	if described, ok := contract[table.name+"."+column.name]; ok {
		return described
	}
	if fk, ok := table.singleColumnFKs[column.name]; ok {
		return fmt.Sprintf("Points at `%s.%s`.", fk.parent, fk.parentColumn)
	}
	if derived := fromOwnChecks(table, column); derived != "" {
		return derived
	}
	if house := houseSentence(column); house != "" {
		return house
	}
	return ""
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
			return fmt.Sprintf("Always `%s`: the column exists for the values it may hold later.", m[2])
		}
		if m := checkLength.FindStringSubmatch(check.def); m != nil && m[1] == column.name {
			return fmt.Sprintf("At most %s characters.", m[2])
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

// typeCell is a column's type and, when it has one, its default. The default
// sits here rather than in the description so an undescribed column reads as
// one: its last cell is empty.
func typeCell(column emColumn) string {
	if column.def == "" {
		return "`" + column.dataType + "`"
	}
	return fmt.Sprintf("`%s`, default `%s`", column.dataType, column.def)
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

// wrapperSuffixes name a component that carries a record's fields without being
// the record: the body of a write. They are tried only after every component
// that names a table outright.
var wrapperSuffixes = []string{"Request", "Response", "Input", "Payload", "Body"}

// tableNameFor is the table a component describes. On the wrapper pass a
// component keeping its own name contributes nothing, so a plain `Contact` is
// not read twice.
func tableNameFor(component string, wrappers bool) string {
	if !wrappers {
		return snakeCase(component)
	}
	for _, suffix := range wrapperSuffixes {
		if trimmed := strings.TrimSuffix(component, suffix); trimmed != component {
			return snakeCase(trimmed)
		}
	}
	return ""
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
		case houseSentence(column) != "":
			house = append(house, column)
		default:
			own = append(own, column)
		}
	}
	sort.Slice(own, func(i, j int) bool { return own[i].name < own[j].name })
	sort.Slice(house, func(i, j int) bool { return house[i].name < house[j].name })
	return append(append(key, own...), house...)
}
