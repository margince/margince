// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command check-apideprecation refuses API removal before its release notice expires.
package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/tools/internal/apideprecation"
)

func main() {
	base := flag.String("base", "", "previous contract")
	current := flag.String("current", "", "candidate contract")
	release := flag.String("release", "", "candidate release version; required for removals")
	at := flag.String("at", "", "RFC3339 evaluation time; defaults to now")
	ratifiedFile := flag.String("ratified-removals", "", "operations a maintainer ratified removing without a notice")
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
	var ratified apideprecation.RatifiedRemovals
	if *ratifiedFile != "" {
		file, err := os.Open(*ratifiedFile)
		if err != nil {
			log.Fatal(err)
		}
		ratified, err = apideprecation.ParseRatifiedRemovals(file)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := apideprecation.CheckChangesRatified(before, after, *release, now, ratified); err != nil {
		log.Fatal(err)
	}
	log.Print("API release notices and removal dates hold")
}
