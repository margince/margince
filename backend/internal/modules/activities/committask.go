// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CommitmentTaskSource marks a task written for a commitment one of our users
// made in a captured conversation.
const CommitmentTaskSource = provenance.CommitmentTaskSource

// CommitmentTaskWritten reports whether a commitment's task was ever written,
// archived and completed ones included. An archived task is how a rep says
// "not mine to do", so the answer must outlive the archive or the next reading
// of the same conversation would write it again.
func (s *Store) CommitmentTaskWritten(ctx context.Context, tx pgx.Tx, locators ...string) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	var written bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity
		                WHERE source_system = $1 AND source_id = ANY($2::text[]))`,
		CommitmentTaskSource, locators).Scan(&written); err != nil {
		return false, fmt.Errorf("activities: reading whether the commitment's task exists: %w", err)
	}
	return written, nil
}

// VisibleOnlyTo narrows a new activity's audience to one member. A task read
// out of mail only one member may read must not be readable by anybody else
// through the records it is filed against.
func (in *LogActivityInput) VisibleOnlyTo(user ids.UUID) {
	in.audienceMembers = []AudienceMember{{
		SubjectType: string(crmcontracts.AudienceMemberSubjectTypeUser), SubjectID: user,
	}}
}

// MessageParties is who wrote one message and who it went to, as capture
// recorded them.
type MessageParties struct {
	From, To, Cc          []crmcontracts.EmailParty
	Organizers, Attendees []crmcontracts.EmailParty
}

// PartiesOf reads who wrote one message and who it went to.
func (s *Store) PartiesOf(ctx context.Context, tx pgx.Tx, message ids.UUID) (MessageParties, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return MessageParties{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return MessageParties{}, err
	}
	// The parties of a message are part of what the message says: a caller who
	// may not open it, or a message since archived, names nobody.
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, message); err != nil {
		return MessageParties{}, err
	}
	parties, err := readEmailParties(ctx, tx, ids.From[ids.ActivityKind](message))
	if err != nil {
		return MessageParties{}, err
	}
	return MessageParties{
		From: parties.from, To: parties.to, Cc: parties.cc,
		Organizers: parties.organizers, Attendees: parties.attendees,
	}, nil
}
