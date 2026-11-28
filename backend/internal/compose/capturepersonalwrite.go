// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (p *CapturePurger) purgePersonalSelection(ctx context.Context, subject capture.PurgeSubject, seat ids.UUID, windows capture.PersonalPurgeWindows) (int, error) {
	destroyed := 0
	for _, id := range append(slices.Clone(subject.SoleImports), subject.SharedImports...) {
		n, err := p.purgePersonalMessage(ctx, id, seat, windows)
		if err != nil {
			return destroyed, err
		}
		destroyed += n
	}
	return destroyed, nil
}

func (p *CapturePurger) purgePersonalMessage(ctx context.Context, id, seat ids.UUID, windows capture.PersonalPurgeWindows) (int, error) {
	destroyed := 0
	err := database.WithWriteTx(ctx, p.pool, func(ctx context.Context) error {
		if err := database.WithTx(ctx, p.pool, func(tx pgx.Tx) error {
			return capture.LockPersonalPurgeSenderTx(ctx, tx, seat, id)
		}); err != nil {
			return err
		}
		var shared bool
		check := func(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
			floor, _, err := purgeFloorTx(ctx, tx)
			if err != nil {
				return false, err
			}
			current, err := capture.SelectPersonalPurgeTx(ctx, tx, seat, windows, floor, 1, id)
			if err != nil {
				return false, err
			}
			shared = slices.Contains(current.SharedImports, id)
			return slices.Contains(current.SoleImports, id), nil
		}
		var err error
		destroyed, err = p.retention.PurgeActivitiesIf(ctx, []ids.UUID{id}, privacy.PurgePersonalVerdict, check)
		if err != nil || !shared {
			return err
		}
		_, err = p.carryOut(ctx, capture.PurgeSubject{SharedImports: []ids.UUID{id}}, nil, nil, seat, privacy.PurgePersonalVerdict)
		return err
	})
	return destroyed, err
}
