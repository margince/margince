// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// profileBundle profiles every importable file of an uploaded ZIP and stores the
// ZIP once. The top-level single-file fields stay empty because the contract
// still requires them; each part carries its own.
func (h importHandlers) profileBundle(
	ctx context.Context, upload importUpload,
) (crmcontracts.ImportSourceProfile, error) {
	bundle, err := migration.OpenBundle(upload.Body, h.uploadLimit)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, importProblem(err)
	}
	// OpenBundle matches every part to an object of ImportOrder, so each has a rank.
	ordered := slices.Clone(bundle.Parts)
	slices.SortStableFunc(ordered, func(a, b migration.BundlePart) int {
		return slices.Index(migration.ImportOrder, a.Object) - slices.Index(migration.ImportOrder, b.Object)
	})
	catalogs, err := readImportCatalogs(ctx, h.db, migration.ImportOrder)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	parts := make([]crmcontracts.ImportPartProfile, 0, len(ordered))
	seeded := make([]migration.PartMapping, 0, len(ordered))
	rows := 0
	for _, part := range ordered {
		profiled, err := profilePart(ctx, bundle, part, catalogs)
		if err != nil {
			return crmcontracts.ImportSourceProfile{}, importProblem(err)
		}
		parts = append(parts, profiled)
		seeded = append(seeded, migration.PartMapping{
			File: part.Entry, Object: part.Object, Fields: profiled.SuggestedMappings[part.Object],
		})
		rows += profiled.RowCount
	}
	if err := refuseTooManyRows(rows); err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	ignored := make([]crmcontracts.ImportIgnoredEntry, 0, len(bundle.Ignored))
	for _, entry := range bundle.Ignored {
		ignored = append(ignored, crmcontracts.ImportIgnoredEntry{
			File: entry.Entry, Reason: crmcontracts.ImportIgnoredEntryReason(entry.Reason),
		})
	}

	key, err := h.storeSource(ctx, upload.Body, zipContentType)
	if err != nil {
		return crmcontracts.ImportSourceProfile{}, err
	}
	sourceName := suggestedSourceName(upload.FileName)
	out := crmcontracts.ImportSourceProfile{
		SourceRef:           key,
		FileName:            &upload.FileName,
		SuggestedSourceName: &sourceName,
		Parts:               &parts,
		Ignored:             &ignored,
		Columns:             []crmcontracts.ImportColumn{},
		RowCount:            rows,
		SuggestedMapping:    map[string]string{},
		Targets:             []string{},
	}
	return h.openDraft(ctx, out, migration.RunMapping{Parts: seeded, SourceName: sourceName, FileName: upload.FileName})
}

// profilePart reads one file of the bundle whole and profiles it. It suggests a
// mapping for every importable object, so the screen can change the file's type
// without asking again. Every error names the file.
func profilePart(
	ctx context.Context, bundle migration.Bundle, part migration.BundlePart, catalogs map[string]importCatalog,
) (crmcontracts.ImportPartProfile, error) {
	body, err := readBundleEntry(ctx, bundle, part.Entry)
	if err != nil {
		return crmcontracts.ImportPartProfile{}, err
	}
	if err := migration.CheckUTF8(body); err != nil {
		return crmcontracts.ImportPartProfile{}, fmt.Errorf("%s: %w", part.Entry, err)
	}
	profile, err := migration.ProfileCSV(bytes.NewReader(body), migration.ProfileRowLimit)
	if err != nil {
		return crmcontracts.ImportPartProfile{}, fmt.Errorf("%s: %w", part.Entry, err)
	}
	rows, err := migration.CountRecords(bytes.NewReader(body))
	if err != nil {
		return crmcontracts.ImportPartProfile{}, fmt.Errorf("%s: %w", part.Entry, err)
	}
	suggestions := make(map[string]map[string]string, len(migration.ImportOrder))
	for _, object := range migration.ImportOrder {
		targets, err := importTargets(object, catalogs[object])
		if err != nil {
			return crmcontracts.ImportPartProfile{}, err
		}
		suggestions[object] = suggestImportMapping(profile, targets, catalogs[object])
	}
	return crmcontracts.ImportPartProfile{
		File:              part.Entry,
		Object:            crmcontracts.ImportObject(part.Object),
		Columns:           toContractColumns(profile),
		RowsProfiled:      profile.RowsProfiled,
		RowCount:          rows,
		SuggestedMappings: suggestions,
	}, nil
}

// readBundleEntry reads one entry whole. Bundle.Open stops it at its declared
// size, and OpenBundle bounded the declared sizes, so the read is bounded too.
func readBundleEntry(ctx context.Context, bundle migration.Bundle, entry string) ([]byte, error) {
	rc, err := bundle.Open(entry)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rc.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing a bundle entry", "entry", entry, "err", cerr)
		}
	}()
	body, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", entry, err)
	}
	return body, nil
}
