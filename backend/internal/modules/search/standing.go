// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// standingColumns is what one record type says about where it stands, as SQL
// over the alias t. An empty column is a typed NULL, so every type answers
// the same row shape and the page is read in one statement.
type standingColumns struct {
	hitType, state, dueOn, jobTitle, lastActivity string
	// clockRows are the activity rows the record's stored clock is folded
	// from, so the standing can say whether that clock names a day entry.
	clockRows string
}

// standingReads are the hit types a result list shows a standing for, in the
// order their parts appear in the statement. A deal adds its stage and its
// masked money in dealStanding.
var standingReads = []standingColumns{
	{hitType: entityContact, jobTitle: "t.title", lastActivity: lastActivityColumn,
		clockRows: occurredday.LinkedClockRowsSQL("contact_id", "t.id")},
	{hitType: entityCompany, lastActivity: lastActivityColumn,
		clockRows: storekit.CompanyClockRowsSQL("t")},
	{hitType: entityDeal, state: "t.status", dueOn: "t.expected_close_date", lastActivity: lastActivityColumn,
		clockRows: occurredday.LinkedClockRowsSQL("deal_id", "t.id")},
	{hitType: entityLead, state: "t.status"},
	{hitType: entityProject, state: "t.phase", dueOn: "t.target_end_date"},
}

// The column every record that tracks touch keeps its latest activity in.
const lastActivityColumn = "t.last_activity_at"

// A text column a type does not have, typed so every leg of the union agrees.
const nullText = "NULL::text"

// Wire names of the deal fields a mask can withhold here.
const (
	maskedAmount   = "amount_minor"
	maskedCurrency = "currency"
)

// attachStandings fills Standing on the page's contact, company, deal, lead
// and project hits. A type the caller may not read is skipped, which leaves
// its hits without a standing rather than failing the search.
func (s *Store) attachStandings(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	wanted := map[string][]ids.UUID{}
	for i := range hits {
		wanted[hits[i].Type] = append(wanted[hits[i].Type], hits[i].ID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	var parts []string
	for _, read := range standingReads {
		recordIDs := wanted[read.hitType]
		if len(recordIDs) == 0 || !auth.ReadGranted(ctx, read.hitType) {
			continue
		}
		part, err := standingPart(ctx, read, recordIDs, arg)
		if err != nil {
			return err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil
	}
	standings, err := readStandings(ctx, tx, strings.Join(parts, " UNION ALL "), args)
	if err != nil {
		return err
	}
	if err := attachClockPrecisions(ctx, tx, standings); err != nil {
		return err
	}
	for i := range hits {
		if standing, ok := standings[hits[i].Type+":"+hits[i].ID.String()]; ok {
			hits[i].Standing = &standing
		}
	}
	return nil
}

// standingPart renders one type's leg: the standing of the named records the
// caller's row scope admits.
func standingPart(ctx context.Context, read standingColumns, recordIDs []ids.UUID, arg func(any) int) (string, error) {
	scope, err := auth.ScopeClauseFor(ctx, read.hitType, "t", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = sqlTrue
	}
	stage, amount, currency, masks, join := nullText, "NULL::bigint", nullText, "NULL::text[]", ""
	if read.hitType == entityDeal {
		if stage, amount, currency, masks, err = dealStanding(ctx, arg); err != nil {
			return "", err
		}
		join = " LEFT JOIN stage st ON st.id = t.stage_id"
	}
	return fmt.Sprintf(
		"SELECT '%s'::text, t.id, %s, %s, %s, %s, %s, %s, %s, %s FROM %s t%s WHERE t.id = ANY($%d) AND (%s)",
		read.hitType, orNull(read.state, "text"), stage, amount, currency, orNull(read.dueOn, "date"),
		orNull(read.jobTitle, "text"), orNull(read.lastActivity, "timestamptz"), masks,
		read.hitType, join, arg(recordIDs), scope,
	), nil
}

// dealStanding renders a deal's stage in the reader's language and its money
// under the caller's field masks, with the names of the fields withheld.
func dealStanding(ctx context.Context, arg func(any) int) (stage, amount, currency, masks string, err error) {
	stage = labeltranslation.SQL(ctx, "st.name", "st.name_translations", arg)
	var withheld []string
	for _, field := range []string{maskedAmount, maskedCurrency} {
		clause, masked, maskErr := auth.MaskExcludedClause(ctx, entityDeal, field, "t", arg)
		if maskErr != nil {
			return "", "", "", "", maskErr
		}
		value := "t." + field
		if masked {
			value = fmt.Sprintf("CASE WHEN %s THEN t.%s END", clause, field)
			withheld = append(withheld, fmt.Sprintf("CASE WHEN %s THEN NULL ELSE '%s' END", clause, field))
		}
		if field == maskedAmount {
			amount = value
		} else {
			currency = value + "::text"
		}
	}
	masks = "NULL::text[]"
	if len(withheld) > 0 {
		masks = "array_remove(ARRAY[" + strings.Join(withheld, ", ") + "]::text[], NULL)"
	}
	return stage, amount, currency, masks, nil
}

// orNull is column, or a NULL of its type where the hit type has none.
func orNull(column, sqlType string) string {
	if column == "" {
		return "NULL::" + sqlType
	}
	return column
}

// readStandings runs the legs and keys each standing by type and id.
func readStandings(ctx context.Context, tx pgx.Tx, statement string, args []any) (map[string]crmcontracts.SearchHitStanding, error) {
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading where this page's records stand: %w", err)
	}
	defer rows.Close()
	standings := map[string]crmcontracts.SearchHitStanding{}
	for rows.Next() {
		var (
			hitType  string
			recordID ids.UUID
			standing crmcontracts.SearchHitStanding
			dueOn    *time.Time
			masked   []string
		)
		if err := rows.Scan(&hitType, &recordID, &standing.State, &standing.Stage, &standing.AmountMinor,
			&standing.Currency, &dueOn, &standing.JobTitle, &standing.LastActivityAt, &masked); err != nil {
			return nil, fmt.Errorf("search: reading where this page's records stand: %w", err)
		}
		if dueOn != nil {
			standing.DueOn = &openapi_types.Date{Time: *dueOn}
		}
		if len(masked) > 0 {
			standing.MaskedFields = &masked
		}
		standings[hitType+":"+recordID.String()] = standing
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search: reading where this page's records stand: %w", err)
	}
	return standings, nil
}

// clockPrecisionLockWait bounds how long the precision read waits for the
// activity table. The standings never wait on it: a page whose activities are
// locked away keeps its standings and draws their times as instants.
const clockPrecisionLockWait = "100ms"

// attachClockPrecisions says how to read each standing's last activity, from
// the activity its stored clock names. It reads under its own savepoint and a
// short lock wait. A locked or slow activity table costs the page only the
// precision, never the search.
func attachClockPrecisions(ctx context.Context, tx pgx.Tx, standings map[string]crmcontracts.SearchHitStanding) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	var parts []string
	for _, read := range standingReads {
		var recordIDs []ids.UUID
		for key, standing := range standings {
			hitType, id, _ := strings.Cut(key, ":")
			parsed, err := ids.Parse(id)
			if hitType == read.hitType && read.clockRows != "" && standing.LastActivityAt != nil && err == nil {
				recordIDs = append(recordIDs, parsed)
			}
		}
		if len(recordIDs) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("SELECT '%s'::text, t.id FROM %s t WHERE t.id = ANY($%d) AND %s",
			read.hitType, read.hitType, arg(recordIDs), occurredday.ClockIsDaySQL(read.clockRows, read.lastActivity)))
	}
	if len(parts) == 0 {
		return nil
	}
	days, err := readDayClocks(ctx, tx, strings.Join(parts, " UNION ALL "), args)
	if err != nil {
		return err
	}
	for key, standing := range standings {
		standing.LastActivityPrecision = occurredday.ClockPrecision[crmcontracts.OccurredPrecision](standing.LastActivityAt, days[key])
		standings[key] = standing
	}
	return nil
}

// readDayClocks runs statement in a savepoint under the short lock wait and
// keys each day clock it names. A lock or a budget it runs out of answers none.
func readDayClocks(ctx context.Context, tx pgx.Tx, statement string, args []any) (map[string]bool, error) {
	var previous string
	if err := tx.QueryRow(ctx, `SELECT current_setting('lock_timeout')`).Scan(&previous); err != nil {
		return nil, fmt.Errorf("search: reading the lock wait: %w", err)
	}
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	days, err := scanDayClocks(ctx, savepoint, statement, args)
	if storekit.IsLockTimeout(err) || storekit.IsQueryCanceled(err) {
		if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
			return nil, rollbackErr
		}
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, errors.Join(err, savepoint.Rollback(ctx))
	}
	if err := savepoint.Commit(ctx); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('lock_timeout', $1, true)`, previous); err != nil {
		return nil, fmt.Errorf("search: restoring the lock wait: %w", err)
	}
	return days, nil
}

func scanDayClocks(ctx context.Context, tx pgx.Tx, statement string, args []any) (map[string]bool, error) {
	if _, err := tx.Exec(ctx, `SELECT set_config('lock_timeout', $1, true)`, clockPrecisionLockWait); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := map[string]bool{}
	for rows.Next() {
		var hitType string
		var id ids.UUID
		if err := rows.Scan(&hitType, &id); err != nil {
			return nil, err
		}
		days[hitType+":"+id.String()] = true
	}
	return days, rows.Err()
}
