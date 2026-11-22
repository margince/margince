// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordexport

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lookups are the words for the ids a page of records names: the seat's email,
// the company's name, the pipeline's and the stage's. The reads of the records
// already left out a company the reader may not open, so every id here is one
// the file may name.
type lookups struct {
	owners, companies, pipelines, stages map[ids.UUID]string
}

func nameLookups(ctx context.Context, tx pgx.Tx, records []map[string]json.RawMessage) (lookups, error) {
	var out lookups
	var err error
	for _, read := range []struct {
		into  *map[ids.UUID]string
		field string
		query string
	}{
		{&out.owners, "owner_id", "SELECT id, email FROM app_user WHERE id = ANY($1)"},
		{&out.companies, "company_id", "SELECT id, display_name FROM company WHERE id = ANY($1)"},
		{&out.pipelines, "pipeline_id", "SELECT id, name FROM pipeline WHERE id = ANY($1)"},
		{&out.stages, "stage_id", "SELECT id, name FROM stage WHERE id = ANY($1)"},
	} {
		// A reader without the company grant gets no company name. The record
		// reads already withheld the reference for them.
		if read.field == "company_id" && !auth.ReadGranted(ctx, "company") {
			*read.into = map[ids.UUID]string{}
			continue
		}
		if *read.into, err = wordsFor(ctx, tx, read.query, namedBy(records, read.field)); err != nil {
			return lookups{}, err
		}
	}
	return out, nil
}

// namedBy is the distinct ids the records hold in one field.
func namedBy(records []map[string]json.RawMessage, field string) []ids.UUID {
	seen := map[ids.UUID]bool{}
	var out []ids.UUID
	for _, record := range records {
		if id := uuidOf(record[field]); !id.IsZero() && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func wordsFor(ctx context.Context, tx pgx.Tx, query string, idList []ids.UUID) (map[ids.UUID]string, error) {
	words := map[ids.UUID]string{}
	if len(idList) == 0 {
		return words, nil
	}
	rows, err := tx.Query(ctx, query, idList)
	if err != nil {
		return nil, fmt.Errorf("export: reading names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var word string
		if err := rows.Scan(&id, &word); err != nil {
			return nil, fmt.Errorf("export: reading names: %w", err)
		}
		words[id] = word
	}
	return words, rows.Err()
}
