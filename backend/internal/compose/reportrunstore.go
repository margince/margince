// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Saving an analytics answer so a report sentence can point at it.
//
// A report block does not carry a number. It carries the id of a run and the
// coordinates of a cell inside it, and the number is dereferenced when the
// report is read. This file is the other half of that: the thing being pointed
// AT.
//
// Two rules govern every function here, and they pull in opposite directions
// on purpose:
//
//   - A saved run is IMMUTABLE. Nothing updates a row; a re-run is a new run
//     with a new id. A block whose numbers moved underneath it would be a
//     sentence that changed meaning after somebody approved it.
//   - A saved run is NOT a cache. The rows were narrowed and floored for the
//     person who asked, so they are never replayed for a second reader. Read
//     re-asks the stored question under the caller's own grants and serves
//     THAT answer.
//
// The second rule is what keeps the first one safe. Storing an answer is only
// tolerable because storing it grants nobody anything.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// tableReportRun is the audited entity type for a saved run.
const tableReportRun = "report_run"

// ReportRun is one saved answer, as the table holds it.
//
// Answer carries the rows as they were served TO THE ASKER. A reader who is
// not the asker never receives this field's stored contents — Read replaces it
// with an answer computed under their own grants — so treat a populated
// Answer as belonging to AskedBy and to nobody else.
type ReportRun struct {
	ID      ids.UUID
	Query   analyticsquery.Query
	Answer  AnalyticsAnswer
	AskedBy ids.UserID
	// Floor is the group floor that judged the stored answer. Two runs served
	// under different floors make different promises about what is missing.
	Floor analyticsquery.Floor
}

// SaveReportRun stores one answer and returns its id.
//
// It takes the query AND the answer rather than running the query itself: the
// caller has just served the answer, and re-running it here would be a second
// execution that could differ from the one the caller returned. The id would
// then name a result nobody ever saw.
//
// There is no read gate on this function and that is not an omission. The
// answer was produced by RunAnalyticsQuery, which gates the population before
// it compiles anything; a second check here would be a second answer to the
// same question and the two would drift.
func SaveReportRun(
	ctx context.Context, tx pgx.Tx,
	q analyticsquery.Query, answer AnalyticsAnswer, floor analyticsquery.Floor,
) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		// A run with no asker cannot be read back: Read serves only the asker,
		// so a row with nobody in that column is one nothing can ever open.
		return ids.UUID{}, fmt.Errorf("compose: saving a report run without an actor")
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}

	queryJSON, err := json.Marshal(q)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: encoding a report run's question: %w", err)
	}
	// Marshalled separately rather than as one struct, because the columns are
	// separate: a reader that wants the row count does not have to parse the
	// rows, and the CHECK constraints can see that each is a list.
	columnsJSON, err := json.Marshal(answer.Columns)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: encoding a report run's columns: %w", err)
	}
	rowsJSON, err := json.Marshal(answer.Rows)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: encoding a report run's rows: %w", err)
	}

	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO report_run
		    (query, result_columns, result_rows, withheld, total_safe,
		     schema_version, asked_by, group_floor, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		queryJSON, columnsJSON, rowsJSON, answer.Withheld, answer.TotalSafe,
		answer.SchemaVersion, actor.UserID, int(floor), capturedBy,
	).Scan(&id); err != nil {
		return ids.UUID{}, fmt.Errorf("compose: saving a report run: %w", err)
	}

	// The audit records that an answer was saved and what was asked. It does
	// not record the ANSWER: an audit row is read by people who did not ask the
	// question, and the rows were narrowed for somebody who did.
	if _, err := storekit.AuditEvent(ctx, tx, "create", tableReportRun, id,
		map[string]any{
			"entity":         q.Entity,
			"schema_version": answer.SchemaVersion,
			"withheld":       answer.Withheld,
		}); err != nil {
		return ids.UUID{}, err
	}
	return id, nil
}

// ReadReportRun returns a saved run's answer, recomputed for THIS caller.
//
// The stored rows are not served. They were narrowed and floored for whoever
// asked, and a second reader's grants narrow a different population — so what
// comes back is the stored QUESTION, re-asked under the caller's own grants
// through the same gate any other query goes through.
//
// That makes a pointer stable without making it a permission. Two readers
// dereferencing one cell can legitimately see different numbers, and one of
// them can legitimately see a refusal.
func ReadReportRun(
	ctx context.Context, tx pgx.Tx, id ids.UUID, floor analyticsquery.Floor,
) (ReportRun, error) {
	var (
		out       ReportRun
		queryJSON []byte
		storedBy  ids.UserID
	)
	if err := tx.QueryRow(ctx, `
		SELECT query, asked_by, group_floor
		FROM report_run
		WHERE id = $1`, id,
	).Scan(&queryJSON, &storedBy, &out.Floor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ReportRun{}, apperrors.ErrNotFound
		}
		return ReportRun{}, fmt.Errorf("compose: reading a report run: %w", err)
	}
	if err := json.Unmarshal(queryJSON, &out.Query); err != nil {
		return ReportRun{}, fmt.Errorf("compose: decoding a report run's question: %w", err)
	}
	out.ID = id
	out.AskedBy = storedBy

	// Re-asked, never replayed. RunAnalyticsQuery re-derives the schema under
	// this caller's grants, re-applies the population's read gate and re-floors
	// the groups — so a reader who may not see the population gets the same
	// refusal they would get asking directly, and a reader who may see less of
	// it gets less.
	//
	// The floor is the CALLER's, not the stored one. A run saved under a floor
	// of 1 must not serve unfloored rows to a reader this installation floors
	// at 5; the stored value is reported so the two answers can be compared,
	// and is not used to judge this one.
	answer, err := RunAnalyticsQuery(ctx, tx, out.Query, floor)
	if err != nil {
		return ReportRun{}, err
	}
	out.Answer = answer
	return out, nil
}
