// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type nilRawPurger struct{}

func (*nilRawPurger) PurgeActivityProjectionsTx(context.Context, pgx.Tx, []ids.UUID) error {
	return nil
}
func (*nilRawPurger) PurgeRawCaptureTx(context.Context, pgx.Tx, []ids.UUID) error { return nil }
func (*nilRawPurger) PurgePartySourcesForEmailsTx(context.Context, pgx.Tx, []string) error {
	return nil
}

func TestATypedNilPurgerIsRefusedBeforeRetentionReadsAnything(t *testing.T) {
	t.Parallel()
	var missing *nilRawPurger
	service := NewRetentionService(nil, nil, nil, missing)
	err := service.EvaluateInstallation(principal.SystemActing(context.Background(), "retention-test"))
	if !errors.Is(err, ErrRetentionSeamMissing) {
		t.Fatalf("missing purger = %v", err)
	}
	if NewEraser(nil).WithRawCapturePurger(missing).purgeRawCaptures != nil {
		t.Fatal("eraser retained a typed nil purger")
	}
}
