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

// The trigger conditions read the row as a whole. So no trigger depends on one
// column, and a later migration may still drop or retype it.
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
CREATE FUNCTION public.text_row_exceeds_bounds(new_row jsonb, old_row jsonb, bounds jsonb)
RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
SELECT EXISTS (SELECT 1 FROM jsonb_each_text(bounds) AS bound(key, maximum)
 WHERE CASE jsonb_typeof(new_row -> bound.key)
  WHEN 'string' THEN public.text_value_exceeds_bound(new_row ->> bound.key, old_row ->> bound.key, bound.maximum::integer)
  WHEN 'array' THEN public.text_values_exceed_bound(
   ARRAY(SELECT jsonb_array_elements_text(new_row -> bound.key)),
   ARRAY(SELECT jsonb_array_elements_text(NULLIF(old_row -> bound.key, 'null'::jsonb))), bound.maximum::integer)
  ELSE false END)
$$;
CREATE FUNCTION public.refuse_oversized_text() RETURNS trigger LANGUAGE plpgsql SET search_path = pg_catalog AS $$
DECLARE
 bound record;
 inserted jsonb := to_jsonb(NEW);
 previous jsonb;
BEGIN
 IF TG_OP = 'UPDATE' THEN previous := to_jsonb(OLD); END IF;
 FOR bound IN SELECT key, value FROM jsonb_each(TG_ARGV[0]::jsonb)
 LOOP
  IF public.text_row_exceeds_bounds(inserted, previous, jsonb_build_object(bound.key, bound.value)) THEN
   RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001',
    CONSTRAINT = 'text_character_bound', COLUMN = bound.key, DETAIL = bound.value::text;
  END IF;
 END LOOP;
 RAISE EXCEPTION 'text exceeds its character limit' USING ERRCODE = '22001';
END
$$;
`

// Migration is one generated up and down pair.
type Migration struct{ Up, Down string }

// SQL builds the guards in two pairs, since custom migrations run after core ones.
// A table that customTables names gets its guards from the custom pair. The core
// pair keeps the shared functions those guards call.
func SQL(catalog io.Reader, customTables map[string]bool) (core, custom Migration, err error) {
	columns, err := Columns(catalog)
	if err != nil {
		return core, custom, err
	}
	policies, err := Policies()
	if err != nil {
		return core, custom, err
	}
	tables := map[string][]Column{}
	for _, column := range columns {
		policy, ok := policies[column.Key]
		if !ok {
			return core, custom, fmt.Errorf("missing text policy: %s", column.Key)
		}
		if policy.Limit != 0 {
			if column.Generated {
				return core, custom, fmt.Errorf("generated column needs an exemption: %s", column.Key)
			}
			tables[column.Table] = append(tables[column.Table], column)
		}
	}
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	slices.Sort(names)
	var coreUp, coreDown, customUp, customDown strings.Builder
	coreUp.WriteString(guardFunctions)
	for _, table := range names {
		if customTables[table] {
			writeTableGuards(&customUp, &customDown, table, tables[table], policies)
		} else {
			writeTableGuards(&coreUp, &coreDown, table, tables[table], policies)
		}
	}
	coreUp.WriteString(existingCustomTextGuards())
	coreDown.WriteString(dropCustomTextGuards)
	coreDown.WriteString("DROP FUNCTION public.refuse_oversized_text();\nDROP FUNCTION public.text_row_exceeds_bounds(jsonb, jsonb, jsonb);\nDROP FUNCTION public.text_values_exceed_bound(text[], text[], integer);\nDROP FUNCTION public.text_value_exceeds_bound(text, text, integer);\n")
	return Migration{coreUp.String(), coreDown.String()}, Migration{customUp.String(), customDown.String()}, nil
}

func writeTableGuards(up, down *strings.Builder, table string, columns []Column, policies map[string]Policy) {
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		fields = append(fields, fmt.Sprintf("%q:%d", column.Name, policies[column.Key].Limit))
	}
	limits := strings.ReplaceAll("{"+strings.Join(fields, ",")+"}", "'", "''")
	relation := pgx.Identifier{"public", table}.Sanitize()
	for _, guard := range []struct{ name, timing, previous string }{
		{"text_bound_insert", "AFTER INSERT", "NULL"},
		{"text_bound_update", "AFTER UPDATE", "to_jsonb(OLD)"},
	} {
		trigger := pgx.Identifier{guard.name}.Sanitize()
		fmt.Fprintf(up, "CREATE TRIGGER %s %s ON %s FOR EACH ROW\nWHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), %s, '%s')) EXECUTE FUNCTION public.refuse_oversized_text('%s');\n",
			trigger, guard.timing, relation, guard.previous, limits, limits)
		fmt.Fprintf(down, "DROP TRIGGER %s ON %s;\n", trigger, relation)
	}
}

func existingCustomTextGuards() string {
	return fmt.Sprintf(`DO $bounds$
DECLARE target record;
BEGIN
 FOR target IN
  SELECT f.object, jsonb_object_agg(f.column_name, %d)::text AS limits
  FROM public.custom_field f
  JOIN pg_catalog.pg_namespace n ON n.nspname = 'public'
  JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = f.object
  JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid AND a.attname = f.column_name AND a.attnum > 0 AND NOT a.attisdropped
  WHERE f.type = 'text' AND a.atttypid = 'text'::regtype
  GROUP BY f.object
 LOOP
  EXECUTE format('CREATE TRIGGER custom_text_bound_insert AFTER INSERT ON public.%%I FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, %%L)) EXECUTE FUNCTION public.refuse_oversized_text(%%L)', target.object, target.limits, target.limits);
  EXECUTE format('CREATE TRIGGER custom_text_bound_update AFTER UPDATE ON public.%%I FOR EACH ROW WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), %%L)) EXECUTE FUNCTION public.refuse_oversized_text(%%L)', target.object, target.limits, target.limits);
 END LOOP;
END
$bounds$;
`, values.TextDocumentCharacters)
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
