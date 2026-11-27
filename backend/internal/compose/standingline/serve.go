// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The record kinds that keep a line, as standing_line's CHECK spells them.
const (
	KindContact = "contact"
	KindCompany = "company"
	KindProject = "project"
)

// ReadTx is the record's line for the reader on ctx. False when none was
// written or the reader may not open an activity it was written from.
func ReadTx(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) (crmcontracts.StandingLine, bool, error) {
	line, found, err := generatedsummary.ReadStandingLineTx(ctx, tx,
		generatedsummary.StandingRecord{EntityType: kind, EntityID: id})
	if err != nil || !found {
		return crmcontracts.StandingLine{}, false, err
	}
	// The object grant first: the row gate below asks only about the row.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.StandingLine{}, false, readable(err)
	}
	cited := make([]openapi_types.UUID, len(line.Evidence))
	for i, activity := range line.Evidence {
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activity); err != nil {
			return crmcontracts.StandingLine{}, false, readable(err)
		}
		cited[i] = openapi_types.UUID(activity)
	}
	return crmcontracts.StandingLine{Text: line.Text, ActivityIds: cited, WrittenAt: line.WrittenAt}, true, nil
}

// AttachTx sets a page's standing_line from ReadTx, and leaves it absent when
// the reader gets none.
func AttachTx(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, into **crmcontracts.StandingLine) error {
	line, ok, err := ReadTx(ctx, tx, kind, id)
	if ok {
		*into = &line
	}
	return err
}

// readable turns a refusal into no line, and passes any other failure on.
func readable(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}
