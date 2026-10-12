// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storeSource puts an uploaded file under a fresh key of this workspace.
func (h importHandlers) storeSource(ctx context.Context, body []byte, contentType string) (string, error) {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return "", fmt.Errorf("no workspace is bound to this request: %w", apperrors.ErrPermissionDenied)
	}
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](ws), importBlobKind, ids.NewV7().String())
	// Declared before the bytes exist. This key is handed back to the client and only
	// a submitted mapping makes it durable, so the ordinary outcome for an abandoned
	// profile is an object nothing ever names — which until now stayed forever.
	if err := migration.NewRunStore(h.db).RecordImportSourceIntent(ctx, key); err != nil {
		return "", err
	}
	if err := h.blobs.Put(ctx, key, bytes.NewReader(body), int64(len(body)), contentType); err != nil {
		return "", fmt.Errorf("storing the import source: %w", err)
	}
	return key, nil
}
