// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Whether each lead on a page is this caller's to change.
//
// A lead has no attach step the way a person and an organization do — nothing
// hangs off it that a second query fills in — so the three read paths (the
// single read, the list, and the work queue) would each have spelled this for
// themselves. One function instead, called by all three: a client that saw the
// list say writable and the record say otherwise would be reading two answers
// to one question.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// stampLeadsWritable answers the write question for a whole page in ONE
// statement, never a probe per row.
func stampLeadsWritable(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	_, err := auth.StampWritable(ctx, tx, "lead", leads,
		func(l crmcontracts.Lead) ids.UUID { return ids.UUID(l.Id) },
		func(l *crmcontracts.Lead, may bool) { l.Writable = &may })
	return err
}

// stampLeadWritable is the single-record spelling, so a caller holding one lead
// does not assemble a slice to ask.
func stampLeadWritable(ctx context.Context, tx pgx.Tx, lead *crmcontracts.Lead) error {
	one := []crmcontracts.Lead{*lead}
	if err := stampLeadsWritable(ctx, tx, one); err != nil {
		return err
	}
	*lead = one[0]
	return nil
}
