// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"
)

// Only a fault makes the database refuse, so the refusal is injected here. What
// a stored state reads back as is proven against Postgres in
// installationstate_integration_test.go.
func TestAFailedStateWriteIsReported(t *testing.T) {
	refused := errors.New("the connection is gone")
	tx := &fakeTx{execErr: refused}
	ctx := context.Background()
	const key = InstallationStateKey("a_pass_under_test")
	if err := StoreInstallationStateTx(ctx, tx, key, struct{}{}); !errors.Is(err, refused) {
		t.Errorf("a failed store answered %v, want the cause", err)
	}
	if _, err := LockInstallationStateTx(ctx, tx, key, struct{}{}); !errors.Is(err, refused) {
		t.Errorf("a failed lock answered %v, want the cause", err)
	}
}
