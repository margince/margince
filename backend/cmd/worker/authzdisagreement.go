// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// `worker authz-disagreement` — how far apart the outbound engine and the old
// purpose gate have been.
//
// The engine has run in observe mode since it shipped: it decides, records, and
// the old gate rules. Ending that is a decision somebody has to make on
// evidence, and this is the evidence. Enforcing a rule nobody has measured is
// how a compliance change becomes an outage.
//
// A subcommand rather than an endpoint because the question is asked once,
// before a rollout, by whoever is deciding — not by the product on every page
// load. It reads and writes nothing.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// runAuthzDisagreement prints the report, strictest-first.
//
// The ORDER is the point: an operator asking "what does enforcing cost me" is
// asking about mail that stops, so the rows where the engine refuses what the
// old gate allowed come first and carry a marker. The other direction is real
// and worth seeing — it is mail enforcement would START allowing — but it is
// not what makes a rollout dangerous.
func runAuthzDisagreement(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("authz-disagreement", flag.ContinueOnError)
	fs.SetOutput(stdout)
	workspace := fs.String("workspace", "", "workspace id to report on (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workspace == "" {
		return errors.New("authz-disagreement: --workspace is required")
	}
	wsID, err := ids.Parse(*workspace)
	if err != nil {
		return fmt.Errorf("authz-disagreement: --workspace is not an id: %w", err)
	}

	// The system principal, because this is the installation asking about its
	// own rollout rather than a seat asking about a contact. Nothing it returns
	// names a subject: no address, no consent state, only how two rules have
	// compared.
	ctx = principal.WithWorkspaceID(ctx, wsID)
	ctx = principal.SystemActing(ctx, "system:authz_disagreement")

	store := consent.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](wsID)))
	report, err := store.DisagreementReport(ctx)
	if err != nil {
		return err
	}
	if len(report) == 0 {
		_, _ = fmt.Fprintln(stdout, "The engine and the old purpose gate have not disagreed about any delivery.")
		return nil
	}
	return writeDisagreements(stdout, report)
}

// writeDisagreements renders the table and the one-line summary under it.
func writeDisagreements(stdout io.Writer, report []consent.Disagreement) error {
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "DIRECTION\tCATEGORY\tREASON\tENGINE\tOLD GATE\tMESSAGES\tRECIPIENTS")
	var stopping, starting int
	for _, d := range report {
		direction := "would start"
		if d.EngineIsStricter {
			direction = "WOULD STOP"
			stopping += d.Deliveries
		} else {
			starting += d.Deliveries
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			direction, d.Category, d.ReasonCode, d.EngineVerdict, d.LegacyVerdict,
			d.Deliveries, d.Decisions)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("authz-disagreement: writing the report: %w", err)
	}
	_, _ = fmt.Fprintf(stdout,
		"\nEnforcing the engine today would stop %d message(s) that went out, and allow %d that did not.\n",
		stopping, starting)
	return nil
}

// databaseCommand is one operator command that runs against the installation
// and exits instead of starting the worker.
type databaseCommand func(
	ctx context.Context, pool *pgxpool.Pool, resolveVault func() (keyvault.Vault, error),
	settings *config.Config, args []string, stdout io.Writer,
) error

// databaseCommands are dispatched by name. The boot reads the same names to keep
// a command and its flags away from the worker's flag parse.
var databaseCommands = map[string]databaseCommand{
	"authz-disagreement": poolOnly(runAuthzDisagreement),
	"deal-key-names":     poolOnly(runDealKeyNames),
	"replay-erasures":    runReplayErasures,
	"reopen-parked":      poolOnly(runReopenParked),
}

// poolOnly adapts a command that needs only the pool.
func poolOnly(run func(context.Context, *pgxpool.Pool, []string, io.Writer) error) databaseCommand {
	return func(ctx context.Context, pool *pgxpool.Pool, _ func() (keyvault.Vault, error), _ *config.Config, args []string, stdout io.Writer) error {
		return run(ctx, pool, args, stdout)
	}
}

// databaseCommandOf names the database command args start with, if any.
func databaseCommandOf(args []string) (databaseCommand, bool) {
	if len(args) == 0 {
		return nil, false
	}
	command, ok := databaseCommands[args[0]]
	return command, ok
}

// workerFlags is the part of the command line the worker's own flag parse reads.
// A database command takes its settings from the environment and files; its
// name and flags are its own, and that parse refuses both.
func workerFlags(args []string) []string {
	if _, ok := databaseCommandOf(args); ok {
		return nil
	}
	return args
}

// runDatabaseSubcommand dispatches the subcommands that need a database, and
// reports whether it handled the arguments.
//
// Separate from runDebugSubcommand (boot.go) because of WHEN it runs, not what
// it contains: those loops take no DSN and run before the worker flags, while
// these read or write the installation's own rows and must run after the role
// and release assertions — a report against a schema this binary does not agree
// with is a report about something else, and a write against one corrupts it.
func runDatabaseSubcommand(
	ctx context.Context, pool *pgxpool.Pool, resolveVault func() (keyvault.Vault, error), settings *config.Config, args []string, stdout io.Writer,
) (bool, error) {
	command, ok := databaseCommandOf(args)
	if !ok {
		return false, nil
	}
	return true, command(ctx, pool, resolveVault, settings, args[1:], stdout)
}
