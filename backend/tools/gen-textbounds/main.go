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

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

func main() {
	output := flag.String("output", "", "write a new core migration pair at this stem")
	customOutput := flag.String("custom-output", "", "write a new custom-track migration pair at this stem, with -output")
	flag.Parse()
	if err := generate(*output, *customOutput); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(output, customOutput string) error {
	if output != "" {
		catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
		if err != nil {
			return err
		}
		customTables, err := customTrackTables()
		if err != nil {
			return err
		}
		core, custom, err := textbounds.SQL(bytes.NewReader(catalog), customTables)
		if err != nil {
			return err
		}
		if err := writePair(output, core); err != nil {
			return err
		}
		if custom.Up != "" {
			if customOutput == "" {
				return errors.New("custom-track tables need guards: pass -custom-output")
			}
			if err := writePair(customOutput, custom); err != nil {
				return err
			}
		}
	}
	return os.WriteFile("../frontend/src/lib/textlimits_gen.ts", []byte(textbounds.Frontend()), 0o644) //nolint:gosec // a committed source file every reader of the tree opens
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
	if err := writeNew(stem+".up.sql", "SET LOCAL lock_timeout = '2s';\n"+migration.Up); err != nil {
		return err
	}
	return writeNew(stem+".down.sql", "SET LOCAL lock_timeout = '2s';\n"+migration.Down)
}

func writeNew(path, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) //nolint:gosec // a new committed migration under this tool's own tree
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	return errors.Join(writeErr, file.Close())
}
