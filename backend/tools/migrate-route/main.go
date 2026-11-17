// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command migrate-route migrates the database MARGINCE_OWNER_DSN names along
// one route. scripts/migration-baseline.sh compares what the routes build.
// `-route baseline` is what `migrate up` does. `-route history` replays every
// core version instead. `-through V` stops the history at core version V and
// leaves custom out: the schema a baseline for V must build.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate-route:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("migrate-route", flag.ContinueOnError)
	route := flags.String("route", "", "baseline or history")
	through := flags.String("through", "", "history only: the last core version to apply; custom is left out")
	if err := flags.Parse(args); err != nil {
		return err
	}
	namespaces, err := routeNamespaces(*route, *through)
	if err != nil {
		return err
	}
	dsn := os.Getenv("MARGINCE_OWNER_DSN")
	if dsn == "" {
		return errors.New("set MARGINCE_OWNER_DSN to the database to migrate")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "migrate-route: closing:", err)
		}
	}()
	applied, err := dbmigrate.Up(ctx, conn, namespaces...)
	if err != nil {
		return err
	}
	fmt.Printf("migrate-route: %s route applied %d migration(s)\n", *route, applied)
	return nil
}

func routeNamespaces(route, through string) ([]dbmigrate.Namespace, error) {
	var core dbmigrate.Namespace
	var err error
	switch route {
	case "baseline":
		if through != "" {
			return nil, errors.New("-through applies to the history route only")
		}
		core, err = migrations.Core()
	case "history":
		core, err = migrations.CoreHistory()
	default:
		return nil, errors.New("-route must be baseline or history")
	}
	if err != nil {
		return nil, err
	}
	if through != "" {
		i := slices.IndexFunc(core.Migrations, func(m dbmigrate.Migration) bool { return m.Version == through })
		if i < 0 {
			return nil, fmt.Errorf("no core migration has version %s", through)
		}
		core.Migrations = core.Migrations[:i+1]
		return []dbmigrate.Namespace{core}, nil
	}
	custom, err := migrations.Custom()
	if err != nil {
		return nil, err
	}
	return []dbmigrate.Namespace{core, custom}, nil
}
