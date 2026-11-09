// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-openenums refuses a closed response enum, or an open request
// enum, in the contract. With -mark it marks every closed response enum open.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/margince/margince/backend/internal/platform/openenums"
)

func main() {
	contract := flag.String("contract", "", "the contract to check, e.g. backend/api/crm.yaml")
	mark := flag.Bool("mark", false, "mark every closed response enum open, in place")
	flag.Parse()
	if *contract == "" {
		log.Fatal("check-openenums: -contract is required")
	}
	raw, err := os.ReadFile(*contract)
	if err != nil {
		log.Fatal(err)
	}
	if *mark {
		marked, count, err := openenums.MarkOpen(raw)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(*contract, marked, 0o600); err != nil { //nolint:gosec // the contract path is a flag the developer passed
			log.Fatal(err)
		}
		log.Printf("check-openenums: marked %d response enums open", count)
		return
	}
	enums, err := openenums.Classify(raw)
	if err != nil {
		log.Fatal(err)
	}
	if violations := openenums.Violations(*contract, enums); len(violations) > 0 {
		for _, v := range violations {
			log.Print(v)
		}
		log.Fatalf("check-openenums: %d enum(s) break the contract's enum rule (see its header); "+
			"run with -mark to open the response ones", len(violations))
	}
	log.Printf("check-openenums: %d enums hold the rule", len(enums))
}
