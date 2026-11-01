// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// employerTwinSource names the queue rows an employment edge files.
const employerTwinSource = "employment_edge_recheck"

// fileEmployerTwins re-scores a contact and files its same-employer twins. The
// create's own duplicate check ran before the contact had an employer, so the
// shared employer counts only after the edge is written.
//
// A company merge calls it for every contact whose edge it moves.
func fileEmployerTwins(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, by string) error {
	return recheckContactForDuplicates(ctx, tx, contactID, employerTwinSource, by)
}

// promoteThenFileEmployerTwins promotes the contact's surviving employment and
// files the twins that promotion can create.
//
// Promoting is what makes an employer current primary, and only a current
// primary employer scores a twin. So the two belong together.
func promoteThenFileEmployerTwins(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, by string) error {
	if err := promoteLoneSurvivingEmployment(ctx, tx, contactID); err != nil {
		return err
	}
	return fileEmployerTwins(ctx, tx, contactID, by)
}
