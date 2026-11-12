// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textbounds

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

const guardFunctions = `CREATE FUNCTION public.text_value_exceeds_bound(new_value text, old_value text, character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT new_value IS NOT NULL AND new_value COLLATE "C" IS DISTINCT FROM old_value COLLATE "C"
 AND char_length(new_value) > character_limit
$$;
CREATE FUNCTION public.text_values_exceed_bound(new_values text[], old_values text[], character_limit integer)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT EXISTS (SELECT 1 FROM unnest(new_values) AS item(value)
 WHERE char_length(value) > character_limit
 AND NOT EXISTS (SELECT 1 FROM unnest(old_values) AS previous(value)
                 WHERE previous.value COLLATE "C" = item.value COLLATE "C"))
$$;
CREATE FUNCTION public.refuse_oversized_text() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
 RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001';
END
$$;
`

func SQL(catalog io.Reader) (string, string, error) {
	columns, err := Columns(catalog)
	if err != nil {
		return "", "", err
	}
	policies, err := Policies()
	if err != nil {
		return "", "", err
	}
	tables := map[string][]Column{}
	for _, column := range columns {
		policy, ok := policies[column.Key]
		if !ok {
			return "", "", fmt.Errorf("missing text policy: %s", column.Key)
		}
		if policy.Limit != 0 {
			if column.Generated {
				return "", "", fmt.Errorf("generated column needs an exemption: %s", column.Key)
			}
			tables[column.Table] = append(tables[column.Table], column)
		}
	}
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	var up, down strings.Builder
	up.WriteString(guardFunctions)
	for _, table := range names {
		writeTableGuards(&up, &down, table, tables[table], policies)
	}
	down.WriteString("DROP FUNCTION public.refuse_oversized_text();\nDROP FUNCTION public.text_values_exceed_bound(text[], text[], integer);\nDROP FUNCTION public.text_value_exceeds_bound(text, text, integer);\n")
	return up.String(), down.String(), nil
}

func writeTableGuards(up, down *strings.Builder, table string, columns []Column, policies map[string]Policy) {
	for _, operation := range []string{"INSERT", "UPDATE"} {
		checks := make([]string, 0, len(columns))
		for _, column := range columns {
			field := pgx.Identifier{column.Name}.Sanitize()
			previous := "NULL"
			if operation == "UPDATE" {
				previous = "OLD." + field
			}
			fn := "text_value_exceeds_bound"
			if column.Array {
				fn = "text_values_exceed_bound"
			}
			checks = append(checks, fmt.Sprintf("public.%s(NEW.%s, %s, %d)", fn, field, previous, policies[column.Key].Limit))
		}
		trigger := pgx.Identifier{"text_bound_" + strings.ToLower(operation)}.Sanitize()
		relation := pgx.Identifier{"public", table}.Sanitize()
		fmt.Fprintf(up, "CREATE TRIGGER %s BEFORE %s ON %s FOR EACH ROW\nWHEN (%s) EXECUTE FUNCTION public.refuse_oversized_text();\n", trigger, operation, relation, strings.Join(checks, " OR\n"))
		fmt.Fprintf(down, "DROP TRIGGER %s ON %s;\n", trigger, relation)
	}
}
