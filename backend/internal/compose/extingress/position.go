// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extingress

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// NoMember is the member id a single-stream source's position is kept under, so
// that one table and one key serve both kinds of source.
var NoMember = ids.UUID{}

// ReadPosition answers a saved position and its version. A source with nothing
// saved answers the zero position at version 0.
func ReadPosition(ctx context.Context, tx pgx.Tx, unit, system string, member ids.UUID) (extension.MemberPosition, error) {
	var saved extension.MemberPosition
	var text string
	err := tx.QueryRow(ctx,
		`SELECT position, version FROM extension_ingest_position WHERE unit = $1 AND source_system = $2 AND member_id = $3`,
		unit, system, member).Scan(&text, &saved.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return extension.MemberPosition{}, nil
	}
	saved.Position = extension.BatchPosition(text)
	return saved, err
}

// SavePosition replaces a single-stream source's position, whatever it was.
func SavePosition(ctx context.Context, tx pgx.Tx, unit, system string, position extension.BatchPosition) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO extension_ingest_position (unit, source_system, member_id, position)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (unit, source_system, member_id) DO UPDATE
		   SET position = EXCLUDED.position, version = extension_ingest_position.version + 1`,
		unit, system, NoMember, string(position))
	return err
}

// SaveMemberPosition replaces one member's position if the saved version is
// still expected, and answers the new position. A save that read an older
// version, or that expects a first save while one exists, is a conflict.
func SaveMemberPosition(
	ctx context.Context, tx pgx.Tx, unit, system string, member ids.UUID,
	position extension.BatchPosition, expected extension.PositionVersion,
) (extension.MemberPosition, error) {
	var version extension.PositionVersion
	var err error
	if expected == 0 {
		err = tx.QueryRow(ctx, `
			INSERT INTO extension_ingest_position (unit, source_system, member_id, position)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (unit, source_system, member_id) DO NOTHING
			RETURNING version`, unit, system, member, string(position)).Scan(&version)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE extension_ingest_position
			   SET position = $4, version = version + 1
			 WHERE unit = $1 AND source_system = $2 AND member_id = $3 AND version = $5
			RETURNING version`, unit, system, member, string(position), expected).Scan(&version)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return extension.MemberPosition{}, extension.ErrPositionConflict
	}
	if err != nil {
		return extension.MemberPosition{}, fmt.Errorf("extingress: saving a member's position: %w", err)
	}
	return extension.MemberPosition{Position: position, Version: version}, nil
}
