// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-agentcontract cuts the agent-facing contract out of api/crm.yaml.
// It keeps every operation a passport may call and the components those
// operations reach, and writes an index of those operations. Both are files of the
// downloadable agent skill, embedded by the package that serves it.
//
// An operation is kept when the agent gate does not refuse its class
// (`x-agent-access: human-only` or `auth-bootstrap`) and its security accepts
// a passport. Descriptions are cut to their first paragraph, and one that
// reads as a note to the contract's developers is dropped. What the x-* keys
// say about the permission and the approval a call needs is written into its
// description before the keys are stripped. `servers` is left out because
// only the running install knows its own address.
//
// Usage:
//
//	gen-agentcontract -in api/crm.yaml -catalog migrations/testdata/head_catalog.txt \
//	  -out internal/compose/agentbundle/agentcontract_gen.yaml -index internal/compose/agentbundle/agentindex_gen.txt
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

func main() {
	in := flag.String("in", "", "authoritative crm.yaml")
	catalog := flag.String("catalog", "", "the schema catalog the storage table names are read from")
	out := flag.String("out", "", "generated agent contract")
	index := flag.String("index", "", "generated operation index")
	flag.Parse()
	if *in == "" || *catalog == "" || *out == "" || *index == "" {
		log.Fatal("gen-agentcontract: -in, -catalog, -out and -index are required")
	}

	src, err := os.ReadFile(*in) // #nosec G304 -- the contract path is a build argument, not user input
	if err != nil {
		log.Fatalf("gen-agentcontract: %v", err)
	}
	catalogText, err := os.ReadFile(*catalog) // #nosec G304 -- a build argument, not user input
	if err != nil {
		log.Fatalf("gen-agentcontract: %v", err)
	}
	contract, operationIndex, operations, err := agentContract(src, storageTables(string(catalogText)))
	if err != nil {
		log.Fatalf("gen-agentcontract: %v", err)
	}
	for path, body := range map[string][]byte{*out: contract, *index: operationIndex} {
		if err := os.WriteFile(path, body, 0o600); err != nil { // #nosec G703 -- the output paths ARE this tool's flags
			log.Fatalf("gen-agentcontract: %v", err)
		}
	}
	fmt.Printf("%d agent operations generated\n", operations)
}

// catalogTable is a table line of the schema catalog.
var catalogTable = regexp.MustCompile(`(?m)^public\.([a-z0-9_]+) acl=`)

// storageTables returns the catalog's table names that cannot be an ordinary
// English word: those with an underscore. "contact" and "deal" are both.
func storageTables(catalog string) []string {
	var tables []string
	for _, match := range catalogTable.FindAllStringSubmatch(catalog, -1) {
		if strings.Contains(match[1], "_") {
			tables = append(tables, match[1])
		}
	}
	return tables
}
