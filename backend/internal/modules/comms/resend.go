// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Sending a bounced message again starts from the delivery that bounced. This
// file reads that delivery back as its author wrote it. Platform/mailresend
// decides which bounces may be sent again and which contact owns one. The
// Worklist, this read and the timeline all use that rule.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailresend"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ResendSource is a bounced delivery as its author wrote it. It holds the text
// without the sign-off and footer the send added, the addressees and the files.
// It also holds what the first send claimed about why it was sent.
type ResendSource struct {
	ID         ids.UUID
	ActivityID ids.UUID
	// ContactID is the contact whose address refused the message.
	ContactID ids.UUID
	Subject   string
	Body      string
	HTMLBody  string
	To        []string
	Cc        []string
	Bcc       []string
	// AttachmentIDs are the library files the original carried.
	AttachmentIDs  []ids.UUID
	ConsentPurpose string
	// ConnectionID is the mailbox the original went out from, zero when none.
	ConnectionID ids.UUID
	// Claims is what the original stated about why it was sent. The sending
	// activities module reads it back.
	Claims []byte
	// BounceRecipient is the address that refused.
	BounceRecipient string
}

// ResendSource answers the caller's own bounced send that may be sent again.
// Anything else answers not-found. That covers another user's send, a soft
// bounce, a send already repeated, a repeat itself and a send an agent wrote.
// It also covers one whose bounced address no readable, live contact holds.
func (s *Store) ResendSource(ctx context.Context, id ids.UUID) (ResendSource, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ResendSource{}, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	rule, err := mailresend.For(ctx, "o", "a", arg)
	if err != nil {
		return ResendSource{}, err
	}
	statement := `
SELECT o.id, o.activity_id, ` + rule.OwnerSQL() + `, COALESCE(o.subject, ''),
       left(o.body, o.authored_body_len), left(COALESCE(o.html_body, ''), COALESCE(o.authored_html_len, 0)),
       o.recipients, o.cc, COALESCE(o.bcc, '[]'::jsonb), o.attachments,
       COALESCE(o.consent_purpose, ''), o.connection_id, o.send_claims, o.bounce_recipient
  FROM comms_outbound o ` + rule.JoinSQL() + `
 WHERE o.id = ` + fmt.Sprintf("$%d", arg(id)) + ` AND ` + rule.EligibleSQL()
	var (
		source             ResendSource
		to, cc, bcc, files []byte
		connection         *ids.UUID
	)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, statement, args...).Scan(&source.ID, &source.ActivityID, &source.ContactID,
			&source.Subject, &source.Body, &source.HTMLBody, &to, &cc, &bcc, &files,
			&source.ConsentPurpose, &connection, &source.Claims, &source.BounceRecipient)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResendSource{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ResendSource{}, fmt.Errorf("comms: reading a bounced send to send again: %w", err)
	}
	if connection != nil {
		source.ConnectionID = *connection
	}
	if err := decodeResendSource(&source, to, cc, bcc, files); err != nil {
		return ResendSource{}, err
	}
	return source, nil
}

func decodeResendSource(source *ResendSource, to, cc, bcc, files []byte) error {
	for _, list := range []struct {
		raw  []byte
		into *[]string
	}{{to, &source.To}, {cc, &source.Cc}, {bcc, &source.Bcc}} {
		if err := json.Unmarshal(list.raw, list.into); err != nil {
			return fmt.Errorf("comms: decoding a bounced send's addressees: %w", err)
		}
	}
	var attached []OutboundFile
	if err := json.Unmarshal(files, &attached); err != nil {
		return fmt.Errorf("comms: decoding a bounced send's files: %w", err)
	}
	for _, file := range attached {
		source.AttachmentIDs = append(source.AttachmentIDs, file.AttachmentID)
	}
	return nil
}

// ownersOf answers, for each of the given bounced sends, the contact whose
// address refused it and whether the caller may send it again.
func (s *Store) ownersOf(ctx context.Context, bounced []ids.UUID) (map[ids.UUID]sendAgainOwner, error) {
	owners := make(map[ids.UUID]sendAgainOwner, len(bounced))
	if len(bounced) == 0 {
		return owners, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	rule, err := mailresend.For(ctx, "o", "a", arg)
	if err != nil {
		return nil, err
	}
	statement := `SELECT o.id, ` + rule.OwnerSQL() + `, ` + rule.EligibleSQL() + `
  FROM comms_outbound o ` + rule.JoinSQL() + `
 WHERE o.id = ANY(` + fmt.Sprintf("$%d", arg(bounced)) + `)`
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, statement, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var owner *ids.UUID
			var again bool
			if err := rows.Scan(&id, &owner, &again); err != nil {
				return err
			}
			found := sendAgainOwner{CanSendAgain: again}
			if owner != nil {
				found.ContactID = *owner
			}
			owners[id] = found
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("comms: finding the contact behind each bounce: %w", err)
	}
	return owners, nil
}

// sendAgainOwner is what the one rule says about a bounced send.
type sendAgainOwner struct {
	ContactID    ids.UUID
	CanSendAgain bool
}
