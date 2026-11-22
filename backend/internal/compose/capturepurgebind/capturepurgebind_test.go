// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturepurgebind

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestCheckRefusesAnySelectionThatDiffersFromThePreview(t *testing.T) {
	d, r, a := 0, 1, 2
	ctx, err := Bind(context.Background(), &d, &r, &a)
	if err != nil {
		t.Fatalf("binding three counts: %v", err)
	}
	if err := Check(ctx, 0, 1, 2); err != nil {
		t.Errorf("the previewed selection was refused: %v", err)
	}
	for _, now := range [][3]int{{1, 1, 2}, {0, 2, 2}, {0, 1, 3}} {
		err := Check(ctx, now[0], now[1], now[2])
		if !errors.Is(err, ErrSelectionChanged) || !errors.Is(err, apperrors.ErrConflict) {
			t.Errorf("selection %v: err = %v, want a conflict", now, err)
		}
	}
	if err := Check(context.Background(), 5, 5, 5); err != nil {
		t.Errorf("no confirmed counts must bind nothing: %v", err)
	}
}

func TestBindRefusesAPartialConfirmation(t *testing.T) {
	d := 1
	if _, err := Bind(context.Background(), &d, nil, nil); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("err = %v, want an invalid argument", err)
	}
}
