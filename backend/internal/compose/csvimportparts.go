// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// bundleSourceFor binds a run to its stored upload. A run that names parts was
// staged from a ZIP, even a ZIP of one file; any other run is one plain CSV.
// uploadCap is the cap the upload applied, so the walk accepts what it accepted.
//
//nolint:ireturn // a run is one CSVSource or one BundleSource; the engine takes either as a Source.
func bundleSourceFor(
	blobs blobstore.Store, run migration.RunMapping, sourceRef string, uploadCap int64,
) (migration.Source, func(object string) []migration.SkippedLine, error) {
	if len(run.Parts) == 0 {
		single := migration.NewCSVSource(blobs, sourceRef, run.Object, run.Fields, run.SourceKey)
		shapeAddressCells(single, run.Object, run.Fields)
		return single, func(string) []migration.SkippedLine { return single.Skipped() }, nil
	}
	parts := make([]*migration.CSVSource, 0, len(run.Parts))
	for _, part := range run.Parts {
		entry := migration.ZipPart{Key: sourceRef, Entry: part.File, Budget: uploadCap}
		partSource := migration.NewZipPartSource(blobs, entry, part.Object, part.Fields, part.SourceKey)
		shapeAddressCells(partSource, part.Object, part.Fields)
		parts = append(parts, partSource)
	}
	bundle, err := migration.NewBundleSource(parts...)
	if err != nil {
		return nil, nil, err
	}
	return bundle, bundle.Skipped, nil
}

// partSourceOf is the source of one type's file within what bundleSourceFor
// built.
func partSourceOf(source migration.Source, object string) (*migration.CSVSource, error) {
	switch s := source.(type) {
	case *migration.BundleSource:
		if part, ok := s.Part(object); ok {
			return part, nil
		}
	case *migration.CSVSource:
		if s.Objects()[0] == object {
			return s, nil
		}
	}
	return nil, fmt.Errorf("import: this run's source holds no %q file: %w", object, apperrors.ErrConflict)
}

// checkEveryPartsColumns runs checkPartColumns over each file of the run.
func checkEveryPartsColumns(ctx context.Context, source migration.Source, mapping migration.RunMapping) error {
	for _, part := range mapping.PartList() {
		partSource, err := partSourceOf(source, part.Object)
		if err != nil {
			return err
		}
		if err := checkPartColumns(ctx, partSource, part.Fields); err != nil {
			return err
		}
	}
	return nil
}

// predictEveryPart refines the dry run's report one part at a time, in import
// order, and names on each type the file it came from. The company part's rows
// are marked pending before the contact part is read, so a contact's link finds
// a company of the same upload.
func predictEveryPart(
	ctx context.Context, source migration.Source, skipped func(object string) []migration.SkippedLine,
	writers *bundleWriters, mapping migration.RunMapping, report migration.Report,
) (migration.Report, error) {
	fileOf := map[string]string{}
	for _, part := range mapping.PartList() {
		fileOf[part.Object] = part.File
	}
	// The engine's dry run counted the links offered; each part adds the ones
	// that will resolve.
	report.Associations = 0
	for _, object := range migration.ImportOrder {
		if _, ok := fileOf[object]; !ok {
			continue
		}
		partSource, err := partSourceOf(source, object)
		if err != nil {
			return migration.Report{}, err
		}
		partWriters, err := writers.part(object)
		if err != nil {
			return migration.Report{}, err
		}
		if report, err = refinePrediction(ctx, partSource, partWriters, object, report); err != nil {
			return migration.Report{}, err
		}
		report = withSkippedLines(report, object, skipped(object))
		report = withWarnings(report, object, newTagWarnings(writers.tags.missing, writers.tagFirstNamedBy, object))
	}
	for i := range report.Objects {
		report.Objects[i].File = fileOf[report.Objects[i].Object]
	}
	return report, nil
}
