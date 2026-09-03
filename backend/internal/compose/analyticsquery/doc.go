// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package analyticsquery answers a question nobody wrote a report for.
//
// The prebuilt reports answer questions somebody anticipated. This answers the
// rest — but the shape of the answer is the same shape, deliberately: a
// population, a grouping, a measure, and the row scope of whoever asked. What
// changes is that the request names those rather than a report key.
//
// THE MODEL NEVER COMPUTES A NUMBER. It writes a query in a typed intermediate
// form; this package compiles that form to SQL and Postgres does the
// arithmetic. A model asked to add up a column will produce a plausible total
// and be wrong in a way nobody can see, and the whole value of a figure on a
// revenue screen is that somebody can trace it to rows.
//
// Three rules bind the compiler and none of them is negotiable:
//
//   - EVERY IDENTIFIER COMES FROM THE DERIVED SCHEMA. A table, column or
//     expression is looked up in a catalog built from the report specs; nothing
//     from a request is ever formatted into SQL. Values are bind parameters.
//   - THE CALLER'S GRANTS NARROW THE SCHEMA BEFORE THE QUERY IS PLANNED. A
//     field somebody may not read is not a field they can be refused for
//     naming — it is absent, so the refusal says "no such field" and discloses
//     nothing about what exists.
//   - A GROUP SMALLER THAN THE FLOOR IS WITHHELD ENTIRELY — keys included — AND
//     SO ARE ITS COMPLEMENTS. Suppressing only the small group leaves it
//     recoverable by subtracting the rest from the total; keeping its keys
//     turns a grouping by a high-cardinality field into a record dump. A filter
//     that separates out fewer records than the floor is refused outright,
//     because the answer to it beside the answer without it is that set.
package analyticsquery
