// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command restampcmd grades committed certification records again after a
// grading-rule bump that moved nothing a model was asked, with no new runs. It
// is a go-run-only developer tool: `make e2e-ai-restamp` invokes it.
//
// It prints one line per record it re-stamps and one per record it leaves
// stale, with the reason. Without -write it changes nothing.
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
	from := flag.String("from", "", "the grading rule the records were graded by, e.g. grading-rule-13 (required)")
	write := flag.Bool("write", false, "write the re-stamped records; without it the run only reports")
	dir := flag.String("dir", "internal/compose/aicert/records", "directory of certification records")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus", "directory of certification scenarios")
	flag.Parse()
	if *from == "" {
		fail(fmt.Errorf("name the rule the records were graded by with -from"))
	}

	census, err := compose.NewTaskCensus()
	if err != nil {
		fail(fmt.Errorf("building the invocation-site census: %w", err))
	}
	corpus, err := aicert.LoadCorpus(*corpusDir, census)
	if err != nil {
		fail(err)
	}
	records, err := aicert.LoadRecords(*dir)
	if err != nil {
		fail(err)
	}
	basis, err := aicert.NewRestampBasis(context.Background(), *from, corpus, census)
	if err != nil {
		fail(err)
	}

	restamped, refused := 0, 0
	for _, rec := range records {
		if rec.Kind == aicert.KindDecision {
			continue
		}
		got := basis.Restamp(rec)
		switch {
		case got.Refusal != "":
			refused++
			fmt.Printf("stale    %s: %s\n", aicert.RecordKey(rec), got.Refusal) //nolint:forbidigo // the tool's report, printed to stdout
		case got.Changed:
			restamped++
			fmt.Printf("restamp  %s: %s -> %s\n", aicert.RecordKey(rec), rec.Verdict, got.Record.Verdict) //nolint:forbidigo // the tool's report, printed to stdout
			if *write {
				if err := aicert.WriteRecord(*dir, got.Record); err != nil {
					fail(err)
				}
			}
		}
	}
	fmt.Printf("%d restamped, %d left stale (written: %t)\n", restamped, refused, *write) //nolint:forbidigo // the tool's report, printed to stdout
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "restampcmd: %v\n", err)
	os.Exit(1)
}
