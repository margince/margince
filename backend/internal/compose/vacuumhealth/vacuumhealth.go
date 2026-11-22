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
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/adminadmit"
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

// Endpoint serves GET /admin/vacuum-recommendations. Server embeds it, and an
// embedded `Handlers` would clash with briefs.Handlers.
type Endpoint struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewEndpoint reads through pool and stamps each report with now.
func NewEndpoint(pool *pgxpool.Pool, now func() time.Time) Endpoint {
	return Endpoint{pool: pool, now: now}
}

// GetVacuumRecommendations reports the tables over both default thresholds.
func (e Endpoint) GetVacuumRecommendations(w http.ResponseWriter, r *http.Request) {
	if !adminadmit.HealthReader(w, r) {
		return
	}
	now := e.now()
	adminadmit.ServeReport(w, r, e.pool, "vacuum recommendations",
		func(ctx context.Context, tx pgx.Tx) (crmcontracts.VacuumRecommendations, error) {
			return Read(ctx, tx, now, Default)
		})
}

// readingsSQL reads every figure from the catalog, never from the table files.
// relpages and reltuples come from the same VACUUM or ANALYZE, so a table that
// grew since then does not look empty. No per-table function opens a relation.
// So a table under the `VACUUM FULL` this advises takes no lock from the read.
// A table dropped meanwhile cannot fail it either.
//
// A table no update or delete ever touched has no dead rows to win back.
const readingsSQL = `
	SELECT n.nspname, c.relname, pg_get_userbyid(c.relowner),
	       c.relpages::bigint * current_setting('block_size')::bigint, c.reltuples::float8,
	       coalesce((SELECT option_value::int FROM pg_options_to_table(c.reloptions)
	                  WHERE option_name = 'fillfactor'), 100),
	       current_setting('block_size')::int,
	       cols.natts, cols.stats,
	       coalesce(cols.aligns, '{}'), coalesce(cols.lengths, '{}'),
	       coalesce(cols.widths, '{}'), coalesce(cols.null_fracs, '{}')
	  FROM pg_stat_user_tables t
	  JOIN pg_class c ON c.oid = t.relid
	  JOIN pg_namespace n ON n.oid = c.relnamespace
	 CROSS JOIN LATERAL (
	       SELECT count(*)::int AS natts,
	              count(s.attname) FILTER (WHERE NOT a.attisdropped)::int AS stats,
	              array_agg(a.attalign::text ORDER BY a.attnum) FILTER (WHERE NOT a.attisdropped) AS aligns,
	              array_agg(a.attlen::int ORDER BY a.attnum) FILTER (WHERE NOT a.attisdropped) AS lengths,
	              array_agg(coalesce(s.avg_width, 0)::float8 ORDER BY a.attnum) FILTER (WHERE NOT a.attisdropped) AS widths,
	              array_agg(coalesce(s.null_frac, 0)::float8 ORDER BY a.attnum) FILTER (WHERE NOT a.attisdropped) AS null_fracs
	         FROM pg_attribute a
	         LEFT JOIN pg_stats s ON s.schemaname = n.nspname AND s.tablename = c.relname
	                             AND s.attname = a.attname AND NOT s.inherited
	        WHERE a.attrelid = c.oid AND a.attnum > 0
	       ) cols
	 WHERE c.relpersistence <> 't'
	   AND t.n_tup_upd + t.n_tup_del > 0
	   AND c.relpages::bigint * current_setting('block_size')::bigint >= $1`

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
	rows, err := tx.Query(ctx, readingsSQL, limits.MinWasteBytes)
	if err != nil {
		return crmcontracts.VacuumRecommendations{}, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanReading(rows)
		if err != nil {
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

func scanReading(rows pgx.Rows) (tableReading, error) {
	var t tableReading
	var aligns []string
	var lengths []int32
	var widths, nullFracs []float64
	if err := rows.Scan(&t.schema, &t.table, &t.owner, &t.tableBytes, &t.liveRows, &t.fillfactor, &t.blockSize,
		&t.attributes, &t.withStats, &aligns, &lengths, &widths, &nullFracs); err != nil {
		return tableReading{}, err
	}
	for i := range aligns {
		var align byte
		if aligns[i] != "" {
			align = aligns[i][0]
		}
		t.columns = append(t.columns, column{align: align, length: int(lengths[i]), width: widths[i], nullFrac: nullFracs[i]})
	}
	return t, nil
}

func (l Thresholds) admit(waste, tableBytes int64) bool {
	return waste > 0 && waste >= l.MinWasteBytes && waste*100 >= int64(l.MinWastePercent)*tableBytes
}

func (t tableReading) recommendation(waste int64) crmcontracts.VacuumRecommendation {
	return crmcontracts.VacuumRecommendation{
		Schema: t.schema, Table: t.table, Owner: t.owner,
		TableBytes: int(t.tableBytes), WasteBytes: int(waste),
		WastePercent: int(waste * 100 / t.tableBytes),
		Statement:    "VACUUM (FULL, ANALYZE) " + pgx.Identifier{t.schema, t.table}.Sanitize() + ";",
	}
}
