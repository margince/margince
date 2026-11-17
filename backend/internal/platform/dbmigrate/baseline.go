// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// A baseline is one file that builds the schema a run of older versions built,
// so a new installation applies it instead of replaying them. It never replaces
// their ledger rows: datatask requirements and the activity repairs read old
// versions and their applied_at, so a baseline install records every replaced
// version as well.

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Baseline is the file that stands for Replaces, and the release a database
// part-way through them must reach first.
type Baseline struct {
	Migration Migration
	Release   string
	Replaces  []Replaced
}

// Replaced is one version a baseline stands for, with the name and digest a
// baseline install records for it. They are the old file's own name and Digest,
// so assertLedgerMatches and assertContentMatches hold on either route, and
// they are declared rather than read off the file so the file can go later.
type Replaced struct {
	Version string
	Name    string
	Digest  string
}

// ParseBaselineManifest reads a `release <name>` line and then one
// `version name digest` line per replaced version; # starts a comment.
func ParseBaselineManifest(text string) (release string, replaces []Replaced, err error) {
	lines := bufio.NewScanner(strings.NewReader(text))
	for n := 1; lines.Scan(); n++ {
		fields := strings.Fields(lines.Text())
		switch {
		case len(fields) == 0 || strings.HasPrefix(fields[0], "#"):
			continue
		case len(fields) == 2 && fields[0] == "release" && release == "":
			release = fields[1]
		case len(fields) == 3:
			replaces = append(replaces, Replaced{Version: fields[0], Name: fields[1], Digest: fields[2]})
		default:
			return "", nil, fmt.Errorf("baseline manifest line %d: want `release <name>` once, then `version name digest`", n)
		}
	}
	if err := lines.Err(); err != nil {
		return "", nil, err
	}
	if release == "" || len(replaces) == 0 {
		return "", nil, fmt.Errorf("baseline manifest: want a release line and at least one replaced version")
	}
	return release, replaces, nil
}

// CheckBaseline refuses a namespace whose baseline does not account for its
// files exactly: a file below the baseline that it does not list would never
// run on a new installation, and a listed file whose name or content moved
// would be recorded as something it is not.
func (ns Namespace) CheckBaseline() error {
	b := ns.Baseline
	if b == nil {
		return nil
	}
	if b.Migration.Unwrapped() {
		return fmt.Errorf("pgmigrate: %s baseline %s: it records the versions it replaces in its own transaction, so it cannot ask to run unwrapped", ns.Name, b.Migration.Version)
	}
	listed := make(map[string]Replaced, len(b.Replaces))
	for _, r := range b.Replaces {
		if r.Version >= b.Migration.Version {
			return fmt.Errorf("pgmigrate: %s baseline %s lists %s, which does not sort below it", ns.Name, b.Migration.Version, r.Version)
		}
		if _, dup := listed[r.Version]; dup {
			return fmt.Errorf("pgmigrate: %s baseline lists %s twice", ns.Name, r.Version)
		}
		listed[r.Version] = r
	}
	for _, m := range ns.Migrations {
		r, ok := listed[m.Version]
		switch {
		case m.Version == b.Migration.Version:
			return fmt.Errorf("pgmigrate: %s %s is both a migration and the baseline", ns.Name, m.Version)
		case !ok && m.Version < b.Migration.Version:
			return fmt.Errorf("pgmigrate: %s %s_%s sorts below the baseline but the baseline does not list it, so a new installation would never build it", ns.Name, m.Version, m.Name)
		case ok && (r.Name != m.Name || r.Digest != Digest(m)):
			return fmt.Errorf("pgmigrate: %s %s_%s: the baseline lists a different name or content for this version; regenerate the manifest (scripts/migration-baseline.sh)", ns.Name, m.Version, m.Name)
		}
	}
	return nil
}

// settleBaseline puts the baseline in the ledger. An empty ledger gets the
// baseline applied, with a row for it and for every version it replaces, in one
// transaction. A ledger holding every replaced version gets the baseline's row
// alone, and nothing runs. A ledger holding only some is refused: the baseline
// would run over a partial schema, and the replaced files are kept only until
// every supported installation is past the baseline.
//
// A recorded baseline settles nothing more. A replaced version missing beside
// it was reverted by Down, and the loop after this applies that file again.
func settleBaseline(ctx context.Context, conn *pgx.Conn, ns Namespace, table string, done map[string]LedgerRow) (installed bool, err error) {
	b := ns.Baseline
	if _, ok := done[b.Migration.Version]; ok {
		if err := assertLedgerMatches(ns.Name, done, b.Migration); err != nil {
			return false, err
		}
		return false, assertContentMatches(ns.Name, done, b.Migration)
	}

	present := 0
	for _, r := range b.Replaces {
		if _, ok := done[r.Version]; ok {
			present++
		}
	}
	install := len(done) == 0
	rows := []Replaced{{Version: b.Migration.Version, Name: b.Migration.Name, Digest: Digest(b.Migration)}}
	switch {
	case install:
		rows = append(rows, b.Replaces...)
	case present < len(b.Replaces):
		return false, fmt.Errorf(
			"pgmigrate: %s: this database applied %d of the %d migrations the baseline %s_%s replaces. "+
				"Upgrade it with release %s first, then run this release again",
			ns.Name, present, len(b.Replaces), b.Migration.Version, b.Migration.Name, b.Release)
	}

	err = inTx(ctx, conn, func(tx pgx.Tx) error {
		if install {
			if _, err := tx.Exec(ctx, b.Migration.UpSQL); err != nil {
				return err
			}
		}
		return recordLedgerRows(ctx, tx, table, rows)
	})
	if err != nil {
		return false, fmt.Errorf("pgmigrate: %s baseline %s_%s: %w", ns.Name, b.Migration.Version, b.Migration.Name, err)
	}
	for _, r := range rows {
		digest := r.Digest
		done[r.Version] = LedgerRow{Name: r.Name, Digest: &digest}
	}
	return install, nil
}

func recordLedgerRows(ctx context.Context, tx pgx.Tx, table string, rows []Replaced) error {
	versions := make([]string, len(rows))
	names := make([]string, len(rows))
	digests := make([]string, len(rows))
	for i, r := range rows {
		versions[i], names[i], digests[i] = r.Version, r.Name, r.Digest
	}
	_, err := tx.Exec(ctx, fmt.Sprintf(
		`INSERT INTO %s (version, name, content_digest) SELECT * FROM unnest($1::text[], $2::text[], $3::text[])`,
		pgx.Identifier{table}.Sanitize()), versions, names, digests)
	return err
}
