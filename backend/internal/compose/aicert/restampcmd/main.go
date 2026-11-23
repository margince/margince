// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command restampcmd grades committed certification records again, with no new
// runs. It serves a grading-rule bump, or a change to how site checks are
// hashed, that moved nothing a model was asked. It is a go-run-only developer
// tool: `make e2e-ai-restamp` invokes it.
//
// It prints one line per record it re-stamps and one per record it leaves
// stale, with the reason. Without -write it changes nothing.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "restampcmd: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("restampcmd", flag.ContinueOnError)
	from := flags.String("from", "", "the grading rule the records were graded by, e.g. grading-rule-13")
	siteChecksFrom := flags.String("site-checks-from", "", "a site_checks.golden the records were stamped under, instead of -from")
	write := flags.Bool("write", false, "write the re-stamped records; without it the run only reports")
	dir := flags.String("dir", "internal/compose/aicert/records", "directory of certification records")
	corpusDir := flags.String("corpus", "internal/compose/aicert/corpus", "directory of certification scenarios")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (*from == "") == (*siteChecksFrom == "") {
		return errors.New("name the rule the records were graded by with -from, or their site checks with -site-checks-from")
	}
	census, err := compose.NewTaskCensus()
	if err != nil {
		return fmt.Errorf("building the invocation-site census: %w", err)
	}
	corpus, err := aicert.LoadCorpus(*corpusDir, census)
	if err != nil {
		return err
	}
	records, err := aicert.LoadRecords(*dir)
	if err != nil {
		return err
	}
	basis, err := restampBasis(*from, *siteChecksFrom, corpus, census)
	if err != nil {
		return err
	}
	report, changed := basis.RestampAll(records)
	for _, line := range report {
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			return err
		}
	}
	if *write {
		if err := writeRecords(*dir, changed); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "%d restamped, %d left stale (written: %t)\n", len(changed), len(report)-len(changed), *write)
	return err
}

func restampBasis(from, siteChecksFrom string, corpus []aicert.Scenario, census *aitasks.Registry) (aicert.RestampBasis, error) {
	if from != "" {
		return aicert.NewRestampBasis(context.Background(), from, corpus, census)
	}
	golden, err := os.ReadFile(siteChecksFrom) // #nosec G304 -- a developer names the file on the command line of a go-run-only tool
	if err != nil {
		return aicert.RestampBasis{}, fmt.Errorf("reading the earlier site checks: %w", err)
	}
	return aicert.NewSiteCheckRestampBasis(context.Background(), string(golden), corpus, census)
}

func writeRecords(dir string, records []aicert.Record) error {
	for _, rec := range records {
		if err := aicert.WriteRecord(dir, rec); err != nil {
			return err
		}
	}
	return nil
}
