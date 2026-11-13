// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-textbounds writes the text-limit guard migrations and the
// frontend limit constants from the textbounds registry.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

func main() {
	version := flag.Int64("version", 0, "write the core migration pairs at this version and the two after it")
	customOutput := flag.String("custom-output", "", "write a new custom-track migration pair at this stem, with -version")
	tables := flag.String("tables", "", "with -version and -name, only move these comma-separated core tables' guards to columns.txt")
	name := flag.String("name", "", "the migration name for -tables")
	flag.Parse()
	if *tables != "" {
		if err := retarget(*version, *name, strings.Split(*tables, ",")); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := generate(*version, *customOutput); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(version int64, customOutput string) error {
	if version != 0 {
		catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
		if err != nil {
			return err
		}
		customTables, err := customTrackTables()
		if err != nil {
			return err
		}
		guards, err := textbounds.SQL(bytes.NewReader(catalog), customTables)
		if err != nil {
			return err
		}
		if guards.Custom.Up != "" && customOutput == "" {
			return errors.New("custom-track tables need guards: pass -custom-output")
		}
		for offset, pair := range []struct {
			name      string
			migration textbounds.Migration
		}{
			{"the_text_limit_checks_are_defined", guards.Functions},
			{"each_table_checks_its_text_limits", guards.Tables},
			{"custom_text_fields_check_their_limits", guards.CustomFields},
		} {
			if err := writePair(fmt.Sprintf("migrations/core/%d_%s", version+int64(offset), pair.name), pair.migration); err != nil {
				return err
			}
		}
		if guards.Custom.Up != "" {
			if err := writePair(customOutput, guards.Custom); err != nil {
				return err
			}
		}
	}
	return os.WriteFile("../frontend/src/lib/textlimits_gen.ts", []byte(textbounds.Frontend()), 0o644) //nolint:gosec // a committed source file every reader of the tree opens
}

// retarget writes the migration for a policy change in columns.txt, since the
// guards a shipped migration created never change in place.
func retarget(version int64, name string, tables []string) error {
	if version == 0 || name == "" {
		return errors.New("-tables needs -version and -name")
	}
	customTables, err := customTrackTables()
	if err != nil {
		return err
	}
	for _, table := range tables {
		if customTables[table] {
			return fmt.Errorf("%s is a custom-track table; its guards need a custom migration", table)
		}
	}
	catalog, err := os.Open("migrations/testdata/head_catalog.txt")
	if err != nil {
		return err
	}
	migration, err := textbounds.Retarget(catalog, tables)
	if closeErr := catalog.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return writePair(fmt.Sprintf("migrations/core/%d_%s", version, name), migration)
}

var createdTable = regexp.MustCompile(`(?i)CREATE TABLE (?:IF NOT EXISTS )?(?:public\.)?"?([a-z_][a-z0-9_]*)"?`)

// customTrackTables names the tables a custom migration creates; a core
// migration runs before them and cannot reach them.
func customTrackTables() (map[string]bool, error) {
	paths, err := filepath.Glob("migrations/custom/*.up.sql")
	if err != nil {
		return nil, err
	}
	tables := map[string]bool{}
	for _, path := range paths {
		source, err := os.ReadFile(path) //nolint:gosec // a migration file this tool globbed from its own tree
		if err != nil {
			return nil, err
		}
		for _, match := range createdTable.FindAllSubmatch(source, -1) {
			tables[string(match[1])] = true
		}
	}
	return tables, nil
}

func writePair(stem string, migration textbounds.Migration) error {
	if err := writeNew(stem+".up.sql", migration.Up); err != nil {
		return err
	}
	return writeNew(stem+".down.sql", migration.Down)
}

func writeNew(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // a new committed migration under this tool's own tree
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	return errors.Join(writeErr, file.Close())
}
