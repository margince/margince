// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readerCtx() context.Context {
	user := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
	})
}

func TestACallerPastTwentyDocumentsAMinuteIsRefused(t *testing.T) {
	reader := readerCtx()
	for i := range invoiceDocumentsPerMinute {
		if err := admitDocumentFetch(reader); err != nil {
			t.Fatalf("fetch %d of %d refused: %v", i+1, invoiceDocumentsPerMinute, err)
		}
	}
	if err := admitDocumentFetch(reader); !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Errorf("fetch past the ceiling: %v, want ErrBudgetExceeded", err)
	}
	if err := admitDocumentFetch(readerCtx()); err != nil {
		t.Errorf("another caller was refused: %v; the ceiling is per caller", err)
	}
}

func TestADocumentFetchWithoutACallerIsRefused(t *testing.T) {
	if err := admitDocumentFetch(context.Background()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a fetch with no caller: %v, want ErrPermissionDenied", err)
	}
}
