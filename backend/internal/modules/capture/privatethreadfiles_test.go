// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func stripCtx(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:imap",
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

// Every entry point of the strip asks for update on activity before it reads
// or writes a row, and a call with nothing to do does nothing. None of these
// reaches the transaction, which is why a nil one is enough here.
func TestThePrivateThreadStripRefusesBeforeItReads(t *testing.T) {
	readOnly := stripCtx(map[string]principal.ObjectGrant{"activity": {Read: true}})
	updater := stripCtx(map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}})
	windows := DefaultPersonalPurgeWindows()

	if _, err := SelectPrivateThreadFilesDueTx(readOnly, nil, windows, StatutoryFloor{}, 10); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("scan without update: %v, want ErrPermissionDenied", err)
	}
	if due, err := SelectPrivateThreadFilesDueTx(updater, nil, windows, StatutoryFloor{}, 0); err != nil || due != nil {
		t.Errorf("scan with no room: %v, %v; want nothing and no error", due, err)
	}
	if err := WithholdStoredOriginalTx(readOnly, nil, ids.NewV7(), []StoredBody{{Ordinal: 1, Body: []byte("x")}}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("rewrite without update: %v, want ErrPermissionDenied", err)
	}
	if err := WithholdStoredOriginalTx(updater, nil, ids.NewV7(), nil); err != nil {
		t.Errorf("rewrite with no files: %v, want nothing done", err)
	}
}
