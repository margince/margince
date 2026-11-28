// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureMailFile stages and records one file on a mail through capture's own
// writer, into blob, and returns its attachment id.
func (e *Env) CaptureMailFile(t *testing.T, blob blobstore.Store, mail ids.UUID, filename, contentType string, body []byte) ids.UUID {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithActor(context.Background(),
		principal.Principal{
			Type: principal.PrincipalConnector, ID: "connector:imap",
			Permissions: principal.Permissions{
				Objects:  map[string]principal.ObjectGrant{"activity": {Create: true}},
				RowScope: principal.RowScopeAll,
			},
		}), ids.NewV7())
	store := activities.NewStore(e.DB()).WithBlobstore(blob)
	staged, err := store.StageCapturedFiles(ctx, []activities.CapturedFile{{
		PartID: filename, Filename: filename, ContentType: contentType, Body: body,
	}})
	if err != nil {
		t.Fatalf("staging %s: %v", filename, err)
	}
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return store.RecordCapturedFiles(ctx, tx, ids.From[ids.ActivityKind](mail), activities.CapturedFileSource{
			System: "imap", MessageID: "m-" + mail.String(), CapturedBy: "connector:imap", Category: "email_attachment",
		}, staged)
	}); err != nil {
		t.Fatalf("recording %s: %v", filename, err)
	}
	var id ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM attachment WHERE activity_id = $1 AND filename = $2`, mail, filename).Scan(&id); err != nil {
		t.Fatalf("reading back %s: %v", filename, err)
	}
	return id
}
