// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package textbounds

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
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
DECLARE
 bound record;
 inserted jsonb := to_jsonb(NEW);
 previous jsonb := '{}'::jsonb;
 oversized boolean;
BEGIN
 IF TG_OP = 'UPDATE' THEN previous := to_jsonb(OLD); END IF;
 FOR bound IN SELECT key, value::integer AS maximum FROM jsonb_each_text(TG_ARGV[0]::jsonb)
 LOOP
  IF jsonb_typeof(inserted -> bound.key) = 'array' THEN
   oversized := public.text_values_exceed_bound(
    ARRAY(SELECT jsonb_array_elements_text(inserted -> bound.key)),
    ARRAY(SELECT jsonb_array_elements_text(NULLIF(previous -> bound.key, 'null'::jsonb))), bound.maximum);
  ELSE
   oversized := public.text_value_exceeds_bound(inserted ->> bound.key, previous ->> bound.key, bound.maximum);
  END IF;
  IF oversized THEN
   RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001',
    CONSTRAINT = 'text_character_bound', COLUMN = bound.key, DETAIL = bound.maximum::text;
  END IF;
 END LOOP;
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
	up.WriteString(existingCustomTextGuards())
	down.WriteString(dropCustomTextGuards)
	down.WriteString("DROP FUNCTION public.refuse_oversized_text();\nDROP FUNCTION public.text_values_exceed_bound(text[], text[], integer);\nDROP FUNCTION public.text_value_exceeds_bound(text, text, integer);\n")
	return up.String(), down.String(), nil
}

func writeTableGuards(up, down *strings.Builder, table string, columns []Column, policies map[string]Policy) {
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, fmt.Sprintf("%q:%d", column.Name, policies[column.Key].Limit))
	}
	limits := strings.ReplaceAll("{"+strings.Join(fields, ",")+"}", "'", "''")
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
		timing := "BEFORE"
		if operation == "INSERT" {
			timing = "AFTER"
		}
		fmt.Fprintf(up, "CREATE TRIGGER %s %s %s ON %s FOR EACH ROW\nWHEN (%s) EXECUTE FUNCTION public.refuse_oversized_text('%s');\n", trigger, timing, operation, relation, strings.Join(checks, " OR\n"), limits)
		fmt.Fprintf(down, "DROP TRIGGER %s ON %s;\n", trigger, relation)
	}
}

func existingCustomTextGuards() string {
	return fmt.Sprintf(`DO $bounds$
DECLARE target record;
BEGIN
 FOR target IN
  SELECT f.object,
   string_agg(format('public.text_value_exceeds_bound(NEW.%%I, NULL, %d)', f.column_name), ' OR ' ORDER BY f.column_name) AS inserted,
   string_agg(format('public.text_value_exceeds_bound(NEW.%%I, OLD.%%I, %d)', f.column_name, f.column_name), ' OR ' ORDER BY f.column_name) AS changed,
   jsonb_object_agg(f.column_name, %d)::text AS limits
  FROM public.custom_field f
  JOIN pg_catalog.pg_namespace n ON n.nspname = 'public'
  JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = f.object
  JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attname = f.column_name AND a.attnum > 0 AND NOT a.attisdropped
  WHERE f.type = 'text' AND a.atttypid = 'text'::regtype
  GROUP BY f.object
 LOOP
  EXECUTE format('CREATE TRIGGER custom_text_bound_insert AFTER INSERT ON public.%%I FOR EACH ROW WHEN (%%s) EXECUTE FUNCTION public.refuse_oversized_text(%%L)', target.object, target.inserted, target.limits);
  EXECUTE format('CREATE TRIGGER custom_text_bound_update BEFORE UPDATE ON public.%%I FOR EACH ROW WHEN (%%s) EXECUTE FUNCTION public.refuse_oversized_text(%%L)', target.object, target.changed, target.limits);
 END LOOP;
END
$bounds$;
`, values.TextDocumentCharacters, values.TextDocumentCharacters, values.TextDocumentCharacters)
}

const dropCustomTextGuards = `DO $bounds$
DECLARE target record;
BEGIN
 FOR target IN
  SELECT c.relname, t.tgname
  FROM pg_catalog.pg_trigger t
  JOIN pg_catalog.pg_class c ON c.oid = t.tgrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'public' AND t.tgname IN ('custom_text_bound_insert', 'custom_text_bound_update')
 LOOP
  EXECUTE format('DROP TRIGGER %I ON public.%I', target.tgname, target.relname);
 END LOOP;
END
$bounds$;
`
