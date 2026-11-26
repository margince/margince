// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

// Sending a bounced message again starts from the delivery that bounced. This
// file reads that delivery back as the author wrote it, and decides which
// bounces may be sent again. The two share one predicate, so the button the
// Worklist draws and the endpoint behind it cannot disagree about a row.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resendable holds when a bounce may be sent again: the caller's own user send
// that bounced hard, named the address that refused, kept its author's text,
// is not itself a repeat and has not been repeated. The first placeholder is
// the caller's user id.
//
// A repeat that bounces is not offered a third send, so the person page, where
// the address is fixed by hand, stays the way on.
const resendable = `o.user_id = $1
   AND o.sender_kind = 'user'
   AND o.channel_user_id IS NULL
   AND o.bounce_kind = 'hard'
   AND o.bounce_recipient IS NOT NULL
   AND o.authored_body_len IS NOT NULL
   AND o.resent_from IS NULL
   AND NOT EXISTS (SELECT 1 FROM comms_outbound r WHERE r.resent_from = o.id)`

// ResendSource is a bounced delivery as its author wrote it: the text without
// the sign-off and footer the send added, the addressees, the files and the
// consent purpose the original send went out under.
type ResendSource struct {
	ID         ids.UUID
	ActivityID ids.UUID
	// ContactID is the contact the send is filed under, whose address is the
	// one that refused.
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
	// BounceRecipient is the address that refused.
	BounceRecipient string
}

// ResendSource answers the caller's own bounced send that may be sent again.
// Anything else — another user's send, a soft bounce, a send already repeated,
// a repeat itself, one filed under no contact the caller can read — answers
// not-found, so a row the caller may not act on looks the same as one that does
// not exist.
func (s *Store) ResendSource(ctx context.Context, id ids.UUID) (ResendSource, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ResendSource{}, fmt.Errorf("sending a bounced message again needs a signed-in user: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ResendSource{}, err
	}
	args := []any{actor.UserID}
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := auth.LinkTargetReadableClause(ctx, "al", arg)
	if err != nil {
		return ResendSource{}, err
	}
	if visible == "" {
		visible = alwaysVisible
	}
	statement := fmt.Sprintf(`
SELECT o.id, o.activity_id, l.contact_id, COALESCE(o.subject, ''),
       left(o.body, o.authored_body_len), left(COALESCE(o.html_body, ''), COALESCE(o.authored_html_len, 0)),
       o.recipients, o.cc, COALESCE(o.bcc, '[]'::jsonb), o.attachments,
       COALESCE(o.consent_purpose, ''), o.bounce_recipient
  FROM comms_outbound o
  `+contactLateral(visible)+`
 WHERE o.id = $%d AND l.contact_id IS NOT NULL AND `+resendable, arg(id))
	var (
		source             ResendSource
		to, cc, bcc, files []byte
	)
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, statement, args...).Scan(&source.ID, &source.ActivityID, &source.ContactID,
			&source.Subject, &source.Body, &source.HTMLBody, &to, &cc, &bcc, &files,
			&source.ConsentPurpose, &source.BounceRecipient)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ResendSource{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ResendSource{}, fmt.Errorf("comms: reading a bounced send to send again: %w", err)
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

// sendAgainFrom answers which of the given bounced sends may be sent again,
// by the same predicate ResendSource reads with.
func (s *Store) sendAgainFrom(ctx context.Context, userID ids.UUID, bounced []ids.UUID) (map[ids.UUID]bool, error) {
	again := make(map[ids.UUID]bool, len(bounced))
	if len(bounced) == 0 {
		return again, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT o.id FROM comms_outbound o WHERE o.id = ANY($2) AND `+resendable, userID, bounced)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			again[id] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("comms: finding which bounces may be sent again: %w", err)
	}
	return again, nil
}
