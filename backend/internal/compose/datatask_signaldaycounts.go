// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// signalDayCountsDataTask rewords the ghosted-thread and quiet-project
// summaries stored with the day count of their first scan. Only a summary in
// the old scan wording is rewritten; a reworded one leaves the match, so a
// replay changes nothing.
func signalDayCountsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793028172_signal_day_counts", Version: 1,
		Title: "Reword signal summaries that froze a day count",
		Steps: []datatask.Step{{
			Name: "reword_signal_day_counts", Limit: 200, MinLimit: 20,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateSignalDayCounts, Batch: rewordSignalDayCountsBatch,
		}},
	}
}

// legacySignalSummaryByLang is the wording the scan wrote before it stopped
// storing an age. Kept only to recognise those rows.
var legacySignalSummaryByLang = map[textlang.Lang]signalSummaryCopy{
	textlang.English: {
		ghostedThread: "We wrote %d days ago and nobody has answered.",
		projectQuiet:  "Nothing has been filed on %s for %d days.",
	},
	textlang.German: {
		ghostedThread: "Wir haben vor %d Tagen geschrieben und niemand hat geantwortet.",
		projectQuiet:  "Zu %s wurde seit %d Tagen nichts abgelegt.",
	},
	textlang.Vietnamese: {
		ghostedThread: "Chúng ta đã viết cách đây %d ngày và chưa ai trả lời.",
		projectQuiet:  "Không có gì được lưu cho %s trong %d ngày.",
	},
}

// legacySummaryPattern matches one legacy sentence, capturing a %s as the name.
func legacySummaryPattern(template string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(template)
	quoted = strings.Replace(quoted, "%s", "(.+)", 1)
	return regexp.MustCompile("^" + strings.Replace(quoted, "%d", `\d+`, 1) + "$")
}

// rewordedDaySummary answers the current wording for a legacy summary of this
// kind, in the language it was written in.
func rewordedDaySummary(kind, summary string) (string, bool) {
	for lang, legacy := range legacySignalSummaryByLang {
		said := signalSummaryCopyFor(lang)
		switch kind {
		case kindGhostedThread:
			if legacySummaryPattern(legacy.ghostedThread).MatchString(summary) {
				return fmt.Sprintf(said.ghostedThread, ghostedThresholdDays), true
			}
		case kindProjectGoneQuiet:
			if m := legacySummaryPattern(legacy.projectQuiet).FindStringSubmatch(summary); m != nil {
				return fmt.Sprintf(said.projectQuiet, m[1], projects.DefaultProjectQuietDays), true
			}
		}
	}
	return "", false
}

const signalDayCountScope = `FROM signal s
 WHERE s.source = 'signal-scan' AND s.kind IN ('` + kindGhostedThread + `', '` + kindProjectGoneQuiet + `')`

type storedSignalSummary struct {
	ID      ids.UUID
	Kind    string
	Summary string
}

func rewordSignalDayCountsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id, s.kind, s.summary `+signalDayCountScope+`
		AND s.id > @counted_after ORDER BY s.id LIMIT @counted_limit`,
		pgx.NamedArgs{"counted_after": after, "counted_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedSignalSummary])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, candidate := range candidates {
		scanned = append(scanned, candidate.ID)
		to, legacy := rewordedDaySummary(candidate.Kind, candidate.Summary)
		if !legacy {
			continue
		}
		reworded, err := signals.RewordScanSummaryTx(ctx, tx, signals.ScanSummaryReword{
			Signal: candidate.ID, From: candidate.Summary, To: to,
		})
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if reworded {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func estimateSignalDayCounts(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT s.id "+signalDayCountScope+" LIMIT @counted_cap) candidates",
		pgx.NamedArgs{"counted_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
