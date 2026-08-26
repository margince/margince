// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// gen-configschema emits the JSON Schema for margince.yaml.
//
// WHY IT IS GENERATED. margince.yaml is sixteen sections deep and grows a field
// whenever a capability gains a switch. A hand-written schema is a second copy
// of deployconfig.Config, and the copy goes stale silently: an editor keeps
// validating against last quarter's shape and reports a new field as an error,
// which is worse than no schema at all. So the schema is derived from the
// struct the LOADER decodes into — the same reason tools/gen-composition reads
// the published extension.Name rule rather than restating it.
//
// The routing subtree rides along under $defs. It used to be its own file,
// which made sense while a routing FILE existed; with the binding living in
// margince.yaml under `seeds.ai_routing` and in the database, its shape belongs
// where the thing it describes is written.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/gradionhq/margince/backend/internal/platform/deployconfig"
)

func main() {
	pkgDir := flag.String("pkg", "", "the deployconfig package directory, read for field documentation")
	out := flag.String("out", "", "the schema file to write")
	flag.Parse()
	if *pkgDir == "" || *out == "" {
		fail(fmt.Errorf("gen-configschema needs -pkg and -out"))
	}
	docs, err := fieldDocs(*pkgDir)
	if err != nil {
		fail(err)
	}
	schema, err := buildSchema(reflect.TypeOf(deployconfig.Config{}), docs)
	if err != nil {
		fail(err)
	}
	encoded, err := encode(schema)
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		fail(err)
	}
	if err := os.WriteFile(*out, encoded, 0o600); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
