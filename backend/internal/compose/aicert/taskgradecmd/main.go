// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command taskgradecmd writes internal/modules/ai/taskgrades_gen.go: every
// completion record's grade on its task by the readiness report's rule.
// Settings shows it beside the task the record's model answers. `make gen` runs
// it, and `make drift` fails a committed table the records no longer give.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
)

func main() {
	out := flag.String("out", "internal/modules/ai/taskgrades_gen.go", "the generated table (relative to backend/)")
	records := flag.String("records", "internal/compose/aicert/records", "directory of certification records (relative to backend/)")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus", "directory of certification scenarios (relative to backend/)")
	flag.Parse()
	if err := generate(*out, *records, *corpusDir); err != nil {
		fmt.Fprintf(os.Stderr, "taskgradecmd: %v\n", err)
		os.Exit(1)
	}
}

func generate(out, recordsDir, corpusDir string) error {
	census, err := compose.NewTaskCensus()
	if err != nil {
		return fmt.Errorf("building the invocation-site census: %w", err)
	}
	corpus, err := aicert.LoadCorpus(corpusDir, census)
	if err != nil {
		return err
	}
	records, err := aicert.LoadRecords(recordsDir)
	if err != nil {
		return err
	}
	rows, err := aicert.TaskGrades(context.Background(), corpus, census, records)
	if err != nil {
		return err
	}
	table, err := aicert.RenderTaskGrades(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(out, table, 0o600)
}
