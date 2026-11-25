// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extingress

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// ErrSubjectNotFound is a chosen subject the ingesting member cannot see. It
// answers the same for a record that is not there and one that is hidden, so it
// never confirms that a record exists.
var ErrSubjectNotFound = errors.New("extingress: no such contact or lead for this member")

// CheckSubject holds a chosen subject to the ingesting member's LIVE authority.
// ctx carries the member's principal. It is the read the member's own screens
// make: the object read grant, then the row scope capture's link probe applies.
// The sink asks the probe again inside the capture transaction, so a subject
// archived or re-shared in between is refused there too.
func CheckSubject(ctx context.Context, pool *pgxpool.Pool, subject datasource.EntityRef) error {
	table := string(subject.Type)
	if err := auth.Require(ctx, table, principal.ActionRead); err != nil {
		return refuseSubject(err)
	}
	return refuseSubject(database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return auth.EnsureLinkTarget(ctx, tx, table, subject.ID)
	}))
}

// refuseSubject maps an access or lookup refusal onto ErrSubjectNotFound and
// lets a real fault through.
func refuseSubject(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return ErrSubjectNotFound
	}
	return err
}

// EnsureChosenLink files a stored message on the subject a replay names. The
// member's authority was checked by CheckSubject, and the capture link probe
// checks it again.
func EnsureChosenLink(ctx context.Context, pool *pgxpool.Pool, activity datasource.EntityRef, subject datasource.EntityRef) error {
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return capture.EnsureChosenLinkTx(ctx, tx, ids.From[ids.ActivityKind](activity.ID), subject)
	})
}
