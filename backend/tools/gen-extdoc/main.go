// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// gen-extdoc writes the tables of docs/explanation/extensions-at-a-glance.md
// from the source of backend/pkg/extension. They show what a unit can do to
// each core record, every field of Extension and the slots on a core page.
// With -check it writes nothing and exits 1 when the page differs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const pagePath = "docs/explanation/extensions-at-a-glance.md"

var errStale = errors.New("gen-extdoc: the page is stale; run `make extdoc-gen`")

func main() {
	check := flag.Bool("check", false, "write nothing; exit 1 when the page differs")
	root := flag.String("root", "..", "the repository root, relative to the working directory (backend/)")
	flag.Parse()
	if err := run(*root, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root string, check bool) error {
	src, err := parseDir(root)
	if err != nil {
		return err
	}
	d := deriveDoors(src)
	fields, err := deriveFields(src)
	if err != nil {
		return err
	}
	slots, err := deriveSlotRows(src)
	if err != nil {
		return err
	}
	if err := census(d, fields, slots); err != nil {
		return err
	}
	path := filepath.Join(root, pagePath)
	old, err := os.ReadFile(path) // #nosec G304 -- a fixed path under the repository root
	if err != nil {
		return fmt.Errorf("gen-extdoc: reading %s: %w", pagePath, err)
	}
	page, err := splice(string(old), region(d, fields, slots))
	if err != nil {
		return err
	}
	if check {
		if page != string(old) {
			return errStale
		}
		return nil
	}
	//nolint:gosec // a fixed output path under -root
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		return fmt.Errorf("gen-extdoc: writing %s: %w", pagePath, err)
	}
	return nil
}

// census fails a scan that found less than the page needs. A moved source file
// then cannot turn into an empty table that still passes.
func census(d doors, fields []extensionField, slots []slotRow) error {
	for _, kind := range knownKinds {
		if d[kind] == nil {
			return fmt.Errorf("gen-extdoc: no door found on %q; the source moved", kind)
		}
	}
	for door, label := range doorLabels {
		if !anyKind(d, door) {
			return fmt.Errorf("gen-extdoc: no record has the door %q; the source moved", label)
		}
	}
	if len(fields) < 10 || len(slots) < 5 {
		return fmt.Errorf("gen-extdoc: found %d fields and %d slots; the source moved", len(fields), len(slots))
	}
	return nil
}

func anyKind(d doors, door int) bool {
	for _, doors := range d {
		if doors[door] {
			return true
		}
	}
	return false
}
