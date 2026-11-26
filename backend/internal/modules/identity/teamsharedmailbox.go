// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A team that reads a shared mailbox grants its members that mailbox's mail.
// Adding a member then hands over a mailbox's whole history, and archiving the
// team leaves the mailbox with no reader at all.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamAuditKeySharedMailboxes names, on a member-added audit row, the shared
// mailboxes the new member can now read.
const teamAuditKeySharedMailboxes = "shared_mailboxes"

// sharedMailboxesReadByTx is the mailboxes the team reads; none without the seam.
func (s *Service) sharedMailboxesReadByTx(ctx context.Context, tx pgx.Tx, team ids.UUID) ([]string, error) {
	if s.mailCapture == nil {
		return nil, nil
	}
	return s.mailCapture.SharedMailboxesReadByTx(ctx, tx, team)
}

// refuseArchivingAReaderTeam keeps a shared mailbox from losing its readers.
func refuseArchivingAReaderTeam(mailboxes []string) error {
	if len(mailboxes) == 0 {
		return nil
	}
	return fmt.Errorf("%w: this team reads the shared mailbox %s; give the mailbox another reader team before archiving it",
		apperrors.ErrConflict, strings.Join(mailboxes, ", "))
}

// refuseAgentAddingAReader keeps an agent from granting a mailbox's mail on its
// own. The MCP tool's ask-first flag is fixed per tool, so the refusal is here:
// an admin adds the member in the app.
func refuseAgentAddingAReader(ctx context.Context, mailboxes []string) error {
	if len(mailboxes) == 0 {
		return nil
	}
	actor, ok := principal.Actor(ctx)
	if ok && actor.Type != principal.PrincipalAgent {
		return nil
	}
	return fmt.Errorf("%w: a member of this team reads the shared mailbox %s; an admin adds them in the app, not an agent",
		apperrors.ErrPermissionDenied, strings.Join(mailboxes, ", "))
}

// memberJoinsTx is asked before every way a member joins a team: an admin's
// add and an invite naming the team. Both are the inviter's or admin's act, so
// an agent behind either is refused. It answers the audit after-image, which
// names the shared mailboxes the new member can now read.
func (s *Service) memberJoinsTx(ctx context.Context, tx pgx.Tx, teamID, userID ids.UUID) (map[string]any, error) {
	mailboxes, err := s.sharedMailboxesReadByTx(ctx, tx, teamID)
	if err != nil {
		return nil, err
	}
	if err := refuseAgentAddingAReader(ctx, mailboxes); err != nil {
		return nil, err
	}
	after := map[string]any{teamAuditKeyMember: userID, "on": true}
	if len(mailboxes) > 0 {
		after[teamAuditKeySharedMailboxes] = mailboxes
	}
	return after, nil
}
