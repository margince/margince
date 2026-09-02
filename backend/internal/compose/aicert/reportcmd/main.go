// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command reportcmd prints the AI certification readiness report — one row per
// shipped invocation site, saying whether a record covers it, whether that
// record still describes what this build sends, and on which
// (provider, model, env) it was measured. It reads three trees: the census of
// sites this build ships, the corpus those sites are scored against, and the
// committed records aicert.LoadRecords reads back
// (backend/internal/compose/aicert/records/ by default).
//
// It is a go-run-only developer tool, not a shipped process role:
// `make e2e-ai-report` invokes it directly with `go run`, so it never gets a
// cmd/<role> entry of its own.
//
// The TEXT report always exits 0 — the certification lane is paid and manual,
// and a report that failed a build would make every prompt edit wait on a paid
// run. `-format=json` is the opposite: it feeds `make gen`, so a snapshot it
// could not build exits non-zero rather than leaving the previous file in place
// looking freshly generated.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
)

func main() {
	dir := flag.String("dir", "internal/compose/aicert/records",
		"directory of certification records (relative to backend/, matching `make e2e-ai-report`'s cwd)")
	corpusDir := flag.String("corpus", "internal/compose/aicert/corpus",
		"directory of certification scenarios, read to tell a current record from a stale one")
	format := flag.String("format", formatText,
		"`text` for the readable table, or `json` to write the snapshot the product embeds")
	out := flag.String("out", "internal/compose/aicert/snapshot/certification.json",
		"where -format=json writes; this path is committed and drift-gated")
	flag.Parse()

	// Refused rather than defaulted: a typo silently producing the text report
	// where a build expected the snapshot would leave a stale committed file
	// looking freshly generated.
	if *format != formatText && *format != formatJSON {
		fmt.Fprintf(os.Stderr, "reportcmd: -format=%s is neither %s nor %s\n", *format, formatText, formatJSON)
		os.Exit(1)
	}

	// The census is what says which sites SHOULD have a record: an absent
	// record cannot name itself, and a report that enumerated only the records
	// it found would read as full coverage of whatever happened to exist.
	census, err := compose.NewTaskCensus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: building the invocation-site census: %v\n", err)
		os.Exit(1)
	}
	corpus, err := aicert.LoadCorpus(*corpusDir, census)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	records, err := aicert.LoadRecords(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	// The stamps are computed here, beside the trees they are computed from: a
	// stamp drives each site's own request builder, which can fail exactly like
	// reading a malformed corpus does — and a report that could not tell a
	// current record from a stale one is not a report worth printing.
	stamps, perScenario, err := currentStamps(context.Background(), corpus, census)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}

	shipped := shippedCensus{sites: census.All(), scopes: census.Scopes()}
	if *format == formatJSON {
		writeSnapshot(shipped, stamps, perScenario, records, *dir, *out)
		return
	}

	fmt.Print(renderReadiness(shipped, stamps, perScenario, records)) //nolint:forbidigo // this IS the report — reportcmd's whole job is printing it to stdout, not application logging
}

// The two things -format can be. Named rather than spelled three times, so the
// flag's help, its validation and its branch all read one word.
const (
	formatText = "text"
	formatJSON = "json"
)

// writeSnapshot generates the committed table the product embeds.
//
// It exits non-zero on failure, unlike the text report, which always exits 0
// because it is a view a human reads. This one feeds `make gen`: a snapshot that
// could not be built must stop the generation rather than leave the previous
// file in place looking current.
func writeSnapshot(census shippedCensus, stamps map[string]string, perScenario map[string]map[string]string,
	records []aicert.Record, recordsDir, out string,
) {
	rows, unclaimed := readinessRows(census, stamps, perScenario, records)
	// Said out loud rather than dropped. The text report lists these; a
	// generator that swallowed them would let a record for a retired site
	// disappear from the snapshot with nothing in the `make gen` output to
	// notice, which is how a census stops covering what it names.
	for _, rec := range unclaimed {
		fmt.Fprintf(os.Stderr, "reportcmd: record %s on %s:%s (%s) covers no shipped site and is not in the snapshot\n",
			rec.Task, rec.Provider, rec.ServedModel, rec.EnvClass)
	}
	encoded, err := renderJSON(recordsDir, rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: %v\n", err)
		os.Exit(1)
	}
	// Written beside the target and renamed, because os.WriteFile truncates
	// first: a failure part-way leaves a half-written file that the next build
	// cannot embed, and `make gen` would have to be diagnosed from a compile
	// error in a generated artifact. The rename is atomic within the directory.
	tmp := out + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: writing %s: %v\n", tmp, err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, out); err != nil {
		fmt.Fprintf(os.Stderr, "reportcmd: renaming %s to %s: %v\n", tmp, out, err)
		os.Exit(1)
	}
}
