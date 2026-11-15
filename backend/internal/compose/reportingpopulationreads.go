// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type reportingPopulationKey struct {
	kind    string
	subject ids.UUID
	teams   string
}

type reportingPopulationFact struct {
	label  string
	member bool
}

type reportingPopulationReads struct {
	root     pgx.Tx
	closed   bool
	checked  bool
	snapshot bool
	facts    map[reportingPopulationKey]reportingPopulationFact
}

type reportingPopulationReadsKey struct{}

// withReportingPopulationReads opens the reads for one evaluation on tx. An
// evaluation that already has open reads on the same connection keeps them, and
// its release leaves them to the caller that opened them.
func withReportingPopulationReads(ctx context.Context, tx pgx.Tx) (context.Context, func()) {
	if open, ok := ctx.Value(reportingPopulationReadsKey{}).(*reportingPopulationReads); ok && !open.closed && open.root != nil && tx != nil && open.root.Conn() == tx.Conn() {
		return ctx, func() {}
	}
	reads := &reportingPopulationReads{root: tx, facts: make(map[reportingPopulationKey]reportingPopulationFact)}
	return context.WithValue(ctx, reportingPopulationReadsKey{}, reads), func() {
		reads.closed = true
		reads.facts = nil
	}
}

// Metric savepoints share one snapshot; callers outside this evaluation do not.
func readReportingPopulationFact(ctx context.Context, tx pgx.Tx, key reportingPopulationKey, read func() (reportingPopulationFact, error)) (reportingPopulationFact, error) {
	reads, ok := ctx.Value(reportingPopulationReadsKey{}).(*reportingPopulationReads)
	if !ok || reads.closed || reads.root == nil || tx == nil || ctx.Err() != nil || tx.Conn() != reads.root.Conn() {
		return read()
	}
	if !reads.checked {
		var isolation string
		if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation')`).Scan(&isolation); err != nil {
			return reportingPopulationFact{}, fmt.Errorf("compose: reading the reporting population snapshot: %w", err)
		}
		reads.snapshot = isolation == "repeatable read" || isolation == "serializable"
		reads.checked = true
	}
	if !reads.snapshot {
		return read()
	}
	if fact, found := reads.facts[key]; found {
		return fact, nil
	}
	fact, err := read()
	if err == nil {
		reads.facts[key] = fact
	}
	return fact, err
}

func reportingPopulationTeams(teams []ids.UUID) string {
	keys := make([]string, len(teams))
	for i, id := range teams {
		keys[i] = id.String()
	}
	slices.Sort(keys)
	return strings.Join(slices.Compact(keys), ",")
}
