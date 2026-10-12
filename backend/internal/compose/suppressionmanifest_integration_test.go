// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A worker booted against a restored database exports the restored list,
// which is older and shorter. The manifest must not move back with it, or the
// replay would claim to cover erasures the list no longer holds.
func TestAnExportFromARestoredListNeverMovesTheManifestBack(t *testing.T) {
	e := integration.Setup(t)
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(blobstore.NewMemory())
	older := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 3, 9, 14, 30, 0, 0, time.UTC)
	seedSuppressionAt(t, "frueher@betroffene.example", older)
	seedSuppressionAt(t, "spaeter@betroffene.example", newer)

	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("exporting the live list: %v", err)
	}
	before := readManifest(t, e, eraser)
	if !before.NewestErasure.Equal(newer) || before.Entries != 2 {
		t.Fatalf("manifest = %+v, want the newest erasure %s and 2 entries", before, newer)
	}

	// The restore takes the newer erasure out of the list.
	clearSuppressionList(t)
	seedSuppressionAt(t, "frueher@betroffene.example", older)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("exporting the restored list: %v", err)
	}

	after := readManifest(t, e, eraser)
	if !after.NewestErasure.Equal(newer) || after.Entries != 2 {
		t.Fatalf("manifest moved back to %+v after a restored export, want it left at %+v", after, before)
	}
}

func seedSuppressionAt(t *testing.T, email string, at time.Time) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		INSERT INTO erasure_suppression (kind, value_hash, created_at) VALUES ('email', $1, $2)`,
		storekit.SuppressionHash(email), at); err != nil {
		t.Fatalf("seeding the suppression: %v", err)
	}
}

func readManifest(t *testing.T, e *integration.Env, eraser *privacy.Eraser) privacy.JournalManifest {
	t.Helper()
	manifest, err := eraser.ReadJournalManifest(e.Admin())
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	return manifest
}
