// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-apideprecation refuses API removal before its release notice expires.
package main

import (
	"flag"
	"log"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/tools/internal/apideprecation"
)

func main() {
	base := flag.String("base", "", "previous contract")
	current := flag.String("current", "", "candidate contract")
	release := flag.String("release", "", "candidate release version; required for removals")
	at := flag.String("at", "", "RFC3339 evaluation time; defaults to now")
	flag.Parse()
	if *base == "" || *current == "" {
		log.Fatal("check-apideprecation: -base and -current are required")
	}
	now := time.Now().UTC()
	if *at != "" {
		parsed, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			log.Fatal(err)
		}
		now = parsed
	}
	loader := openapi3.NewLoader()
	before, err := loader.LoadFromFile(*base)
	if err != nil {
		log.Fatal(err)
	}
	after, err := loader.LoadFromFile(*current)
	if err != nil {
		log.Fatal(err)
	}
	if err := apideprecation.CheckChanges(before, after, *release, now); err != nil {
		log.Fatal(err)
	}
	log.Print("API release notices and removal dates hold")
}
