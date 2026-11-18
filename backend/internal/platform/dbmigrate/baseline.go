// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// A baseline is one file that builds the schema a run of older versions built.
// A new installation applies it instead of replaying them. It still records
// their ledger rows, because datatask requirements and the activity repairs
// read old versions and their applied_at.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Baseline is the file that stands for Replaces, and the release a database
// part-way through them must reach first. NoEarlierRelease names none.
type Baseline struct {
	Migration Migration
	Release   string
	Replaces  []LedgerEntry
}

// NoEarlierRelease is the release line of a baseline that the first release
// ships. A database part-way through its history was built from a commit, not
// a release, so the only way forward is to build it again.
const NoEarlierRelease = "none"

// LedgerEntry is one ledger row as an install writes it. For a version a
// baseline replaces, it is the old file's own name and Digest, so the ledger
// checks hold on either route. Those are declared, not read off the file, so
// the file can go later.
type LedgerEntry struct {
	Version string
	Name    string
	Digest  string
}

// ParseBaselineManifest reads a `release <name>` line and then one
// `version name digest` line per replaced version; # starts a comment.
func ParseBaselineManifest(text string) (release string, replaces []LedgerEntry, err error) {
	lines := bufio.NewScanner(strings.NewReader(text))
	for n := 1; lines.Scan(); n++ {
		fields := strings.Fields(lines.Text())
		switch {
		case len(fields) == 0 || strings.HasPrefix(fields[0], "#"):
			continue
		case len(fields) == 2 && fields[0] == "release" && release == "":
			release = fields[1]
		case len(fields) == 3:
			replaces = append(replaces, LedgerEntry{Version: fields[0], Name: fields[1], Digest: fields[2]})
		default:
			return "", nil, fmt.Errorf("baseline manifest line %d: want `release <name>` once, then `version name digest`", n)
		}
	}
	if err := lines.Err(); err != nil {
		return "", nil, err
	}
	if release == "" || len(replaces) == 0 {
		return "", nil, errors.New("baseline manifest: want a release line and at least one replaced version")
	}
	return release, replaces, nil
}

// CheckBaseline refuses a namespace whose baseline disagrees with its files. A
// file below the baseline that it does not list would never run on a new
// installation. A listed file whose name or content moved would be recorded as
// something it is not.
func (ns Namespace) CheckBaseline() error {
	b := ns.Baseline
	if b == nil {
		return nil
	}
	if b.Migration.Unwrapped() {
		return fmt.Errorf("pgmigrate: %s baseline %s: it records the versions it replaces in its own transaction, so it cannot ask to run unwrapped", ns.Name, b.Migration.Version)
	}
	listed := make(map[string]LedgerEntry, len(b.Replaces))
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

// baselinePlan is what Up writes for a namespace's baseline. That is nothing
// once it is recorded, its own row, or every row with the baseline applied.
type baselinePlan struct {
	apply bool
	rows  []LedgerEntry
}

// planBaseline decides before Up writes anything, renumbers included, so a
// refused database keeps its ledger. A ledger holding only some replaced
// versions is refused: the baseline would run over a partial schema. Beside a
// recorded baseline, a missing replaced version was reverted by Down, and Up
// applies that file again.
func planBaseline(ns Namespace, done map[string]LedgerRow) (baselinePlan, error) {
	b := ns.Baseline
	if b == nil {
		return baselinePlan{}, nil
	}
	if err := assertReplacedRowsMatch(ns, done); err != nil {
		return baselinePlan{}, err
	}
	if _, ok := done[b.Migration.Version]; ok {
		if err := assertLedgerMatches(ns.Name, done, b.Migration); err != nil {
			return baselinePlan{}, err
		}
		return baselinePlan{}, assertContentMatches(ns.Name, done, b.Migration)
	}

	present := 0
	for _, r := range b.Replaces {
		if _, ok := done[r.Version]; ok {
			present++
		}
	}
	own := LedgerEntry{Version: b.Migration.Version, Name: b.Migration.Name, Digest: Digest(b.Migration)}
	switch {
	case len(done) == 0:
		return baselinePlan{apply: true, rows: append([]LedgerEntry{own}, b.Replaces...)}, nil
	case present < len(b.Replaces):
		return baselinePlan{}, partialHistoryRefusal(ns.Name, b, present)
	}
	return baselinePlan{rows: []LedgerEntry{own}}, nil
}

// assertReplacedRowsMatch compares each recorded replaced version whose file
// is gone with its manifest entry. A shipped file's row is checked like any
// other migration's, against the file.
func assertReplacedRowsMatch(ns Namespace, done map[string]LedgerRow) error {
	shipped := make(map[string]bool, len(ns.Migrations))
	for _, m := range ns.Migrations {
		shipped[m.Version] = true
	}
	for _, r := range ns.Baseline.Replaces {
		row, ok := done[r.Version]
		if !ok || shipped[r.Version] || (row.Name == r.Name && row.Digest != nil && *row.Digest == r.Digest) {
			continue
		}
		return fmt.Errorf(
			"pgmigrate: %s %s: the ledger records %q with other content than the baseline %s lists as %q. "+
				"This database did not build the schema the baseline stands for; restore it from a backup or rebuild it (make dev-fresh)",
			ns.Name, r.Version, row.Name, ns.Baseline.Migration.Version, r.Name)
	}
	return nil
}

// partialHistoryRefusal says what to do with a database part-way through the
// versions b replaces.
func partialHistoryRefusal(namespace string, b *Baseline, present int) error {
	next := fmt.Sprintf("Upgrade it with release %s first, then run this release again", b.Release)
	if b.Release == NoEarlierRelease {
		next = "No release ships them without the baseline, so build this database again (make dev-fresh)"
	}
	return fmt.Errorf("pgmigrate: %s: this database applied %d of the %d migrations the baseline %s_%s replaces. %s",
		namespace, present, len(b.Replaces), b.Migration.Version, b.Migration.Name, next)
}

// settleBaseline writes what planBaseline decided, in one transaction, and
// adds the rows to done.
func settleBaseline(ctx context.Context, conn *pgx.Conn, ns Namespace, table string, plan baselinePlan, done map[string]LedgerRow) error {
	if len(plan.rows) == 0 {
		return nil
	}
	b := ns.Baseline
	record, args := recordLedgerRows(table, plan.rows)
	var err error
	if plan.apply {
		err = apply(ctx, conn, b.Migration, b.Migration.UpSQL, record, args)
	} else {
		_, err = conn.Exec(ctx, record, args...)
	}
	if err != nil {
		return fmt.Errorf("pgmigrate: %s baseline %s_%s: %w", ns.Name, b.Migration.Version, b.Migration.Name, err)
	}
	for _, r := range plan.rows {
		digest := r.Digest
		done[r.Version] = LedgerRow{Name: r.Name, Digest: &digest}
	}
	return nil
}

// recordLedgerRows is one INSERT for every row, with its arguments.
func recordLedgerRows(table string, rows []LedgerEntry) (string, []any) {
	versions := make([]string, len(rows))
	names := make([]string, len(rows))
	digests := make([]string, len(rows))
	for i, r := range rows {
		versions[i], names[i], digests[i] = r.Version, r.Name, r.Digest
	}
	return fmt.Sprintf(
		`INSERT INTO %s (version, name, content_digest) SELECT * FROM unnest($1::text[], $2::text[], $3::text[])`,
		pgx.Identifier{table}.Sanitize()), []any{versions, names, digests}
}

// LedgerAtHead lists the rows the ledger holds once Up has run: one per
// migration, the baseline's, and one per replaced version whose file is gone.
// The test-database probe reads it, so it expects what settleBaseline writes.
func (ns Namespace) LedgerAtHead() []LedgerEntry {
	entries := make([]LedgerEntry, 0, len(ns.Migrations)+1)
	shipped := make(map[string]bool, len(ns.Migrations))
	for _, m := range ns.Migrations {
		shipped[m.Version] = true
		entries = append(entries, LedgerEntry{Version: m.Version, Name: m.Name, Digest: Digest(m)})
	}
	if b := ns.Baseline; b != nil {
		entries = append(entries, LedgerEntry{Version: b.Migration.Version, Name: b.Migration.Name, Digest: Digest(b.Migration)})
		for _, r := range b.Replaces {
			if !shipped[r.Version] {
				entries = append(entries, r)
			}
		}
	}
	return entries
}
