// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// `worker deal-key-names` — renames imported deals still named by their source
// key, from that system's export.
//
// A subcommand rather than an endpoint because it runs once per export, by an
// operator holding the file, and leaves nothing behind in the product.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var keyNameExportHeader = []string{"source_system", "source_key", "source_title"}

var utf8ByteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// keyNameOutcomeOrder fixes the summary line's order, so two runs over one
// export print the same text.
var keyNameOutcomeOrder = []deals.KeyNameOutcome{
	deals.KeyNameRenamed, deals.KeyNameWouldRename, deals.KeyNameNoMatch,
	deals.KeyNameAmbiguous, deals.KeyNameNoCompany,
}

func runDealKeyNames(ctx context.Context, pool *pgxpool.Pool, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("deal-key-names", flag.ContinueOnError)
	fs.SetOutput(stdout)
	workspace := fs.String("workspace", "", "workspace id whose deals to rename (required)")
	export := fs.String("export", "", "CSV export: source_system,source_key,source_title (required)")
	apply := fs.Bool("apply", false, "write the renames; without it the run only reports")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *workspace == "" || *export == "" {
		return errors.New("deal-key-names: --workspace and --export are both required")
	}
	wsID, err := ids.Parse(*workspace)
	if err != nil {
		return fmt.Errorf("deal-key-names: --workspace is not an id: %w", err)
	}
	file, err := os.Open(*export)
	if err != nil {
		return fmt.Errorf("deal-key-names: opening the export: %w", err)
	}
	entries, readErr := readKeyNamedDeals(file)
	if err := file.Close(); err != nil && readErr == nil {
		readErr = fmt.Errorf("deal-key-names: closing the export: %w", err)
	}
	if readErr != nil {
		return readErr
	}

	ctx = principal.WithWorkspaceID(ctx, wsID)
	ctx = principal.SystemActing(ctx, "system:deal_key_names")
	store := deals.NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](wsID)), compose.DealsInstallation())
	results, err := store.RenameKeyNamedDeals(ctx, entries, *apply)
	// The report prints even when a row failed: the renames before it are
	// committed, and the operator needs to see which ones.
	if reportErr := writeKeyNameReport(stdout, results, *apply); reportErr != nil && err == nil {
		err = reportErr
	}
	return err
}

// readKeyNamedDeals parses the export and refuses it whole on the first row it
// cannot trust, naming the line so the operator can fix the file.
func readKeyNamedDeals(r io.Reader) ([]deals.KeyNamedDeal, error) {
	// A spreadsheet's "Save as CSV" opens with a byte-order mark; dropped before
	// parsing, it cannot turn a quoted first header field into a malformed one.
	buffered := bufio.NewReader(r)
	if mark, err := buffered.Peek(len(utf8ByteOrderMark)); err == nil && bytes.Equal(mark, utf8ByteOrderMark) {
		if _, err := buffered.Discard(len(utf8ByteOrderMark)); err != nil {
			return nil, fmt.Errorf("deal-key-names: reading the export: %w", err)
		}
	}
	reader := csv.NewReader(buffered)
	reader.FieldsPerRecord = len(keyNameExportHeader)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("deal-key-names: reading the export header: %w", err)
	}
	if !slices.Equal(header, keyNameExportHeader) {
		return nil, fmt.Errorf("deal-key-names: line 1: the header must be %s",
			strings.Join(keyNameExportHeader, ","))
	}
	var entries []deals.KeyNamedDeal
	seen := map[[2]string]int{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("deal-key-names: %w", err)
		}
		line, _ := reader.FieldPos(0)
		entry := deals.KeyNamedDeal{
			SourceSystem: strings.TrimSpace(record[0]),
			SourceKey:    strings.TrimSpace(record[1]),
			SourceTitle:  strings.TrimSpace(record[2]),
		}
		switch {
		case entry.SourceSystem == "":
			return nil, fmt.Errorf("deal-key-names: line %d: source_system is empty", line)
		case entry.SourceKey == "":
			return nil, fmt.Errorf("deal-key-names: line %d: source_key is empty", line)
		case slices.ContainsFunc(record, isNotUTF8):
			return nil, fmt.Errorf("deal-key-names: line %d: a field is not valid UTF-8; save the export as UTF-8", line)
		case slices.ContainsFunc(record, hasControlCharacter):
			return nil, fmt.Errorf("deal-key-names: line %d: a field holds a control character", line)
		}
		pair := [2]string{entry.SourceSystem, entry.SourceKey}
		if first, dup := seen[pair]; dup {
			return nil, fmt.Errorf("deal-key-names: line %d: %s key %q already appears on line %d",
				line, entry.SourceSystem, entry.SourceKey, first)
		}
		seen[pair] = line
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errors.New("deal-key-names: the export has a header and no rows")
	}
	return entries, nil
}

// writeKeyNameReport renders one row per export entry the repair reached, the
// counts per outcome, and on a dry run how to make it real.
func writeKeyNameReport(stdout io.Writer, results []deals.KeyNameResult, applied bool) error {
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "OUTCOME\tDEAL\tSOURCE KEY\tNEW NAME")
	counts := map[deals.KeyNameOutcome]int{}
	for _, result := range results {
		counts[result.Outcome]++
		deal := ""
		if !result.DealID.IsZero() {
			deal = result.DealID.String()
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", result.Outcome, deal,
			printable(result.Entry.SourceKey), printable(result.To))
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("deal-key-names: writing the report: %w", err)
	}
	summary := []string{fmt.Sprintf("%d row(s)", len(results))}
	for _, outcome := range keyNameOutcomeOrder {
		if counts[outcome] > 0 {
			summary = append(summary, fmt.Sprintf("%d %s", counts[outcome], outcome))
		}
	}
	_, _ = fmt.Fprintf(stdout, "\n%s\n", strings.Join(summary, ", "))
	if !applied {
		_, _ = fmt.Fprintln(stdout, "Nothing was written. Run again with --apply to rename the deals marked would-rename.")
	}
	return nil
}

func isNotUTF8(field string) bool {
	return !utf8.ValidString(field)
}

func hasControlCharacter(field string) bool {
	return strings.ContainsFunc(field, unicode.IsControl)
}

// printable masks control characters in text bound for the operator's
// terminal: a deal's company and stage names are anyone's to edit in the app.
func printable(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}
		return r
	}, text)
}
