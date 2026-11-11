// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capturetimeline adapts the timeline store to capture's seams: the
// attachment store a captured message files into, and the meeting-notes landing.
//
// Capture reads the mailbox and owns the transaction; activities owns the
// attachment table. Neither imports the other, so the join is here, and each
// table keeps one writer.
package capturetimeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errStagedFileNotOurs marks a staged file that did not come from this
// adapter's own Stage call: a wiring mistake, never a runtime condition.
var errStagedFileNotOurs = errors.New(
	"capturetimeline: a captured file was staged by something other than the timeline store",
)

// FileKeeper adapts the timeline store to capture's seam.
type FileKeeper struct {
	Store *activities.Store
}

var _ capture.FileKeeper = FileKeeper{}

// Stage hands the files to the timeline store to keep until Record.
func (k FileKeeper) Stage(
	ctx context.Context, files []capture.CapturedFile,
) ([]capture.StagedFile, error) {
	owned := make([]activities.CapturedFile, 0, len(files))
	for _, file := range files {
		owned = append(owned, activities.CapturedFile{
			PartID:       file.PartID,
			Filename:     file.Filename,
			ContentType:  file.ContentType,
			DeclaredType: file.DeclaredType,
			Body:         file.Body,
		})
	}
	staged, err := k.Store.StageCapturedFiles(ctx, owned)
	if err != nil {
		return nil, err
	}
	// Returned opaque: capture carries these from Stage to Record and never
	// reads inside one, which is what keeps the row shape the owner's business.
	out := make([]capture.StagedFile, 0, len(staged))
	for _, file := range staged {
		out = append(out, file)
	}
	return out, nil
}

// Record files the staged files on the activity, inside capture's transaction.
func (k FileKeeper) Record(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, staged []capture.StagedFile,
) error {
	owned := make([]activities.StagedFile, 0, len(staged))
	for _, file := range staged {
		typed, ok := file.(activities.StagedFile)
		if !ok {
			// Only activities.StagedFile meets the marker, so this adapter never
			// gets here. A skipped file would break the promise that what
			// arrived is what is stored, so it fails instead.
			return errStagedFileNotOurs
		}
		owned = append(owned, typed)
	}
	return k.Store.RecordCapturedFiles(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, owned)
}

// RecordCopiedWithheld files a copied message's files by size and fingerprint only.
func (k FileKeeper) RecordCopiedWithheld(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, files []capture.CapturedFile,
) error {
	// The size and a fingerprint cross, never the body. The fingerprint is what
	// lets a later addressed arrival be proven the same file, which is how its
	// bytes reach this row at all.
	withheld := make([]activities.WithheldFile, 0, len(files))
	for _, file := range files {
		sum := sha256.Sum256(file.Body)
		withheld = append(withheld, activities.WithheldFile{
			PartID:       file.PartID,
			Filename:     file.Filename,
			ContentType:  file.ContentType,
			DeclaredType: file.DeclaredType,
			ByteSize:     len(file.Body),
			Checksum:     hex.EncodeToString(sum[:]),
		})
	}
	return k.Store.RecordWithheldFiles(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, withheld)
}

// AdoptWithheld gives withheld rows the bytes of the same files arriving addressed.
func (k FileKeeper) AdoptWithheld(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, files []capture.CapturedFile,
) (int, error) {
	carried := make([]activities.CapturedFile, 0, len(files))
	for _, file := range files {
		carried = append(carried, activities.CapturedFile{
			PartID:       file.PartID,
			Filename:     file.Filename,
			ContentType:  file.ContentType,
			DeclaredType: file.DeclaredType,
			Body:         file.Body,
		})
	}
	return k.Store.AdoptWithheldFiles(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, carried)
}

// Stored reports where each staged file landed in the object store.
func (k FileKeeper) Stored(staged []capture.StagedFile) ([]capture.StoredFile, error) {
	out := make([]capture.StoredFile, 0, len(staged))
	for _, file := range staged {
		typed, ok := file.(activities.StagedFile)
		if !ok {
			return nil, errStagedFileNotOurs
		}
		part, key, checksum, size := typed.Location()
		out = append(out, capture.StoredFile{PartID: part, StorageKey: key, Sha256: checksum, Bytes: int64(size)})
	}
	return out, nil
}

// RecordWithheld files the size of each file whose body is not kept.
func (k FileKeeper) RecordWithheld(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, files []capture.CapturedFile,
) error {
	// Only the size crosses: the body is what this row exists not to keep.
	withheld := make([]activities.WithheldFile, 0, len(files))
	for _, file := range files {
		withheld = append(withheld, activities.WithheldFile{
			PartID:       file.PartID,
			Filename:     file.Filename,
			ContentType:  file.ContentType,
			DeclaredType: file.DeclaredType,
			ByteSize:     len(file.Body),
		})
	}
	return k.Store.RecordWithheldFiles(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, withheld)
}

// RetireLinks retires the message's links that are not in kept.
func (k FileKeeper) RetireLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, kept []string,
) (int, error) {
	return k.Store.RetireCapturedLinks(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, kept)
}

// RestoreLinks brings back the message's links that are listed again.
func (k FileKeeper) RestoreLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, listed []string,
) (int, error) {
	return k.Store.RestoreCapturedLinks(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, listed)
}

// RecordLinks files the links a message carries.
func (k FileKeeper) RecordLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from capture.FileSource, links []capture.MailLink,
) (int, error) {
	owned := make([]activities.CapturedLink, 0, len(links))
	for _, link := range links {
		owned = append(owned, activities.CapturedLink{URL: link.URL, Text: link.Text})
	}
	return k.Store.RecordCapturedLinks(ctx, tx, activityID, activities.CapturedFileSource{
		System:     from.System,
		MessageID:  from.MessageID,
		CapturedBy: from.CapturedBy,
		Category:   from.Category,
	}, owned)
}
