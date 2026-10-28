// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// uncopiedCompanyPoints selects the companies whose point is still only on the
// company row: geocoded before the side row held points, with no status there.
const uncopiedCompanyPoints = `FROM company o
	  LEFT JOIN company_geocode_state g ON g.company_id = o.id
	 WHERE o.geocode_status IS NOT NULL AND g.status IS NULL`

type companyPointRow struct {
	ID                                          ids.UUID
	Line1, Line2, City, Region, Postal, Country *string
	Lat, Lon                                    *float64
	Status                                      string
	Provider, InputHash                         *string
	GeocodedAt                                  *time.Time
}

// CopyCompanyPointsTx moves up to limit companies' points from the company row
// to the side row, after the cursor, and returns the companies it inspected.
//
// A point whose address moved since it was resolved is copied as stale. The
// company row's own stale mark is gone. An answer for a previous address
// must not stand in for the address the company holds now. A side row the
// worker already answered is left as it is. The point is derived data, so the
// write is not audited, the same as the worker's own write.
func CopyCompanyPointsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	rows, err := tx.Query(ctx, `
		SELECT o.id, o.address_line1, o.address_line2, o.address_city, o.address_region,
		       o.address_postal_code, o.address_country, o.geocode_lat, o.geocode_lon,
		       o.geocode_status, o.geocode_provider, o.geocode_input_hash, o.geocoded_at
		  `+uncopiedCompanyPoints+` AND o.id > $1
		 ORDER BY o.id LIMIT $2 FOR UPDATE OF o`, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading company points to copy: %w", err)
	}
	points, err := pgx.CollectRows(rows, pgx.RowToStructByPos[companyPointRow])
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading company points to copy: %w", err)
	}
	inspected := make([]ids.UUID, 0, len(points))
	statuses := make([]string, 0, len(points))
	for _, point := range points {
		inspected = append(inspected, point.ID)
		statuses = append(statuses, copiedStatus(point))
	}
	if len(inspected) == 0 {
		return inspected, 0, nil
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO company_geocode_state (company_id, lat, lon, status, provider, input_hash, geocoded_at)
		SELECT o.id, o.geocode_lat, o.geocode_lon, copied.status, o.geocode_provider, o.geocode_input_hash, o.geocoded_at
		  FROM unnest($1::uuid[], $2::text[]) AS copied(id, status)
		  JOIN company o ON o.id = copied.id
		ON CONFLICT (company_id) DO UPDATE
		   SET lat = EXCLUDED.lat, lon = EXCLUDED.lon, status = EXCLUDED.status, provider = EXCLUDED.provider,
		       input_hash = EXCLUDED.input_hash, geocoded_at = EXCLUDED.geocoded_at
		 WHERE company_geocode_state.status IS NULL`, inspected, statuses)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: copying company points: %w", err)
	}
	return inspected, int(tag.RowsAffected()), nil
}

// copiedStatus keeps a point's status while it still describes the address the
// company holds. Once the address moved on it is stale, whatever the status.
func copiedStatus(point companyPointRow) string {
	query := addressQuery(point.Line1, point.Line2, point.City, point.Region, point.Postal, point.Country)
	if point.InputHash != nil && query != "" && addressHash(query) == *point.InputHash {
		return point.Status
	}
	return GeocodeStale
}

// CountUncopiedCompanyPoints counts what CopyCompanyPointsTx has yet to
// inspect, reading at most maximum+1.
func CountUncopiedCompanyPoints(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 `+uncopiedCompanyPoints+` LIMIT $1) uncopied`, maximum+1).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contacts: counting company points to copy: %w", err)
	}
	return n, nil
}
