// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// The export writes to the separate journal store, not the main one. A rollback
// of the main store then leaves the replay's input intact.
func TestAJournalOnItsOwnStoreSurvivesTheMainStoreRollingBack(t *testing.T) {
	e := integration.Setup(t)
	main, journal := blobstore.NewMemory(), blobstore.NewMemory()
	writer := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(main).WithJournalStore(journal)

	seedSuppressedEmail(t, resurrectedAddress)
	if _, err := writer.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	if n, err := main.DeletePrefix(e.Admin(), "erasure-suppression/"); err != nil || n != 0 {
		t.Fatalf("the main store held %d journal object(s), err %v; want none", n, err)
	}

	clearSuppressionList(t)
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)
	rolledBackMain := blobstore.NewMemory()
	reader := eraserWithGraph(InstallationDB(e.Pool)).WithBlobstore(rolledBackMain).WithJournalStore(journal)
	took, err := reader.ReapplySuppressions(e.Admin())
	if err != nil || took != 1 {
		t.Fatalf("re-erased %d, err %v; want the separate journal to have answered", took, err)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("%d address(es) survived", live)
	}
}
