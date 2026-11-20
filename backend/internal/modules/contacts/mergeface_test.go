// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Only contacts, companies and leads merge. Asked about any other type, the
// merge card's two reads answer not-found before they read a row.
func TestTheMergeCardReadsRefuseATypeThatDoesNotMerge(t *testing.T) {
	store, rows := &Store{}, []ids.UUID{ids.NewV7()}
	if _, err := store.DescribeForMerge(context.Background(), "deal", rows); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("describing deals for a merge = %v, want ErrNotFound", err)
	}
	if _, err := store.DecidableForMerge(context.Background(), "deal", rows); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("asking who may merge deals = %v, want ErrNotFound", err)
	}
}
