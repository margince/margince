// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// policy-refresh compares each messaging policy pack with its official
// sources. It fetches every instrument's source, digests the text, and says
// which changed and which it could not read. Then it stamps the digests and
// source-checked into the layers. With -report it writes nothing.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/margince/margince/backend/internal/platform/messagingpolicy"
	"github.com/margince/margince/backend/internal/platform/outbound"
)

const (
	// fetchTimeout bounds one source, so a host that never answers is
	// reported and the run goes on.
	fetchTimeout = 45 * time.Second
	// maxSourceBytes is more than a consolidated act in any form takes.
	maxSourceBytes = 32 << 20
)

func main() {
	root := flag.String("root", "", "the repository root: refresh every extensions/*/policy")
	report := flag.Bool("report", false, "write nothing; print what a refresh finds")
	flag.Parse()
	fetch := httpSource(&http.Client{Timeout: fetchTimeout})
	if err := run(context.Background(), os.Stdout, *root, *report, fetch, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run refreshes each unit's policy folder under root and prints the report.
// A source it cannot read is a line of the report and no failure. A publisher
// that refuses a client says nothing about the pack.
func run(ctx context.Context, out io.Writer, root string, reportOnly bool, fetch messagingpolicy.FetchSource, today time.Time) error {
	folders, err := filepath.Glob(filepath.Join(root, "extensions", "*", "policy"))
	if err != nil {
		return err
	}
	if len(folders) == 0 {
		return fmt.Errorf("policy-refresh: no extensions/*/policy under %q; check -root", root)
	}
	counts := map[messagingpolicy.SourceOutcome]int{}
	for _, folder := range folders {
		report, err := messagingpolicy.RefreshSources(ctx, os.DirFS(folder), fetch, today)
		if err != nil {
			return fmt.Errorf("%s: %w", folder, err)
		}
		if err := printFindings(out, folder, report.Findings, counts); err != nil {
			return err
		}
		if reportOnly {
			continue
		}
		for name, layer := range report.Stamped {
			//nolint:gosec // a layer the compiler just read from this folder
			if err := os.WriteFile(filepath.Join(folder, filepath.FromSlash(name)), layer, 0o644); err != nil {
				return err
			}
		}
	}
	return printSummary(out, counts, reportOnly)
}

func printFindings(out io.Writer, folder string, findings []messagingpolicy.SourceFinding, counts map[messagingpolicy.SourceOutcome]int) error {
	layer := ""
	for _, finding := range findings {
		counts[finding.Outcome]++
		if finding.Layer != layer {
			layer = finding.Layer
			if _, err := fmt.Fprintln(out, filepath.Join(folder, filepath.FromSlash(layer))); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "  %-12s  %s\n                %s  (%s)\n",
			finding.Outcome, finding.Instrument, finding.Address, finding.Detail); err != nil {
			return err
		}
	}
	return nil
}

// printSummary counts each outcome in a fixed order and says what is owed.
func printSummary(out io.Writer, counts map[messagingpolicy.SourceOutcome]int, reportOnly bool) error {
	order := []messagingpolicy.SourceOutcome{
		messagingpolicy.SourceUnchanged, messagingpolicy.SourceChanged, messagingpolicy.SourceFirstRead,
		messagingpolicy.SourceUnread, messagingpolicy.SourceByHand, messagingpolicy.SourceNone,
	}
	lines := []string{""}
	for _, outcome := range order {
		lines = append(lines, fmt.Sprintf("%4d  %s", counts[outcome], outcome))
	}
	if counts[messagingpolicy.SourceChanged] > 0 {
		lines = append(lines, "Read each changed source and update its layer where the law changed.")
	}
	if counts[messagingpolicy.SourceUnread] > 0 {
		lines = append(lines, "Give each source that was not read a text-url, or read it and set checked-by-hand.")
	}
	if !reportOnly {
		lines = append(lines, "The layers are stamped. Their generated rules are stale until gen-messaging-rules runs.")
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// httpSource reads an address with one plain GET, as the report promises. It
// runs no script, so a page that loads its text by script comes back empty.
func httpSource(client *http.Client) messagingpolicy.FetchSource {
	return func(ctx context.Context, address string) (page messagingpolicy.SourcePage, err error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return messagingpolicy.SourcePage{}, err
		}
		request.Header.Set("User-Agent", outbound.PolicySourceHeader)
		response, err := client.Do(request)
		if err != nil {
			return messagingpolicy.SourcePage{}, err
		}
		defer func() { err = errors.Join(err, response.Body.Close()) }()
		if response.StatusCode != http.StatusOK {
			return messagingpolicy.SourcePage{}, fmt.Errorf("the host answered %d", response.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maxSourceBytes+1))
		if err != nil {
			return messagingpolicy.SourcePage{}, err
		}
		if len(body) > maxSourceBytes {
			return messagingpolicy.SourcePage{}, fmt.Errorf("the answer is larger than %d bytes", maxSourceBytes)
		}
		return messagingpolicy.SourcePage{ContentType: response.Header.Get("Content-Type"), Body: body}, nil
	}
}
