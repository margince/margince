// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vacuumhealth tells an operator which tables a full vacuum, run out of
// band, would shrink by enough to be worth a maintenance window.
//
// Margince only advises. `VACUUM FULL` holds an `ACCESS EXCLUSIVE` lock for the whole
// rewrite, so reads and writes of the table wait. Only the table's owner may run it.
package vacuumhealth

import (
	"cmp"
	"context"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/adminread"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Thresholds decide which tables are listed. A table must clear both.
type Thresholds struct {
	// MinWastePercent: under a fifth of the file empty, the space is mostly
	// what normal updates reuse, and a rewrite wins little back for its lock.
	MinWastePercent int
	// MinWasteBytes keeps a small table with a high share off the list: its
	// rewrite returns too little disk to be worth a maintenance window.
	MinWasteBytes int64
}

// Default is the one place the thresholds the endpoint uses are set.
var Default = Thresholds{MinWastePercent: 20, MinWasteBytes: 100_000_000}

// Endpoint serves GET /admin/vacuum-recommendations.
type Endpoint struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

// GetVacuumRecommendations reports the tables over both default thresholds.
func (e Endpoint) GetVacuumRecommendations(w http.ResponseWriter, r *http.Request) {
	if !adminread.AdmitHealthReader(w, r) {
		return
	}
	now := e.Now()
	adminread.ServeReport(w, r, e.Pool, "vacuum recommendations",
		func(ctx context.Context, tx pgx.Tx) (crmcontracts.VacuumRecommendations, error) {
			return Read(ctx, tx, now, Default)
		})
}

// tableReading is one table as the catalog describes it.
type tableReading struct {
	schema, table, owner string
	tableBytes           int64
	// liveRows is the planner's row estimate; below zero, the table was never
	// analyzed and its row count is unknown.
	liveRows float64
	// rowWidth is the average stored width of a row's values, unknown when no
	// column has statistics.
	rowWidth   *float64
	fillfactor int
	blockSize  int
}

// Read lists the tables over the thresholds, most empty space first.
//
// Only tables whose file already reaches MinWasteBytes are read: a smaller
// file cannot hold that much empty space, so leaving it out loses no answer.
func Read(ctx context.Context, tx pgx.Tx, now time.Time, limits Thresholds) (crmcontracts.VacuumRecommendations, error) {
	out := crmcontracts.VacuumRecommendations{
		GeneratedAt:     now,
		MinWastePercent: limits.MinWastePercent,
		MinWasteBytes:   int(limits.MinWasteBytes),
		Tables:          []crmcontracts.VacuumRecommendation{},
	}
	rows, err := tx.Query(ctx, `
		WITH widths AS (
			SELECT schemaname, tablename, sum((1 - null_frac) * avg_width)::float8 AS width
			  FROM pg_stats
			 GROUP BY schemaname, tablename
		)
		SELECT n.nspname, c.relname, pg_get_userbyid(c.relowner),
		       pg_relation_size(c.oid), c.reltuples::float8, w.width,
		       coalesce((SELECT option_value::int FROM pg_options_to_table(c.reloptions)
		                  WHERE option_name = 'fillfactor'), 100),
		       current_setting('block_size')::int
		  FROM pg_stat_user_tables t
		  JOIN pg_class c ON c.oid = t.relid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  LEFT JOIN widths w ON w.schemaname = n.nspname AND w.tablename = c.relname
		 WHERE pg_relation_size(c.oid) >= $1`, limits.MinWasteBytes)
	if err != nil {
		return crmcontracts.VacuumRecommendations{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var t tableReading
		if err := rows.Scan(&t.schema, &t.table, &t.owner, &t.tableBytes, &t.liveRows, &t.rowWidth,
			&t.fillfactor, &t.blockSize); err != nil {
			return crmcontracts.VacuumRecommendations{}, err
		}
		waste, known := t.wasteBytes()
		switch {
		case !known:
			out.UnmeasuredTables++
		case limits.admit(waste, t.tableBytes):
			out.Tables = append(out.Tables, t.recommendation(waste))
		}
	}
	if err := rows.Err(); err != nil {
		return crmcontracts.VacuumRecommendations{}, err
	}
	slices.SortFunc(out.Tables, func(a, b crmcontracts.VacuumRecommendation) int {
		return cmp.Or(cmp.Compare(b.WasteBytes, a.WasteBytes), cmp.Compare(a.Schema, b.Schema), cmp.Compare(a.Table, b.Table))
	})
	return out, nil
}

func (l Thresholds) admit(waste, tableBytes int64) bool {
	return waste >= l.MinWasteBytes && waste*100 >= int64(l.MinWastePercent)*tableBytes
}

func (t tableReading) recommendation(waste int64) crmcontracts.VacuumRecommendation {
	return crmcontracts.VacuumRecommendation{
		Schema: t.schema, Table: t.table, Owner: t.owner,
		TableBytes: int(t.tableBytes), WasteBytes: int(waste),
		WastePercent: int(waste * 100 / t.tableBytes),
		Statement:    "VACUUM (FULL, ANALYZE) " + pgx.Identifier{t.schema, t.table}.Sanitize() + ";",
	}
}

// Postgres stores a row as a 24-byte header plus its values, padded to 8 bytes.
// Each row also has a 4-byte pointer in its page, which has a 24-byte header.
const (
	rowHeaderBytes  = 24
	rowAlignBytes   = 8
	rowPointerBytes = 4
	pageHeaderBytes = 24
)

// wasteBytes estimates the part of the file that holds no live row. That is the
// file size less the pages the live rows fill when packed to the fillfactor.
// It leaves out padding between values, so it reads a little high.
func (t tableReading) wasteBytes() (int64, bool) {
	if t.liveRows < 0 {
		return 0, false
	}
	var needed int64
	if t.liveRows > 0 {
		if t.rowWidth == nil {
			return 0, false
		}
		row := int64(math.Ceil((rowHeaderBytes+*t.rowWidth)/rowAlignBytes))*rowAlignBytes + rowPointerBytes
		usable := int64(t.blockSize-pageHeaderBytes) * int64(t.fillfactor) / 100
		perPage := max(usable/row, 1)
		pages := int64(math.Ceil(t.liveRows / float64(perPage)))
		needed = pages * int64(t.blockSize)
	}
	return max(t.tableBytes-needed, 0), true
}
