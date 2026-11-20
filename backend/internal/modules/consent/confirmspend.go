// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Redeeming a confirm link: naming whose it is, then spending it once.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectOfConfirmTokenTx names whose link this is, without taking a row lock.
//
// The submit needs the subject before it locks anything, because the subject
// row is the first lock its transaction may take. Art. 17 erasure holds the
// subject and then deletes these token rows. A transaction touching the token
// first would close a cycle.
//
// Naming the subject is not authorization. The spend below redeems the link,
// and it runs under the subject lock this read makes possible.
func (s *Store) subjectOfConfirmTokenTx(ctx context.Context, tx pgx.Tx, token string) (subject, string, bool, error) {
	var contactID ids.ContactID
	var leadID ids.LeadID
	var kind string
	var asksNews bool
	// The kind comes back with the subject so a submission wider than the mail
	// is refused before the link is spent. Refusing after would burn the
	// subject's one chance to answer.
	err := tx.QueryRow(ctx, `
		SELECT coalesce(ct.contact_id, '00000000-0000-0000-0000-000000000000'),
		       coalesce(ct.lead_id, '00000000-0000-0000-0000-000000000000'), ct.kind, ct.asks_news
		  FROM confirm_token ct
		 WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
		   AND `+liveLinkSubjectSQL, hashPublicToken(token), s.now().UTC()).Scan(&contactID, &leadID, &kind, &asksNews)
	if errors.Is(err, pgx.ErrNoRows) {
		return subject{}, "", false, fmt.Errorf("confirm token: %w", apperrors.ErrNotFound)
	}
	return linkSubjectOf(contactID, leadID), kind, asksNews, err
}

// liveLinkSubjectSQL says a link's subject is live. A lead's only submittable
// link is its consent question; its privacy notice is read, never answered.
const liveLinkSubjectSQL = `(EXISTS (SELECT 1 FROM contact p WHERE p.id = ct.contact_id AND p.archived_at IS NULL)
		    OR (ct.kind = '` + LinkConsentConfirmation + `'
		        AND EXISTS (SELECT 1 FROM lead l WHERE l.id = ct.lead_id AND l.archived_at IS NULL)))`

// spendConfirmTokenTx marks the link used, in the caller's transaction, so the
// submit it authorizes and the spend commit together. A token no longer live
// is refused, so a replayed submit writes nothing.
//
// A MailboxProof is only reachable through a token this statement could spend.
func (s *Store) spendConfirmTokenTx(ctx context.Context, tx pgx.Tx, token string) (ConfirmRef, error) {
	var ref ConfirmRef
	var purposeID *ids.PurposeID
	// The published question this link pinned, which the grant names.
	var questionKey *string
	var questionLocale *string
	var questionVersion *int
	err := tx.QueryRow(ctx, `
		UPDATE confirm_token ct SET consumed_at = $2
		WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
		  AND `+liveLinkSubjectSQL+`
		RETURNING coalesce(ct.contact_id, '00000000-0000-0000-0000-000000000000'),
		          coalesce(ct.lead_id, '00000000-0000-0000-0000-000000000000'), ct.id, ct.delivered_to, ct.kind, ct.purpose_id,
		          ct.question_key, ct.question_locale, ct.question_version, ct.asks_news,
		          coalesce(ct.subject_request_id, '00000000-0000-0000-0000-000000000000')`,
		hashPublicToken(token), s.now().UTC()).Scan(
		&ref.ContactID, &ref.LeadID, &ref.TokenID, &ref.DeliveredTo, &ref.Kind, &purposeID,
		&questionKey, &questionLocale, &questionVersion, &ref.AsksNews, &ref.SubjectRequest)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmRef{}, fmt.Errorf("confirm token: %w", apperrors.ErrNotFound)
	}
	if purposeID != nil {
		ref.PurposeID = *purposeID
	}
	if questionKey != nil {
		ref.QuestionKey = *questionKey
	}
	if questionLocale != nil {
		ref.QuestionLocale = *questionLocale
	}
	if questionVersion != nil {
		ref.QuestionVersion = *questionVersion
	}
	return ref, err
}
