// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The seat a business chat account files through. A company's WhatsApp, Telegram
// Business or Zalo OA account is a team seat, as a shared mailbox is. It is a
// capture_connection with no user and a reader team, held or shared.
// A member's own chat account is a member seat, which starts where a member's
// mailbox does. A message on either reads its posture from the seat. A chat is
// then shared or held as mail is, and the posture is set where a mailbox's is.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chatSeatProviders are the capture_connection providers of chat seats. A chat
// seat has no mail connector, so the mailbox lists and sweeps leave it out.
const chatSeatProviders = `('telegram_business', 'zalo_oa')`

type chatSeatKey struct{}

// withChatSeat marks a capture as filed through one chat seat.
func withChatSeat(ctx context.Context, seat ids.UUID) context.Context {
	return context.WithValue(ctx, chatSeatKey{}, seat)
}

// chatSeatOf answers the seat a capture files through, when it files through one.
func chatSeatOf(ctx context.Context) (ids.UUID, bool) {
	seat, ok := ctx.Value(chatSeatKey{}).(ids.UUID)
	return seat, ok && !seat.IsZero()
}

// chatPostureTx gives a chat message the posture of its seat. A shared seat
// leaves the decision as it stands, and anything else holds the message. A chat
// with no seat is held, as mail from a mailbox with no live connection is.
//
// A classified chat stays held: the confidentiality ledger judges a thread once,
// and a chat is one long thread.
func chatPostureTx(ctx context.Context, tx pgx.Tx, decision birthDecision) (birthDecision, error) {
	posture := PostureHeld
	if seat, ok := chatSeatOf(ctx); ok {
		read, err := storekit.RememberCaptureFact(ctx, tx, mailboxPostureMemo+seat.String(), func() (string, error) {
			return connectionPostureTx(ctx, tx, connectionMatch{where: "id = $1", args: []any{seat}, id: seat})
		})
		if err != nil {
			return birthDecision{}, err
		}
		posture = read
	}
	if posture == PostureShared {
		if decision.posture != "" {
			return decision, nil
		}
		return birthDecision{posture: posture}, nil
	}
	decision = decision.hold(postureReason(ctx))
	decision.posture = posture
	return decision, nil
}

// chatSeatProviderTx is the seat's provider, which an import row names as its
// delivering connector. The message's own provider differs ("telegram" arrives
// through a "telegram_business" seat).
func chatSeatProviderTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (string, error) {
	var provider string
	if err := tx.QueryRow(ctx, `SELECT provider FROM capture_connection WHERE id = $1`, seat).Scan(&provider); err != nil {
		return "", fmt.Errorf("capture: reading the chat seat's provider: %w", err)
	}
	return provider, nil
}

// ChatSeat names the account a seat is for.
type ChatSeat struct {
	// Provider is the seat's capture_connection provider.
	Provider string
	// Account is the provider's id of the account, unique per provider.
	Account string
	// Owner is the member behind a member-owned account; zero for a company's.
	Owner ids.UUID
	// ReaderTeam reads a company account's messages. Required for a new team
	// seat, and for a reconnect only when it names the team the seat already has.
	ReaderTeam ids.UUID
	// By is the admin who approved the account.
	By ids.UUID
}

// EnsureChatSeatTx gives an approved account its one seat, or wakes the one it
// had. A team seat starts held and a member seat classified, and a reconnect
// keeps the posture and the readers the seat already has.
func EnsureChatSeatTx(ctx context.Context, tx pgx.Tx, in ChatSeat) (ids.UUID, error) {
	if !in.Owner.IsZero() {
		return ensureMemberSeatTx(ctx, tx, in)
	}
	var prior struct {
		id      ids.UUID
		readers ids.UUID
		status  string
		posture string
	}
	err := tx.QueryRow(ctx, `
		SELECT id, reader_team_id, status, mail_posture FROM capture_connection
		 WHERE user_id IS NULL AND provider = $1 AND lower(account_label) = lower($2) FOR UPDATE`,
		in.Provider, in.Account).Scan(&prior.id, &prior.readers, &prior.status, &prior.posture)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, fmt.Errorf("capture: reading the chat seat of an account: %w", err)
	}
	if found && (in.ReaderTeam.IsZero() || in.ReaderTeam == prior.readers) {
		return prior.id, wakeChatSeatTx(ctx, tx, prior.id, prior.status)
	}
	if found {
		return ids.Nil, &SharedReconnectError{}
	}
	if in.ReaderTeam.IsZero() {
		return ids.Nil, &ReaderTeamError{missing: true}
	}
	if err := refuseUnreadableTeamTx(ctx, tx, in.ReaderTeam); err != nil {
		return ids.Nil, err
	}
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO capture_connection (provider, user_id, connected_by, reader_team_id, account_label, mail_posture, status)
		VALUES ($1, NULL, $2, $3, $4, $5, 'connected') RETURNING id`,
		in.Provider, in.By, in.ReaderTeam, in.Account, PostureHeld).Scan(&id); err != nil {
		return ids.Nil, fmt.Errorf("capture: creating the chat seat of an account: %w", err)
	}
	if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
		return ids.Nil, err
	}
	view := SharedMailboxView{Provider: in.Provider, Address: in.Account, Status: channelStatusConnected, ReaderTeam: in.ReaderTeam, Posture: PostureHeld}
	return id, announceSharedMailboxTx(ctx, tx, auditVerbCreate, sharedChangeConnected, id, nil, sharedMailboxImage(view))
}

// ensureMemberSeatTx seats a member's own account, one seat per account.
func ensureMemberSeatTx(ctx context.Context, tx pgx.Tx, in ChatSeat) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO capture_connection (provider, user_id, connected_by, account_label, mail_posture, status)
		VALUES ($1, $2, $3, $4, $5, 'connected')
		ON CONFLICT `+memberMailboxConflict+` DO UPDATE SET status = 'connected'
		RETURNING id`,
		in.Provider, in.Owner, in.By, in.Account, PostureClassified).Scan(&id)
	if err != nil {
		return ids.Nil, fmt.Errorf("capture: seating a member's chat account: %w", err)
	}
	return id, storekit.CaptureFactsChanged(ctx, tx)
}

func wakeChatSeatTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, status string) error {
	if status == channelStatusConnected {
		return nil
	}
	return setChatSeatStatusTx(ctx, tx, seat, channelStatusConnected)
}

// EndChatSeatTx ends a seat whose account was disconnected. The row, its mail
// and its readers stay, as a disconnected shared mailbox's do, and a reconnect of
// the same account wakes it.
func EndChatSeatTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) error {
	return setChatSeatStatusTx(ctx, tx, seat, channelStatusDisconnected)
}

func setChatSeatStatusTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, status string) error {
	var before SharedMailboxView
	var member bool
	err := tx.QueryRow(ctx, `
		SELECT provider, account_label, reader_team_id, mail_posture, status, (user_id IS NOT NULL) FROM capture_connection
		 WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, seat).
		Scan(&before.Provider, &before.Address, &before.ReaderTeam, &before.Posture, &before.Status, &member)
	if err != nil {
		return fmt.Errorf("capture: reading a chat seat: %w", err)
	}
	if before.Status == status {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_connection SET status = $2 WHERE id = $1 AND archived_at IS NULL`, seat, status); err != nil {
		return fmt.Errorf("capture: moving a chat seat to %s: %w", status, err)
	}
	if err := storekit.CaptureFactsChanged(ctx, tx); err != nil {
		return err
	}
	after := before
	after.Status = status
	change := sharedChangeConnected
	if status == channelStatusDisconnected {
		change = sharedChangeDisconnected
	}
	if member {
		return auditLifecycle(ctx, tx, auditVerbUpdate, captureConnectionObject, seat, sharedMailboxImage(before), sharedMailboxImage(after))
	}
	return announceSharedMailboxTx(ctx, tx, auditVerbUpdate, change, seat, sharedMailboxImage(before), sharedMailboxImage(after))
}
