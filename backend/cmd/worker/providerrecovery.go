// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// `worker reopen-parked` — reopen the capture work a provider outage parked.
//
// While a provider was down, sender questions spent their attempts and were
// retired to the review queue with no judgement, and company enrichment ran out
// of attempts and stopped. This puts both back in the queue for the window the
// operator names, once the provider answers again. A subcommand rather than an
// endpoint for the reason authz-disagreement is one: somebody decides it once,
// after an outage, on a window only they know.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// runReopenParked parses the window and prints what it reopened, or with
// --dry-run what it would.
func runReopenParked(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("reopen-parked", flag.ContinueOnError)
	fs.SetOutput(stdout)
	from := fs.String("from", "", "start of the outage window, RFC3339 (required)")
	to := fs.String("to", "", "end of the outage window, RFC3339, exclusive (required)")
	dryRun := fs.Bool("dry-run", false, "count what would be reopened and change nothing")
	batch := fs.Int("batch", compose.DefaultRecoveryBatch, "most rows of each kind to reopen per workspace in one run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("reopen-parked: unexpected argument %q; every option is a --flag", fs.Arg(0))
	}
	window, err := parseReopenWindow(*from, *to)
	if err != nil {
		return err
	}
	if *batch <= 0 {
		return fmt.Errorf("reopen-parked: --batch must be positive")
	}

	recovery := compose.NewProviderRecovery(pool)
	if *dryRun {
		found, err := recovery.Count(ctx, window)
		if err != nil {
			return err
		}
		return writeParked(stdout, "Would reopen", found, 0)
	}
	reopened, err := recovery.Reopen(ctx, window, *batch)
	if err != nil {
		return err
	}
	left, err := recovery.Count(ctx, window)
	if err != nil {
		return err
	}
	return writeParked(stdout, "Reopened", reopened, left.Counterparties+left.Enrichments)
}

func parseReopenWindow(from, to string) (capture.ReopenWindow, error) {
	if from == "" || to == "" {
		return capture.ReopenWindow{}, fmt.Errorf("reopen-parked: --from and --to are required (RFC3339)")
	}
	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return capture.ReopenWindow{}, fmt.Errorf("reopen-parked: --from is not RFC3339: %w", err)
	}
	end, err := time.Parse(time.RFC3339, to)
	if err != nil {
		return capture.ReopenWindow{}, fmt.Errorf("reopen-parked: --to is not RFC3339: %w", err)
	}
	window := capture.ReopenWindow{From: start, To: end}
	if err := window.Validate(); err != nil {
		return capture.ReopenWindow{}, fmt.Errorf("reopen-parked: %w", err)
	}
	return window, nil
}

// writeParked prints one line per kind, and for a real run how many remain in
// the window, so an operator knows whether another run is owed.
func writeParked(stdout io.Writer, verb string, work compose.ParkedWork, remaining int) error {
	if _, err := fmt.Fprintf(stdout, "%s %d sender question(s) and %d company enrichment(s).\n",
		verb, work.Counterparties, work.Enrichments); err != nil {
		return fmt.Errorf("reopen-parked: writing the report: %w", err)
	}
	if remaining > 0 {
		if _, err := fmt.Fprintf(stdout, "%d more remain in the window; run it again to continue.\n", remaining); err != nil {
			return fmt.Errorf("reopen-parked: writing the report: %w", err)
		}
	}
	return nil
}
