// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command forecastevalcmd scores the forecasts an installation froze against
// what its periods went on to win, and prints the report as JSON. It reads
// the database once, in one bounded read-only transaction, or a dataset file
// an earlier run wrote. Given one dataset and one plan, it prints the same
// report every time.
//
// A developer and operator tool run with `go run`, not a deployed role:
// docs/reference/forecast-model-evaluation.md lists its flags.
package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/database"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "forecastevalcmd: %v\n", err)
		os.Exit(1)
	}
}

type options struct {
	dsn, datasetIn, datasetOut string
	asOf                       time.Time
	plan                       forecasting.EvaluationPlan
	bounds                     compose.ForecastEvaluationBounds
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	opts, err := parseOptions(args, getenv)
	if err != nil {
		return err
	}
	dataset, err := loadDataset(ctx, opts)
	if err != nil {
		return err
	}
	if opts.datasetOut != "" {
		if err := writeDataset(opts.datasetOut, dataset); err != nil {
			return err
		}
	}
	report, err := forecasting.Evaluate(dataset, opts.plan)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func parseOptions(args []string, getenv func(string) string) (options, error) {
	fs := flag.NewFlagSet("forecastevalcmd", flag.ContinueOnError)
	var opts options
	var trainEnd, validationEnd, edges, asOf string
	fs.StringVar(&opts.dsn, "dsn", "", "database to read; MARGINCE_DSN when unset")
	fs.StringVar(&opts.datasetIn, "dataset", "", "score this dataset file instead of reading the database")
	fs.StringVar(&opts.datasetOut, "write-dataset", "", "write the dataset read from the database to this file")
	fs.StringVar(&asOf, "as-of", "", "instant outcomes are read at, RFC 3339; now when unset")
	fs.StringVar(&trainEnd, "train-end", "", "first local day after the training window, YYYY-MM-DD")
	fs.StringVar(&validationEnd, "validation-end", "", "first local day of the test window, YYYY-MM-DD")
	fs.StringVar(&edges, "horizon-edges", "", "horizon bucket edges in days, such as 30,60")
	fs.IntVar(&opts.plan.SettleDays, "settle-days", 0, "days after a period ends before its outcome counts")
	fs.IntVar(&opts.bounds.Vintages, "max-vintages", 2000, "most snapshots one database read takes")
	fs.IntVar(&opts.bounds.Deals, "max-deals", 500000, "most deal rows one database read takes")
	fs.DurationVar(&opts.bounds.Statement, "statement-timeout", 30*time.Second, "ceiling of each statement")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	var err error
	if opts.plan.TrainEnd, err = parseDay("train-end", trainEnd); err != nil {
		return options{}, err
	}
	if opts.plan.ValidationEnd, err = parseDay("validation-end", validationEnd); err != nil {
		return options{}, err
	}
	if opts.plan.HorizonEdges, err = parseEdges(edges); err != nil {
		return options{}, err
	}
	opts.asOf = time.Now().UTC()
	if asOf != "" {
		if opts.asOf, err = time.Parse(time.RFC3339, asOf); err != nil {
			return options{}, fmt.Errorf("-as-of %q is not an RFC 3339 instant: %w", asOf, err)
		}
	}
	if opts.datasetIn == "" {
		opts.dsn = cmp.Or(opts.dsn, getenv("MARGINCE_DSN"))
		if opts.dsn == "" {
			return options{}, errors.New("pass -dataset, or -dsn or MARGINCE_DSN to read the database")
		}
	}
	return opts, nil
}

func parseDay(flagName, raw string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("-%s %q is not a YYYY-MM-DD day", flagName, raw)
	}
	return day, nil
}

func parseEdges(raw string) ([]int, error) {
	if raw == "" {
		return nil, nil
	}
	var edges []int
	for part := range strings.SplitSeq(raw, ",") {
		edge, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("-horizon-edges %q is not a list of whole days", raw)
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func loadDataset(ctx context.Context, opts options) (forecasting.EvaluationDataset, error) {
	if opts.datasetIn != "" {
		body, err := os.ReadFile(opts.datasetIn)
		if err != nil {
			return forecasting.EvaluationDataset{}, fmt.Errorf("reading the dataset: %w", err)
		}
		var dataset forecasting.EvaluationDataset
		if err := json.Unmarshal(body, &dataset); err != nil {
			return forecasting.EvaluationDataset{}, fmt.Errorf("the dataset file is not a dataset: %w", err)
		}
		return dataset, nil
	}
	pool, err := database.NewPool(ctx, opts.dsn)
	if err != nil {
		return forecasting.EvaluationDataset{}, err
	}
	defer pool.Close()
	return compose.ForecastEvaluationDataset(ctx, pool, opts.asOf, opts.bounds)
}

func writeDataset(path string, dataset forecasting.EvaluationDataset) error {
	body, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the dataset: %w", err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil { // #nosec G703 -- the operator's own -write-dataset path; writing it is this flag's purpose
		return fmt.Errorf("writing the dataset: %w", err)
	}
	return nil
}
