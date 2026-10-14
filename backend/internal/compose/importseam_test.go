// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func TestARoleThatQueuesNoImportWorkRefusesToTestOne(t *testing.T) {
	storing := importSeam{handlers: importHandlers{blobs: blobstore.NewMemory()}}
	if _, err := storing.StageRun(t.Context(), crmcontracts.CreateImportRunRequest{}); !errors.Is(err, errNoImportQueue) {
		t.Fatalf("err = %v, want errNoImportQueue naming the missing queue", err)
	}
	if _, err := importsOverDB(nil).StageRun(t.Context(), crmcontracts.CreateImportRunRequest{}); !errors.Is(err, errNoObjectStore) {
		t.Fatalf("err = %v, want errNoObjectStore: a role with neither names the store first", err)
	}
}
