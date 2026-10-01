// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Field masks meeting the report engine. A mask withholds a column of a row
// (deals' amount for a rep outside the owning team, ADR-level rule shipped
// with the field-mask work); an aggregate over that column would disclose it
// through the total, and a drill-through would print it. The report engine
// therefore EXCLUDES masked rows — from the aggregate and from the
// drill-through alike, so "Explain This Number" keeps reconciling exactly —
// and reports how many visible rows were withheld as excluded_by_permission,
// so a smaller number reads as governed, never as missing data.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// maskExclusionClauses renders the predicate for the rows of the spec's entity
// this caller may still aggregate — no mask reaching the entity yields
// (nil, false). The clauses AND into the query's WHERE; their negation is the
// excluded_by_permission count's filter.
//
// Which fields the entity withholds is auth's closure to answer, never this
// engine's to infer from the object a mask is configured under: a mask reaches
// the record that republishes its fact, and a report over THAT record is where
// an aggregate would hand the value back as a total.
func maskExclusionClauses(ctx context.Context, spec reportSpec, arg func(any) int) ([]string, bool, error) {
	clauses, err := auth.MaskExclusionClauses(ctx, string(spec.entity), "t", arg)
	if err != nil {
		return nil, false, err
	}
	return clauses, len(clauses) > 0, nil
}

// countMaskExcluded answers how many rows of the report's visible base set the
// mask withheld: the same FROM and WHERE minus the mask clauses, counting the
// rows that fail them. Every bind of the main statement is referenced here
// too, so one args slice serves both.
func countMaskExcluded(ctx context.Context, tx pgx.Tx, frame reportFrame, spec reportSpec, baseWhere []string, maskClauses []string, args []any) (int, error) {
	where := strings.Join(baseWhere, " AND ")
	sql := fmt.Sprintf("SELECT count(*) FILTER (WHERE NOT (%s)) FROM %s WHERE %s",
		strings.Join(maskClauses, " AND "), spec.fromClause(), where)
	sql, args, err := bindReportTokens(ctx, frame, sql, args)
	if err != nil {
		return 0, err
	}
	var excluded int64
	if err := tx.QueryRow(ctx, sql, args...).Scan(&excluded); err != nil {
		return 0, fmt.Errorf("report mask exclusion count: %w", err)
	}
	return int(excluded), nil
}
