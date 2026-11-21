// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command attemptlimitcmd writes internal/modules/ai/attemptlimits_gen.go: the
// per-try limit of each interactive task, from the p95 latency of the slowest
// model a committed record lets serve it. `make gen` runs it, and `make drift`
// fails a committed table that disagrees with the records.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/margince/margince/backend/internal/compose/aicert"
)

func main() {
	out := flag.String("out", "internal/modules/ai/attemptlimits_gen.go", "the generated table (relative to backend/)")
	records := flag.String("records", "internal/compose/aicert/records", "directory of certification records (relative to backend/)")
	flag.Parse()
	if err := generate(*out, *records); err != nil {
		fmt.Fprintf(os.Stderr, "attemptlimitcmd: %v\n", err)
		os.Exit(1)
	}
}

func generate(out, recordsDir string) error {
	records, err := aicert.LoadRecords(recordsDir)
	if err != nil {
		return err
	}
	table, err := aicert.RenderAttemptLimits(aicert.AttemptLimitRows(records))
	if err != nil {
		return err
	}
	return os.WriteFile(out, table, 0o600)
}
