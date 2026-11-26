// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-worklist compiles backend/api/worklist-kinds.yaml into two
// tables. The engine's, compose/attention/kinds_gen.go, decides each kind's
// category, screen, owner answers, scope and Done. The frontend's,
// frontend/src/lib/worklistkinds.gen.ts, names each kind.
//
// A catalog that cannot be true fails generation. That is a missing or unknown
// field, a value outside its set, or a kind with no title in some locale.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	contractPath = flag.String("contract", "api/worklist-kinds.yaml", "the kind catalog to compile")
	localesDir   = flag.String("locales", "../frontend/src/i18n", "the directory holding the frontend locale tables")
	outGoPath    = flag.String("out-go", "internal/compose/attention/kinds_gen.go", "generated engine table destination")
	outTSPath    = flag.String("out-frontend", "../frontend/src/lib/worklistkinds.gen.ts", "generated frontend table destination")
)

func main() {
	flag.Parse()
	if err := generate(); err != nil {
		log.Fatal(err)
	}
}

func generate() error {
	c, err := loadCatalog(*contractPath)
	if err != nil {
		return err
	}
	if err := checkTitles(c, *localesDir); err != nil {
		return err
	}
	goTable, err := emitGo(c)
	if err != nil {
		return err
	}
	//nolint:gosec // committed source files every reader of the tree opens
	if err := os.WriteFile(*outGoPath, goTable, 0o644); err != nil {
		return fmt.Errorf("gen-worklist: writing %s: %w", *outGoPath, err)
	}
	//nolint:gosec // committed source files every reader of the tree opens
	if err := os.WriteFile(*outTSPath, emitFrontend(c), 0o644); err != nil {
		return fmt.Errorf("gen-worklist: writing %s: %w", *outTSPath, err)
	}
	fmt.Printf("%d worklist kinds generated\n", len(c))
	return nil
}

func loadCatalog(path string) (catalog, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // build-time tool, operator-chosen catalog path
	if err != nil {
		return nil, fmt.Errorf("gen-worklist: reading %s: %w", path, err)
	}
	return parseCatalog(raw)
}
