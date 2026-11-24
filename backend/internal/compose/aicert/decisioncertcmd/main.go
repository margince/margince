// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command decisioncertcmd writes two tables. internal/modules/ai/tiercert_gen.go
// holds the verdict of every ladder record.
// internal/modules/ai/decisioncert_gen.go lists the (task, site, provider, model)
// rows the decisions lane may serve. It has one row per decision record that is
// certified and still describes the question this build asks.
// `make gen` runs it, and `make drift` fails a table that disagrees with the records.
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
	out := flag.String("out", "internal/modules/ai/decisioncert_gen.go", "the generated table (relative to backend/)")
	tierOut := flag.String("tier-out", "internal/modules/ai/tiercert_gen.go", "the generated ladder verdict table (relative to backend/)")
	records := flag.String("records", "internal/compose/aicert/records", "directory of certification records (relative to backend/)")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus", "directory of certification scenarios (relative to backend/)")
	flag.Parse()
	if err := generate(*out, *tierOut, *records, *corpusDir); err != nil {
		fmt.Fprintf(os.Stderr, "decisioncertcmd: %v\n", err)
		os.Exit(1)
	}
}

func generate(out, tierOut, recordsDir, corpusDir string) error {
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
	if err := os.WriteFile(out, table, 0o600); err != nil {
		return err
	}
	tierRows, err := aicert.TierCertTable(context.Background(), corpus, census, records)
	if err != nil {
		return err
	}
	tiers, err := aicert.RenderTierCertTable(tierRows)
	if err != nil {
		return err
	}
	return os.WriteFile(tierOut, tiers, 0o600)
}
