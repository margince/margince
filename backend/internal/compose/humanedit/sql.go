// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package humanedit renders the SQL fragments of the human-edit-precedence
// lookup, which compose runs over the audit trail.
package humanedit

// LiveValueOf renders the record's current value of latest.key, or NULL
// without a table to read it from.
func LiveValueOf(table string) string {
	if table == "" {
		return "NULL::jsonb"
	}
	return `(SELECT to_jsonb(t) -> latest.key FROM ` + table + ` t WHERE t.id = $2)`
}

// UnauditedHolder renders the predicate deciding whether an UNAUDITED patch
// key is human-owned. With a table to read, the answer is "only if the row
// holds a value there that is not the column's own default".
// Filling an empty field undoes nobody. Neither does moving a value the
// database supplied. An access field is the exception: every value is the
// human's. Without a table, every unaudited key on a human-created record is
// treated as theirs.
func UnauditedHolder(table string) string {
	if table == "" {
		return "true"
	}
	return `EXISTS (
			    SELECT 1 FROM ` + table + ` t
			    WHERE t.id = $2 AND to_jsonb(t) -> p.key IS NOT NULL
			      AND to_jsonb(t) -> p.key <> 'null'::jsonb
			      AND to_jsonb(t) -> p.key <> '""'::jsonb
			      AND (p.key IN (` + accessFields + `)
			        OR (to_jsonb(t) ->> p.key) IS DISTINCT FROM (` + ConstantColumnDefault + `))
			  )`
}

// accessFields are the pair platform/auth's row scope reads to decide who
// sees a record. Moving one off its default hides the record from colleagues.
const accessFields = `'visibility', 'owner_id'`

// ConstantColumnDefault reads, as text, the catalog default of column p.key on
// row t. The column must hold text or an enum, and the default a quoted
// literal. Any other reads as NULL, so the tie goes to asking the human.
const ConstantColumnDefault = `SELECT ` + literalOfDefault + `
			      FROM pg_attribute att
			      JOIN pg_type typ ON typ.oid = att.atttypid
			      JOIN pg_attrdef def ON def.adrelid = att.attrelid AND def.adnum = att.attnum
			      CROSS JOIN LATERAL (SELECT pg_get_expr(def.adbin, def.adrelid) AS expr) d
			      WHERE att.attrelid = t.tableoid AND att.attname = p.key`

// literalOfDefault unquotes a default d.expr shaped like
// 'unknown'::company_lifecycle on a column of type typ. Only a text or enum
// default is nobody's choice. A number, boolean, date or jsonb default is a
// value somebody chose, so it reads as NULL. So do now(), an array and a
// domain.
const literalOfDefault = `CASE
			        WHEN (typ.typtype = 'e' OR typ.typname IN ('text', 'varchar', 'bpchar', 'citext'))
			         AND d.expr ~ '^''.*''::[a-z_][a-z_0-9 (),.]*$'
			          THEN replace(substring(d.expr FROM '^''(.*)''::'), '''''', '''')
			      END`
