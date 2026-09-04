// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The answer the visibility gate gives before it ever reaches a database.

func TestAHandlerSetWithNoPoolRefusesRatherThanSpends(t *testing.T) {
	t.Parallel()

	// No pool means no transaction to ask through, so the check cannot be
	// performed — and an unperformable authority check refuses. The gate in
	// backend/gates/enrichmentpool_test.go keeps the wiring from reaching
	// here; this says what happens if one ever does.
	err := integrationsHandlers{}.requireVisibleContact(
		context.Background(), crmcontracts.Id{})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a handler set that cannot check visibility answered %v, want "+
			"permission denied — an authority check that cannot run must not pass", err)
	}
}
