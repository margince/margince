// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type activityLanding struct {
	ref      datasource.EntityRef
	created  bool
	decision counterpartyDecision
	internal bool
}

type captureAttempt struct {
	activityLanding
	duplicate       *ids.LeadID
	duplicateFields json.RawMessage
	dropped         string
}

func (s *Sink) landActivityTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields, owner ids.UUID) (activityLanding, error) {
	var landed activityLanding
	var err error
	if rec, fields, err = s.readMailForTheSeatTx(ctx, tx, owner, rec, fields); err != nil {
		return landed, err
	}
	// The privacy check must inspect the provider copy after resolving its owner.
	if rec, err = storeOriginalTx(ctx, tx, rec); err != nil {
		return landed, err
	}
	if internalMailAdmitted(ctx) {
		rec.Links = nil
	}
	landed.ref, landed.created, landed.decision, err = s.captureActivity(ctx, tx, rec, fields)
	if err != nil {
		return landed, err
	}
	landed.internal = internalMailAdmitted(ctx)
	if landed.created {
		return landed, nil
	}
	err = tx.QueryRow(ctx, `SELECT internal_mail_authorization IS NOT NULL FROM activity WHERE id = $1 AND restricted_at IS NULL`, landed.ref.ID).Scan(&landed.internal)
	return landed, err
}

// occurredPrecision is the activity column's word for DateOnly.
func (f ActivityFields) occurredPrecision() string {
	if f.DateOnly {
		return string(crmcontracts.OccurredPrecisionOccurredPrecisionDay)
	}
	return string(crmcontracts.OccurredPrecisionOccurredPrecisionInstant)
}
