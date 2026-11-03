// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The contact step runs only as the system, refuses a cursor it cannot read,
// and estimates only for a reader of data tasks.
func TestTheSeatContactStepRefusesWhatItCannotTrust(t *testing.T) {
	ctx := context.Background()
	if _, err := archiveSeatContactsBatch(ctx, nil, datatask.BatchIn{Limit: 10}); err == nil {
		t.Error("a batch ran without the system principal")
	}
	system := principal.SystemActing(ctx, "system:datatask-test")
	if _, err := archiveSeatContactsBatch(system, nil, datatask.BatchIn{Limit: 10, Cursor: []byte(`"not a cursor"`)}); err == nil {
		t.Error("a batch ran from a cursor that names no row")
	}
	if _, _, err := estimateSeatContacts(ctx, nil, 10); err == nil {
		t.Error("a caller with no principal estimated the contact step")
	}
}
