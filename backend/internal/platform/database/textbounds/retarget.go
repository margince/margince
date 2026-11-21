// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textbounds

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"regexp"
	"strings"
)

var catalogGuardBounds = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*\.text_bound_(?:insert|update)) .* EXECUTE FUNCTION refuse_oversized_text\('(\{[^']*\})'\)$`)

// catalogGuard matches a text guard as the catalog prints it: the table, then
// the definition after CREATE TRIGGER.
var catalogGuard = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.text_bound_(?:insert|update) CREATE TRIGGER (.*)$`)

// unqualifiedGuardFunction finds the shared functions in a printed definition,
// which Postgres prints without their schema.
var unqualifiedGuardFunction = regexp.MustCompile(`\b(text_row_exceeds_bounds|text_values_exceed_bound|refuse_oversized_text)\(`)

// Retarget builds the migration that moves the named tables' guards from the
// bounds the catalog holds now to the bounds columns.txt states. Its rollback
// puts the catalog's bounds back, so a policy change stays reversible. Like the
// table guards, it runs unwrapped, so each table is locked only for its own guard.
func Retarget(catalog io.Reader, tables []string) (Migration, error) {
	raw, err := io.ReadAll(catalog)
	if err != nil {
		return Migration{}, fmt.Errorf("read text columns: %w", err)
	}
	current, err := GuardBounds(string(raw))
	if err != nil {
		return Migration{}, err
	}
	wanted, err := PolicyBounds(string(raw))
	if err != nil {
		return Migration{}, err
	}
	columns, err := Columns(strings.NewReader(string(raw)))
	if err != nil {
		return Migration{}, err
	}
	var up, down strings.Builder
	for _, table := range tables {
		before, after := current[table+".text_bound_insert"], wanted[table]
		if maps.Equal(before, after) {
			return Migration{}, fmt.Errorf("the guards of %s already match columns.txt", table)
		}
		replaceGuards(&up, table, TableBounds(table, after, columns))
		if NamesColumns(table) {
			restorePrintedGuards(&down, table, string(raw))
			continue
		}
		replaceGuards(&down, table, TableBounds(table, before, columns))
	}
	return Migration{
		Up:   unwrappedHeader + up.String() + "RESET lock_timeout;\n",
		Down: unwrappedHeader + down.String() + "RESET lock_timeout;\n",
	}, nil
}

// replaceGuards sets a table's guards to bounds, or removes them when no
// column of the table is bounded.
func replaceGuards(out *strings.Builder, table string, bounds []Bound) {
	var unused strings.Builder
	if len(bounds) == 0 {
		writeGuardPair(&unused, out, table, nil)
		return
	}
	writeGuardPair(out, &unused, table, bounds)
}

// restorePrintedGuards puts back a table's guards as the catalog prints them,
// with no rebuilt condition. After a rename Postgres has already moved the condition to the new
// name, while the refusal still lists the old one. So a condition built again
// from the refusal's list would lose that column's check.
func restorePrintedGuards(out *strings.Builder, table, catalog string) {
	restored := false
	for line := range strings.SplitSeq(catalog, "\n") {
		if match := catalogGuard.FindStringSubmatch(line); match != nil && match[1] == table {
			fmt.Fprintf(out, "CREATE OR REPLACE TRIGGER %s;\n", unqualifiedGuardFunction.ReplaceAllString(match[2], "public.$1("))
			restored = true
		}
	}
	if !restored {
		replaceGuards(out, table, nil)
	}
}

// TableBounds gives each limit the shape of its column in the catalog.
func TableBounds(table string, limits map[string]int, columns []Column) []Bound {
	shapes := map[string]Column{}
	for _, column := range columns {
		if column.Table == table {
			shapes[column.Name] = column
		}
	}
	bounds := make([]Bound, 0, len(limits))
	for name, limit := range limits {
		bounds = append(bounds, Bound{Column: name, Limit: limit, Array: shapes[name].Array})
	}
	sortBounds(bounds)
	return bounds
}

// GuardBounds reads the bounds of every text guard in a catalog, keyed by
// table and trigger name, such as "contact.text_bound_insert".
func GuardBounds(catalog string) (map[string]map[string]int, error) {
	bounds := map[string]map[string]int{}
	scanner := bufio.NewScanner(strings.NewReader(catalog))
	scanner.Buffer(make([]byte, 65536), 4*1024*1024)
	for scanner.Scan() {
		match := catalogGuardBounds.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		var limits map[string]int
		if err := json.Unmarshal([]byte(match[2]), &limits); err != nil {
			return nil, fmt.Errorf("read the guard bounds of %s: %w", match[1], err)
		}
		bounds[match[1]] = limits
	}
	return bounds, scanner.Err()
}

// PolicyBounds is what each table's guards must hold under columns.txt: every
// bounded column of the catalog with its limit, keyed by table.
func PolicyBounds(catalog string) (map[string]map[string]int, error) {
	columns, err := Columns(strings.NewReader(catalog))
	if err != nil {
		return nil, err
	}
	policies, err := Policies()
	if err != nil {
		return nil, err
	}
	bounds := map[string]map[string]int{}
	for _, column := range columns {
		policy, ok := policies[column.Key]
		if !ok {
			return nil, fmt.Errorf("missing text policy: %s", column.Key)
		}
		if policy.Limit == 0 {
			continue
		}
		if bounds[column.Table] == nil {
			bounds[column.Table] = map[string]int{}
		}
		bounds[column.Table][column.Name] = policy.Limit
	}
	return bounds, nil
}
