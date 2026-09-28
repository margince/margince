// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command decisioncertcmd writes internal/modules/ai/decisioncert_gen.go: the
// table of (task, site, provider, model) the decisions lane may serve, one row
// per committed decision record that is certified and still describes the
// decision question this build asks. `make gen` runs it, and `make drift` fails
// a committed table that disagrees with the records.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
)

func main() {
	out := flag.String("out", "internal/modules/ai/decisioncert_gen.go", "the generated table (relative to backend/)")
	records := flag.String("records", "internal/compose/aicert/records", "directory of certification records (relative to backend/)")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus", "directory of certification scenarios (relative to backend/)")
	flag.Parse()
	if err := generate(*out, *records, *corpusDir); err != nil {
		fmt.Fprintf(os.Stderr, "decisioncertcmd: %v\n", err)
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
	rows, _, err := aicert.DecisionCertTable(corpus, census, records)
	if err != nil {
		return err
	}
	table, err := aicert.RenderDecisionCertTable(rows)
	if err != nil {
		return err
	}
	return os.WriteFile(out, table, 0o600)
}
