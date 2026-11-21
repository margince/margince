// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A new message's files reach the object store before its capture
// transaction opens.
//
// That transaction holds the shared authorization lock and a FOR SHARE on the
// connection row until it commits. An upload inside it made every role change
// wait for the object store, and every capture behind that change. So a short
// read asks the gates that keep files out first, and the files are stored when
// it finds none. The capture transaction still judges the message: it takes the
// stored files only when its own gates agree, and uploads itself when they
// differ. What it names nowhere is deleted once it has committed.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// prestageFiles stores rec's files ahead of its capture transaction when the
// short read finds no gate that keeps them out. The guess writes nothing.
func (s *Sink) prestageFiles(ctx context.Context, rec connector.NormalizedRecord) error {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || len(rec.Parts) == 0 || s.files == nil {
		return nil
	}
	var keep bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		keep, err = s.filesWouldBeKeptTx(ctx, tx, rec, fields)
		return err
	}); err != nil || !keep {
		return err
	}
	_, err := s.stageParts(ctx, rec)
	return err
}

// filesWouldBeKeptTx asks the gates finishNewActivity asks before it stages. A
// stored message stores no bytes again and internal mail is dropped. A private
// or copied-only message keeps only its files' names.
func (s *Sink) filesWouldBeKeptTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (bool, error) {
	if stored, _, err := s.mailHeldTx(ctx, tx, rec); err != nil || stored {
		return false, err
	}
	if internal, err := internalOnlyTx(ctx, tx, rec); err != nil || internal {
		return false, err
	}
	if private, _, err := messageIsPrivateTx(ctx, tx, rec, fields); err != nil || private {
		return false, err
	}
	copied, err := copiedOnlyTx(ctx, tx, fields)
	return !copied, err
}

// discardUnnamedFiles deletes what this capture stored and its committed
// transaction named nowhere. A failed delete leaves the object to the
// stored-object sweep, which reaps an intent no row cleared.
func (s *Sink) discardUnnamedFiles(ctx context.Context) {
	keeper, ok := s.files.(*retryFileKeeper)
	if !ok {
		return
	}
	unnamed := keeper.unnamed()
	if len(unnamed) == 0 {
		return
	}
	if err := keeper.FileKeeper.Discard(ctx, unnamed); err != nil {
		slog.WarnContext(ctx, "capture: deleting files a message did not keep; the stored-object sweep will reap them",
			"files", len(unnamed), "err", err)
	}
}

// discardFilesOfFailedCapture deletes what a capture stored before it failed.
// A failed commit may have committed, so its files are left to the sweep.
func (s *Sink) discardFilesOfFailedCapture(ctx context.Context) {
	keeper, ok := s.files.(*retryFileKeeper)
	if !ok || keeper.committing {
		return
	}
	keeper.recorded = false
	s.discardUnnamedFiles(ctx)
}

// beginFileAttempt starts one try of the capture transaction: what an earlier
// try recorded rolled back with it.
func (s *Sink) beginFileAttempt() {
	if keeper, ok := s.files.(*retryFileKeeper); ok {
		keeper.recorded, keeper.committing = false, false
	}
}

// notingCommit runs one try's work and notes whether it reached the commit.
func (s *Sink) notingCommit(work func(pgx.Tx) error) func(pgx.Tx) error {
	return func(tx pgx.Tx) error {
		err := work(tx)
		if keeper, ok := s.files.(*retryFileKeeper); ok {
			keeper.committing = err == nil
		}
		return err
	}
}
