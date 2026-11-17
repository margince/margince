// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-baseline writes core's baseline from three pg_dump sections of a
// database the history built: the up file, and replaces.txt naming the
// versions it stands for. scripts/migration-baseline.sh generate runs it, and
// verify proves the result builds what the history builds.
//
// A dump is not yet a migration. It names the owner role, it grants to roles a
// throwaway database lacks, it cannot create a role at all, and its reference
// rows carry the uuids and clocks of the run that built it. Each rewrite below
// answers one of those.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

type options struct {
	pre, data, post, roles string
	since                  time.Time
	version, name, through string
	release, outDir        string
}

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err == nil {
		err = run(opts)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-baseline:", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (options, error) {
	var o options
	var since string
	flags := flag.NewFlagSet("gen-baseline", flag.ContinueOnError)
	flags.StringVar(&o.pre, "pre", "", "pg_dump --schema-only --section=pre-data")
	flags.StringVar(&o.data, "data", "", "pg_dump --data-only --column-inserts --rows-per-insert=1")
	flags.StringVar(&o.post, "post", "", "pg_dump --schema-only --section=post-data")
	flags.StringVar(&o.roles, "roles", "", "the migration that makes the cluster-level roles, which no dump carries")
	flags.StringVar(&since, "since", "", "the first applied_at of the build; a later timestamp in a reference row becomes now()")
	flags.StringVar(&o.through, "through", "", "the last core version the baseline replaces")
	flags.StringVar(&o.version, "version", "", "the baseline's own version")
	flags.StringVar(&o.name, "name", "", "the baseline's name")
	flags.StringVar(&o.release, "release", "", "the release a database part-way through the history must reach first")
	flags.StringVar(&o.outDir, "out", "", "the directory to write the baseline into")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	for _, required := range []string{o.pre, o.data, o.post, o.roles, since, o.through, o.version, o.name, o.release, o.outDir} {
		if required == "" {
			return o, errors.New("every flag is required")
		}
	}
	var err error
	o.since, err = time.Parse("2006-01-02 15:04:05.999999-07", since)
	return o, err
}

func run(o options) error {
	replaced, err := replacedThrough(o.through)
	if err != nil {
		return err
	}
	if o.version <= o.through {
		return fmt.Errorf("version %s must sort above %s, the last version it replaces", o.version, o.through)
	}
	sections := make(map[string]string, 4)
	for key, path := range map[string]string{"pre": o.pre, "data": o.data, "post": o.post, "roles": o.roles} {
		raw, err := os.ReadFile(path) //nolint:gosec // the path is a flag the script passed
		if err != nil {
			return err
		}
		sections[key] = string(raw)
	}

	pre, preGrants := schemaSection(sections["pre"])
	post, postGrants := schemaSection(sections["post"])
	// One restatement may sit in either section, and each must match once
	// across both, so they are restated together and split again.
	const sectionBreak = "\n-- gen-baseline: section break\n"
	restated, err := restate(pre + sectionBreak + post)
	if err != nil {
		return err
	}
	pre, post, _ = strings.Cut(restated, sectionBreak)
	rows, err := referenceRows(sections["data"], o.since)
	if err != nil {
		return err
	}
	appGrants, err := grantsToApp(append(preGrants, postGrants...))
	if err != nil {
		return err
	}

	var up strings.Builder
	up.WriteString(header(o, len(replaced)))
	// The app's grants go before the roles migration's, as in the history: an
	// object's privileges are a list, and pg_dump prints it in grant order.
	for _, part := range []string{pre, rows, post, appGrants, statementsOf(sections["roles"])} {
		up.WriteString("\n")
		up.WriteString(strings.TrimSpace(part))
		up.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(o.outDir, o.version+"_"+o.name+".up.sql"), []byte(up.String()), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(o.outDir, "replaces.txt"), []byte(manifest(o.release, replaced)), 0o600)
}

// replacedThrough is every core version up to and including through, read
// from the history route so each carries the Digest its file has today.
func replacedThrough(through string) ([]dbmigrate.Migration, error) {
	core, err := migrations.CoreHistory()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(core.Migrations, func(m dbmigrate.Migration) bool { return m.Version == through })
	if i < 0 {
		return nil, fmt.Errorf("no core migration has version %s", through)
	}
	return core.Migrations[:i+1], nil
}

func header(o options, replaced int) string {
	return fmt.Sprintf(`-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The schema core's %d versions through %s build, for a new installation.
-- dbmigrate applies it only to an empty ledger and records each of those
-- versions beside it (replaces.txt). Written by scripts/migration-baseline.sh
-- generate; scripts/migration-baseline.sh verify proves it builds what the
-- history builds. Regenerate it; never edit it.
--
-- The order is pg_dump's restore order: tables and functions, then the
-- reference rows, then constraints, indexes and triggers, so no row passes
-- through a trigger the history did not run it through.

-- A LANGUAGE sql body may name a table that a later statement creates.
SET LOCAL check_function_bodies = false;
`, replaced, o.through)
}

func manifest(release string, replaced []dbmigrate.Migration) string {
	var b strings.Builder
	b.WriteString("# The versions the baseline beside this file stands for, as\n")
	b.WriteString("# `version name digest`, and the release that ships the last of them.\n")
	b.WriteString("# Written by scripts/migration-baseline.sh generate.\n")
	fmt.Fprintf(&b, "release %s\n", release)
	for _, m := range replaced {
		fmt.Fprintf(&b, "%s %s %s\n", m.Version, m.Name, dbmigrate.Digest(m))
	}
	return b.String()
}

var (
	dumpComment  = regexp.MustCompile(`^-- (Name|Data for Name): .*; Type: |^-- (PostgreSQL database dump|Dumped (from|by) )`)
	sessionState = regexp.MustCompile(`^(SET |SELECT pg_catalog\.set_config\(|\\(un)?restrict )`)
	privilege    = regexp.MustCompile(`^(GRANT|REVOKE|ALTER DEFAULT PRIVILEGES) `)
)

// schemaSection drops what pg_dump writes about the dump rather than the
// schema, and returns the privilege statements apart, because they name roles
// that need not exist. A `--` line is dropped only as part of pg_dump's own
// three-line object header, so a comment inside a function body survives.
func schemaSection(dump string) (string, []string) {
	lines := strings.Split(dump, "\n")
	drop := make([]bool, len(lines))
	for i, line := range lines {
		if !dumpComment.MatchString(line) {
			continue
		}
		drop[i] = true
		if i > 0 && lines[i-1] == "--" {
			drop[i-1] = true
		}
		if i+1 < len(lines) && lines[i+1] == "--" {
			drop[i+1] = true
		}
	}
	var kept, grants []string
	for i, line := range lines {
		switch {
		case drop[i] || sessionState.MatchString(line) || strings.HasPrefix(line, "COMMENT ON EXTENSION "):
			// An extension a superuser created at bootstrap is not the owner's
			// to comment on, and it carries its own comment anyway.
		case privilege.MatchString(line):
			grants = append(grants, line)
		case line == "CREATE SCHEMA ext;":
			kept = append(kept, "CREATE SCHEMA IF NOT EXISTS ext;")
		default:
			kept = append(kept, line)
		}
	}
	return squeezeBlankLines(kept), grants
}

// restatements are expressions whose stored tree pg_dump cannot print back.
// Postgres keeps a BETWEEN, or a row compared with IS DISTINCT FROM, as an AND
// or OR nested inside another, while the text pg_dump prints is parsed into a
// flat one, so the dump would build a different, if equivalent, expression.
// Each is put back in the words of the migration that wrote it. A restatement
// whose dump text is not found stops the generator, so none goes stale.
var restatements = []struct{ dumped, source string }{
	{ // 1790648439_a_working_week_has_a_day_in_it
		dumped: `CHECK (((work_days IS NULL) OR (((cardinality(work_days) >= 1) AND (cardinality(work_days) <= 7)) AND (work_days <@ ARRAY[(1)::smallint, (2)::smallint, (3)::smallint, (4)::smallint, (5)::smallint, (6)::smallint, (7)::smallint]))))`,
		source: `CHECK (work_days IS NULL OR (cardinality(work_days) BETWEEN 1 AND 7 AND work_days <@ ARRAY[1,2,3,4,5,6,7]::smallint[]))`,
	},
	{ // 1792911117_keying_a_signal_leaves_its_version
		dumped: `WHEN (((NOT (((((old.entity_unkeyed IS DISTINCT FROM new.entity_unkeyed) OR (old.entity_deal_id IS DISTINCT FROM new.entity_deal_id)) OR (old.entity_company_id IS DISTINCT FROM new.entity_company_id)) OR (old.entity_contact_id IS DISTINCT FROM new.entity_contact_id)) OR (old.entity_project_id IS DISTINCT FROM new.entity_project_id))) OR ((to_jsonb(old.*) - ARRAY['entity_unkeyed'::text, 'entity_deal_id'::text, 'entity_company_id'::text, 'entity_contact_id'::text, 'entity_project_id'::text]) IS DISTINCT FROM (to_jsonb(new.*) - ARRAY['entity_unkeyed'::text, 'entity_deal_id'::text, 'entity_company_id'::text, 'entity_contact_id'::text, 'entity_project_id'::text]))))`,
		source: `WHEN ((OLD.entity_unkeyed, OLD.entity_deal_id, OLD.entity_company_id, OLD.entity_contact_id, OLD.entity_project_id) IS NOT DISTINCT FROM (NEW.entity_unkeyed, NEW.entity_deal_id, NEW.entity_company_id, NEW.entity_contact_id, NEW.entity_project_id) OR (to_jsonb(OLD) - ARRAY['entity_unkeyed', 'entity_deal_id', 'entity_company_id', 'entity_contact_id', 'entity_project_id']) IS DISTINCT FROM (to_jsonb(NEW) - ARRAY['entity_unkeyed', 'entity_deal_id', 'entity_company_id', 'entity_contact_id', 'entity_project_id']))`,
	},
}

func restate(schema string) (string, error) {
	for _, r := range restatements {
		if strings.Count(schema, r.dumped) != 1 {
			return "", fmt.Errorf("a restatement's dump text occurs %d times, not once: %.80s", strings.Count(schema, r.dumped), r.dumped)
		}
		schema = strings.Replace(schema, r.dumped, r.source, 1)
	}
	return schema, nil
}

// grantsToApp keeps the margince_app privileges, inside a block that skips
// them where the role does not exist, as every core grant migration does. The
// ext_runtime ones are what the roles migration grants itself.
func grantsToApp(grants []string) (string, error) {
	var app []string
	for _, g := range grants {
		switch {
		case strings.HasSuffix(g, " TO ext_runtime;"):
		case strings.HasSuffix(g, " TO margince_app;"):
			app = append(app, "    "+forRole.ReplaceAllString(g, "ALTER DEFAULT PRIVILEGES "))
		default:
			return "", fmt.Errorf("a privilege names a role the baseline does not know how to grant to: %s", g)
		}
	}
	return "DO $$\nBEGIN\n  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN\n" +
		strings.Join(app, "\n") + "\n  END IF;\nEND $$;", nil
}

// forRole names the role that ran the dump. Without it the default privileges
// belong to whichever role applies the baseline, which is what the history did.
var forRole = regexp.MustCompile(`^ALTER DEFAULT PRIVILEGES FOR ROLE \S+ `)

var (
	uuidLiteral      = regexp.MustCompile(`'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'`)
	timestampLiteral = regexp.MustCompile(`'(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?[+-]\d{2})'`)
	unusedSequence   = regexp.MustCompile(`^SELECT pg_catalog\.setval\('[^']+', 1, false\);$`)
)

// referenceRows keeps the INSERTs and turns what the build's run generated
// back into what generates it: a uuid into uuidv7(), and a timestamp the build
// wrote into now(). A uuid that occurs twice links two rows, and regenerating
// it would break the link, so it stops the generator instead.
func referenceRows(dump string, since time.Time) (string, error) {
	var rows []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(dump, "\n") {
		if unusedSequence.MatchString(line) {
			continue
		}
		if !strings.HasPrefix(line, "INSERT INTO ") && !strings.HasPrefix(line, "SELECT pg_catalog.setval(") {
			continue
		}
		for _, id := range uuidLiteral.FindAllString(line, -1) {
			if seen[id] {
				return "", fmt.Errorf("uuid %s occurs twice in the reference rows; a row links to another by uuid", id)
			}
			seen[id] = true
		}
		line = uuidLiteral.ReplaceAllString(line, "public.uuidv7()")
		line = timestampLiteral.ReplaceAllStringFunc(line, func(literal string) string {
			at, err := time.Parse("2006-01-02 15:04:05.999999-07", strings.Trim(literal, "'"))
			if err != nil || at.Before(since) {
				return literal
			}
			return "now()"
		})
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n"), nil
}

// statementsOf drops a migration's licence header and opening comment, which
// describe that file and not the baseline it is copied into.
func statementsOf(sql string) string {
	lines := strings.Split(sql, "\n")
	i := 0
	for i < len(lines) && (strings.HasPrefix(lines[i], "--") || strings.TrimSpace(lines[i]) == "") {
		i++
	}
	return strings.Join(lines[i:], "\n")
}

func squeezeBlankLines(lines []string) string {
	var out []string
	for i, line := range lines {
		if line == "" && i > 0 && lines[i-1] == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
