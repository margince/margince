// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package migrations embeds the SQL migration namespaces (ADR-0017):
// core/ is upstream-owned and named for the unix second the migration was
// written; custom/ is fork-owned and stamped YYYYMMDDHHMMSS; jurisdiction
// packs bring their own third namespace. Apply order is always core, then
// custom, then packs.
//
// core/ carries two version shapes. 0001 is the BASELINE: the schema core
// builds, as the shape it arrives at rather than the path it took, in one
// file whose ORDER is a dependency order — extensions and the ext schema,
// the functions a column default can call, every table, the functions whose
// bodies read a table, then every constraint, index, trigger, grant and
// reference row, because a foreign key names two tables and contact/lead
// reference each other. Everything after the baseline is named for the unix
// second it was written. The baseline's version is zero-padded, so it sorts
// below every ten-digit stamp, and the runner's string ordering puts the
// two in the order they were written.
//
// A database built by migrations that predate the baseline cannot be moved
// onto it: the baseline reuses 0001, its ledger records 0001 under another
// name, and dbmigrate stops there rather than skip a migration as done. That
// stop is the intended outcome — rebuild the database (make dev-fresh).
//
// baseline/core/ holds a second baseline, for the installations that must be
// kept: one file that builds what core's versions through a release built, and
// replaces.txt, which names that release and the versions the file stands for.
// The replaced files stay in core/, so a database that applied them upgrades
// as it always did, and dbmigrate records the baseline beside them.
//
// What the baseline BUILDS is committed too, in testdata/head_catalog.txt,
// and TestMigrationsBuildTheCommittedSchema compares the two. So a migration
// added here shows its schema effect as a diff a reviewer can read, and the
// next consolidation is checkable the same way this one was
// (scripts/migration-baseline.sh verify).
package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

//go:embed core custom
var files embed.FS

// baselines is kept apart from files because it holds no namespace: each
// directory is the baseline OF one, and the gates walk files as namespaces.
//
//go:embed baseline
var baselines embed.FS

// renumbered declares the migrations that moved to a new version with
// identical bytes. Up moves a database's ledger row along such a pair instead
// of running the migration again.
//
//go:embed renumbered.txt
var renumbered string

// Core returns the upstream-owned namespace, as an installation applies it:
// an empty database takes the baseline instead of the versions it replaces.
func Core() (dbmigrate.Namespace, error) {
	ns, err := CoreHistory()
	if err != nil {
		return dbmigrate.Namespace{}, err
	}
	if ns.Baseline, err = baseline("core"); err != nil {
		return dbmigrate.Namespace{}, err
	}
	return ns, ns.CheckBaseline()
}

// CoreHistory returns the upstream-owned namespace without its baseline, so
// even an empty database replays every version. The tests that migrate to a
// version part-way through the history, and scripts/migration-baseline.sh,
// take this route.
func CoreHistory() (dbmigrate.Namespace, error) {
	return namespace("core")
}

// Custom returns the fork-owned namespace. Empty upstream by design: a
// fork's agent-authored migrations land here with x_-prefixed columns and
// never collide with a core upgrade (ADR-0017 Amendment 1).
func Custom() (dbmigrate.Namespace, error) {
	return namespace("custom")
}

func namespace(name string) (dbmigrate.Namespace, error) {
	ms, err := dbmigrate.Load(files, name)
	if err != nil {
		return dbmigrate.Namespace{}, err
	}
	pairs, err := Renumbered()
	if err != nil {
		return dbmigrate.Namespace{}, err
	}
	return dbmigrate.Namespace{Name: name, Migrations: ms, Renumbered: pairs[name]}, nil
}

// Renumbered reads renumbered.txt into its pairs, keyed by namespace. Each
// line is `namespace old_version new_version name old_digest`; a line
// starting with # is a comment.
func Renumbered() (map[string][]dbmigrate.Renumber, error) {
	return dbmigrate.ParseRenumbers(renumbered)
}

// baseline reads baseline/<name>/: one up file and the replaces.txt manifest.
// It has no down file, because Down reverts the replaced versions instead.
func baseline(name string) (*dbmigrate.Baseline, error) {
	dir := "baseline/" + name
	ups, err := fs.Glob(baselines, dir+"/*.up.sql")
	if err != nil || len(ups) != 1 {
		return nil, fmt.Errorf("migrations: %s: want exactly one baseline .up.sql, found %d (%v)", dir, len(ups), err)
	}
	up, err := fs.ReadFile(baselines, ups[0])
	if err != nil {
		return nil, err
	}
	manifest, err := fs.ReadFile(baselines, dir+"/replaces.txt")
	if err != nil {
		return nil, err
	}
	release, replaces, err := dbmigrate.ParseBaselineManifest(string(manifest))
	if err != nil {
		return nil, fmt.Errorf("migrations: %s: %w", dir, err)
	}
	version, title, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(ups[0], dir+"/"), ".up.sql"), "_")
	if !ok {
		return nil, fmt.Errorf("migrations: %s: want <version>_<name>.up.sql", ups[0])
	}
	return &dbmigrate.Baseline{
		Migration: dbmigrate.Migration{Version: version, Name: title, UpSQL: string(up)},
		Release:   release,
		Replaces:  replaces,
	}, nil
}
