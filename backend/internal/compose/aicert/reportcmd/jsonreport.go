// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/margince/margince/backend/internal/compose/aicert/snapshot"
)

// The machine-readable half of the readiness report.
//
// It serialises the SAME rows the text table prints — readinessRows already
// decided every status, count and band — so the card in Settings and the report
// on a terminal can never disagree about a site. Nothing is recomputed here; a
// second derivation would be a second answer.

// renderJSON writes the rows as the committed snapshot the product embeds.
//
// Deterministic: rows sorted by their key, two-space indent, trailing newline.
// The file is committed and diffed by a drift gate, so an unstable ordering
// would show up as a spurious change on every regeneration.
func renderJSON(recordsDir string, rows []readinessRow) ([]byte, error) {
	out := snapshot.Snapshot{GeneratedFrom: recordsDir, Rows: make([]snapshot.Row, 0, len(rows))}
	for _, row := range rows {
		out.Rows = append(out.Rows, row.snapshotRow())
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		return lessRow(out.Rows[i], out.Rows[j])
	})
	if err := refuseDuplicates(out.Rows); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding the certification snapshot: %w", err)
	}
	return append(encoded, '\n'), nil
}

// snapshotRow is one report row as the snapshot carries it. An absent row keeps
// its zeroed counts and empty band: the report prints a dash there, and a zero
// band word would be a measurement nobody made.
func (r readinessRow) snapshotRow() snapshot.Row {
	out := snapshot.Row{
		Task:     string(r.site.Task),
		Site:     r.site.Variant,
		Status:   r.status(),
		Scope:    r.claimedScope(),
		Measured: r.measured,
		Pending:  r.pending,
	}
	if !r.certified {
		return out
	}
	out.Provider = r.record.Provider
	out.Model = r.record.ServedModel
	out.EnvClass = r.record.EnvClass
	out.Band = r.tally.Verdict
	out.Runs = r.tally.Runs
	out.Passed = r.tally.Passed
	out.RanAt = r.record.RanAt
	return out
}

func lessRow(a, b snapshot.Row) bool {
	for _, pair := range [][2]string{
		{a.Task, b.Task},
		{a.Site, b.Site},
		{a.Provider, b.Provider},
		{a.Model, b.Model},
		{a.EnvClass, b.EnvClass},
	} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	return false
}

// refuseDuplicates stops a snapshot that answers one question twice.
//
// LoadRecords appends every record file it walks, and the text report
// deliberately prints a site once per binding that measured it. The snapshot's
// key admits exactly one row per (task, site, provider, model, env), so two here
// mean two record files claiming the same measurement — a generation fault worth
// naming, never a silent last-writer-wins.
//
// Delegated to the snapshot's own index rather than rebuilt here. A key joined
// with "/" would be WRONG: a model id contains slashes (openai/gpt-oss-120b), so
// two distinct rows can produce one joined string and read as a duplicate that
// the binary indexes separately. Asking the indexer is also the only way this
// check and the load-time one cannot disagree.
func refuseDuplicates(rows []snapshot.Row) error {
	_, err := snapshot.FromRows(rows)
	return err
}
