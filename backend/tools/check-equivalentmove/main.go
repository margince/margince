// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-equivalentmove answers whether a database that applied a
// migration at its old version is admitted at the new one after the SQL
// changed. scripts/check-migration-versions.sh asks it about a declared
// renumber whose bytes differ, so the gate and the migrator read one list of
// checked equivalences. It exits 0 only when that list admits the move.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

const (
	admitted    = 0
	notAdmitted = 1
	cannotTell  = 2
)

func main() {
	code, verdict := run(os.Args[1:])
	fmt.Println(verdict)
	os.Exit(code)
}

func run(args []string) (int, string) {
	flags := flag.NewFlagSet("check-equivalentmove", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	namespace := flags.String("namespace", "", "the ledger namespace, e.g. core or ext_notes")
	name := flags.String("name", "", "the migration's name, the same at both versions")
	oldVersion := flags.String("old", "", "the version the database applied")
	newVersion := flags.String("new", "", "the version the migration ships at now")
	oldUp := flags.String("old-up", "", "the up half as the database applied it")
	oldDown := flags.String("old-down", "", "the down half as the database applied it")
	newUp := flags.String("new-up", "", "the up half that ships")
	newDown := flags.String("new-down", "", "the down half that ships")
	if err := flags.Parse(args); err != nil {
		return cannotTell, "check-equivalentmove: " + err.Error()
	}

	applied, err := migrationFrom(*namespace, *oldVersion, *name, *oldUp, *oldDown)
	var moved dbmigrate.Migration
	if err == nil {
		moved, err = migrationFrom(*namespace, *newVersion, *name, *newUp, *newDown)
	}
	if err != nil {
		return cannotTell, "check-equivalentmove: " + err.Error()
	}
	verdict, code := "no checked equivalence admits", notAdmitted
	if dbmigrate.ContentAdmitted(*namespace, moved, dbmigrate.Digest(applied)) {
		verdict, code = "a checked equivalence admits", admitted
	}
	return code, fmt.Sprintf("%s %s_%s: %s its content at %s", *namespace, applied.Version, *name, verdict, moved.Version)
}

func migrationFrom(namespace, version, name, upPath, downPath string) (dbmigrate.Migration, error) {
	if namespace == "" || version == "" || name == "" || upPath == "" || downPath == "" {
		return dbmigrate.Migration{}, errors.New("-namespace, -name, -old, -new and all four SQL files are required")
	}
	up, err := os.ReadFile(upPath) //nolint:gosec // the path is a flag the gate passed
	if err != nil {
		return dbmigrate.Migration{}, err
	}
	down, err := os.ReadFile(downPath) //nolint:gosec // the path is a flag the gate passed
	if err != nil {
		return dbmigrate.Migration{}, err
	}
	return dbmigrate.Migration{Version: version, Name: name, UpSQL: string(up), DownSQL: string(down)}, nil
}
