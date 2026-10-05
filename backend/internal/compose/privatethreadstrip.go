// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Withholding the files of personal-thread mail captured before the verdict.
//
// Capture owns the stored original and activities owns the attachment rows and
// their objects, so the two halves meet here, in one transaction per message:
// the original is rewritten without the bytes, the rows are marked withheld and
// the objects are deleted, or none of it happens.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// privateThreadStripBatch bounds one pass, as the personal sweep does: the
// backlog is a query, so what one tick leaves the next reaches.
const privateThreadStripBatch = 200

// privateThreadStripper withholds the stored files of personal-thread mail
// whose undo window has closed.
type privateThreadStripper struct {
	pool  *pgxpool.Pool
	files *activities.Store
}

// privateThreadStripperFor is nil without an object store: a strip that marked
// rows withheld and left their bytes in a bucket would report files as gone
// while they are not.
func privateThreadStripperFor(pool *pgxpool.Pool, blob blobstore.Store) *privateThreadStripper {
	if blob == nil {
		return nil
	}
	return &privateThreadStripper{pool: pool, files: activities.NewStore(InstallationDB(pool)).WithBlobstore(blob)}
}

// StripWorkspace withholds the due files of one workspace and reports how many
// messages it reached.
func (s *privateThreadStripper) StripWorkspace(ctx context.Context, windows capture.PersonalPurgeWindows) (int, error) {
	// The verdict job carries no actor; the strip writes audit rows, so it
	// names itself as the system act it is.
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:private-thread-strip",
	})
	var due []ids.UUID
	if err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		due, err = capture.SelectPrivateThreadFilesDueTx(ctx, tx, windows, statutoryFloor(), privateThreadStripBatch)
		return err
	}); err != nil {
		return 0, fmt.Errorf("verdict: finding personal-thread mail whose files are due: %w", err)
	}
	for i, activity := range due {
		if err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
			return s.stripMessage(ctx, tx, activity)
		}); err != nil {
			return i, fmt.Errorf("verdict: withholding a personal-thread message's files: %w", err)
		}
	}
	return len(due), nil
}

func (s *privateThreadStripper) stripMessage(ctx context.Context, tx pgx.Tx, activity ids.UUID) error {
	files, err := s.files.StoredFilesOfMessageTx(ctx, tx, activity)
	if err != nil || len(files) == 0 {
		return err
	}
	bodies := make([]capture.StoredBody, 0, len(files))
	for _, f := range files {
		bodies = append(bodies, capture.StoredBody{Ordinal: f.Ordinal, Key: f.Key, Body: f.Body})
	}
	if err := capture.WithholdStoredOriginalTx(ctx, tx, activity, bodies); err != nil {
		return err
	}
	return s.files.WithholdStoredFilesTx(ctx, tx, files)
}
