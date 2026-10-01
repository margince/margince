// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The per-preset half of the report: for each task, the rung that answers and
// the one a failed call falls to, each with its record's state. What a state
// means belongs to aicert.PresetRungs; this file owns the columns.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// presetReport is one preset file and the rung states of its tasks.
type presetReport struct {
	File  string
	Rungs []aicert.PresetRungState
}

// loadPresets reads every *.yaml under dir as a deploy config's routing. A file
// that does not parse is an error: a report that skipped it would describe a
// tree without the preset an operator is about to deploy.
func loadPresets(dir string) (map[string]ai.RoutingConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading presets: %w", err)
	}
	presets := map[string]ai.RoutingConfig{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 -- a preset under the directory the operator named
		if err != nil {
			return nil, fmt.Errorf("reading preset %s: %w", entry.Name(), err)
		}
		routing, err := ai.ParsePreset(raw)
		if err != nil {
			return nil, fmt.Errorf("preset %s: %w", entry.Name(), err)
		}
		presets[entry.Name()] = routing
	}
	return presets, nil
}

// renderPresets prints one table per preset, sorted by file name.
func renderPresets(reports []presetReport) string {
	sort.Slice(reports, func(i, j int) bool { return reports[i].File < reports[j].File })
	var b strings.Builder
	b.WriteString("\nBy preset: the rung that answers each task, and the one a failed call falls to.\n")
	for _, report := range reports {
		fmt.Fprintf(&b, "\n%s\n", report.File)
		if err := writePresetTable(&b, report.Rungs); err != nil {
			fmt.Fprintf(&b, "(table could not be written: %v)\n", err)
		}
	}
	return b.String()
}

// writePresetTable writes one preset's rows to b; a strings.Builder never fails
// a write, but the tabwriter's own error is still the caller's to print.
func writePresetTable(b *strings.Builder, rungs []aicert.PresetRungState) error {
	w := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "TASK\tFIRST RUNG\tFALLBACK"); err != nil {
		return err
	}
	for _, r := range rungs {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", r.Task, rungCell(r.FirstTier, r.FirstModel, r.FirstState), fallbackCell(r)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func fallbackCell(r aicert.PresetRungState) string {
	if r.FallbackState == aicert.RungSameModel || r.FallbackState == aicert.RungNone {
		return r.FallbackState
	}
	return rungCell(r.FallbackTier, r.FallbackModel, r.FallbackState)
}

func rungCell(tier ai.Tier, model, state string) string {
	return fmt.Sprintf("%s · %s · %s", tier, model, state)
}
