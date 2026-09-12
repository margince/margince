// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Minting a confirm link, on a transaction the caller owns.
//
// Split from the door that calls it so the preference centre can start a
// resubscribe inside the save that asked for it (resubscribe.go): the token
// row, the mail and the consent write then commit together or not at all.
//
// AND SO THE PLAINTEXT STAYS OUT. The confirm token is a bearer credential over
// one contact's record, and the census that follows it
// (gates/doitokenexposure_test.go) exists because a copy anywhere an operator
// can read it ends the claim that a grant made through it was the subject's
// own. Nothing here holds one: what crosses the boundary is the digest the row
// stores and the URL the mail carries, each derived by the caller that already
// has the token.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mintRequest is what the mint needs, and it deliberately holds no plaintext.
//
// The confirm token is a bearer credential over one contact's record, and the
// gate that follows it (gates/doitokenexposure_test.go) exists because a copy
// anywhere an operator can read it ends the claim that a grant made through it
// was the subject's own. So the two things the mint actually does with the
// token — hash it for the row, and build the URL for the mail — are done by the
// caller that already holds it, and what crosses this boundary is the DIGEST
// and the LINK.
type mintRequest struct {
	contactID       ids.ContactID
	kind            string
	purposeID       ids.PurposeID
	expectedAddress string
	tokenHash       string
	link            string
	// askedByTheSubject lifts the re-solicitation guard; see linkRequest.
	askedByTheSubject bool
}

// mintedLink is what the mint produces, also without the plaintext.
type mintedLink struct {
	expiresAt             time.Time
	deliveredTo           string
	staged                bool
	noticeCasesDischarged int
}

// issueLinkTx is the mint itself, on a transaction the caller owns.
//
// SPLIT OUT so the preference centre can start a resubscribe inside the save
// that asked for it (resubscribe.go). The token row, the mail and the consent
// write then commit together or not at all — a link minted beside a save that
// rolled back is a link nobody was sent for a choice nobody made.
//
// IT TAKES THE TOKEN rather than minting one, because the plaintext is the
// caller's to hold: issueLink returns it to an operator, and the resubscribe
// path never sees it at all.
//
// NO PERMISSION CHECK HERE. issueLink asks for contact:update above, and the
// resubscribe path is authorized by the preference token the subject is
// holding — their own mailbox, their own choice. Putting the operator's grant
// on this shared body would refuse the subject acting for themselves.
func (s *Store) issueLinkTx(
	ctx context.Context, tx pgx.Tx, req mintRequest,
) (mintedLink, error) {
	contactID, kind, purposeID := req.contactID, req.kind, req.purposeID
	var out mintedLink
	err := func(tx pgx.Tx) error {
		deliveredTo, err := admitLinkTx(ctx, tx, linkRequest{
			contactID:         req.contactID,
			purposeID:         purposeID,
			expectedAddress:   req.expectedAddress,
			askedByTheSubject: req.askedByTheSubject,
		})
		if err != nil {
			return err
		}
		issued := s.now().UTC()
		expires := issued.Add(confirmTokenTTL)
		// Per KIND, and per purpose within a kind. A fresh record-confirmation
		// link must not expire somebody's pending consent link and the other
		// way round: they ask different questions and arrive in different
		// mails, so superseding across them would silently kill an answer the
		// subject was still coming back to.
		if _, err := tx.Exec(ctx, `
			UPDATE confirm_token SET expires_at = $2
			WHERE contact_id = $1 AND consumed_at IS NULL AND expires_at > $2
			  AND kind = $3 AND purpose_id IS NOT DISTINCT FROM $4`,
			contactID, issued, kind, nullablePurpose(purposeID)); err != nil {
			return err
		}
		// A confirm_token row is a security artifact, not a kernel entity, so
		// the row id stays untyped — as consent_doi_token's does.
		// THE QUESTION THIS LINK WILL ASK, pinned now rather than read back when
		// the subject answers. The page renders in the installation's mail
		// language, and both that and the question's version can change between
		// the mail going out and the click coming back — so a grant resolving
		// either at proof time would name wording the subject never saw.
		//
		// The language is resolved through the same call that renders the mail
		// below, so the row and the page cannot disagree about which one.
		questionLocale := s.mailLanguage(ctx, tx)
		// WHICH question, decided from the link kind so the pin and the page
		// cannot disagree about what the subject will be asked.
		questionKey := QuestionKeyForLink(kind)
		var tokenRowID ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO confirm_token (contact_id, token_hash, delivered_to, issued_at, expires_at, kind, purpose_id,
			                           question_key, question_locale, question_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id`,
			contactID, req.tokenHash, deliveredTo, issued, expires,
			kind, nullablePurpose(purposeID),
			questionKey, questionLocale, marketingQuestionVersion).Scan(&tokenRowID); err != nil {
			return err
		}
		// The address is audited because it is the evidence: a later reader
		// asking why a grant counted needs to see which mailbox was reached.
		// The plaintext token never lands in audit or outbox payloads.
		if _, err := storekit.Audit(ctx, tx, "create", "confirm_token", tokenRowID, nil, map[string]any{
			contactIDKey:   contactID,
			"delivered_to": deliveredTo,
			"expires_at":   expires,
			auditKeyKind:   kind,
		}); err != nil {
			return err
		}
		out = mintedLink{expiresAt: expires, deliveredTo: deliveredTo}
		// The mail itself, on THIS transaction. The token row and the message
		// that carries it commit together or not at all: a token minted without
		// its mail is a link nobody was ever sent, and a mail staged without its
		// token is a link that resolves to nothing.
		staged, err := s.stageConfirmMail(ctx, tx, confirmMailInput{
			contactID:  contactID,
			recipient:  deliveredTo,
			kind:       kind,
			tokenRowID: tokenRowID,
			link:       req.link,
			expiresAt:  expires,
		})
		if err != nil {
			return err
		}
		out.staged = staged
		// The mail that discharges a duty is what moves the duty. A
		// record-confirmation link IS the Art. 14 disclosure route named in
		// allowed_routes, so sending one settles the cases that named it —
		// on this transaction, so a rolled-back mail leaves no duty marked
		// handled by a message nobody sent.
		//
		// GATED ON `staged`, which is not the same guarantee as the
		// transaction. An installation with no lane wired still mints the link
		// and reports queued=false — a supported outcome, not an error, so it
		// COMMITS. Discharging there would write an audit row saying a duty was
		// met by a message that was never staged, and the cooldown would then
		// suppress the genuine send once an operator fixed the relay.
		if route, discharges := noticeRouteFor(kind); discharges && staged {
			moved, err := dischargeNoticeCases(ctx, tx, contactID, route, issued)
			if err != nil {
				return err
			}
			out.noticeCasesDischarged = moved
		}
		return nil
	}(tx)
	if err != nil {
		return mintedLink{}, err
	}
	return out, nil
}
