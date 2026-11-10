// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The pin follows identity changes, never a lookup that could choose a different subject.
func pinnedSubjectTx(ctx context.Context, tx pgx.Tx, lead ids.UUID, recipient connector.Recipient) (subjectRef, bool, error) {
	subject := subjectRef{Kind: entityLead, ID: lead.String()}
	if recipient.Channel != nil {
		return subject, false, nil
	}
	visited := map[string]bool{}
	for range 32 {
		key := subject.Kind + ":" + subject.ID
		if visited[key] {
			return subjectRef{}, false, nil
		}
		visited[key] = true
		subjectID, err := ids.Parse(subject.ID)
		if err != nil {
			return subjectRef{}, false, err
		}
		if subject.Kind == entityLead {
			err = auth.EnsureVisible(ctx, tx, "lead", subjectID)
		} else {
			err = auth.EnsureVisible(ctx, tx, "contact", subjectID)
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			return subjectRef{}, false, nil
		}
		if err != nil {
			return subjectRef{}, false, err
		}
		var merged, promoted *ids.UUID
		var archived, holds bool
		switch subject.Kind {
		case entityLead:
			err = tx.QueryRow(ctx, `SELECT l.merged_into_id, l.promoted_contact_id, l.archived_at IS NOT NULL,
 coalesce(lower(l.email) = lower(@address), false) OR EXISTS (SELECT 1 FROM communication_suppression s
 WHERE s.lead_id = l.id AND lower(s.address) = lower(@address))
 FROM lead l WHERE l.id = @pinned_id`, pgx.StrictNamedArgs{"pinned_id": subject.ID, "address": recipient.Email}).Scan(&merged, &promoted, &archived, &holds)
		case entityContact:
			err = tx.QueryRow(ctx, `SELECT c.merged_into_id, c.archived_at IS NOT NULL,
 EXISTS (SELECT 1 FROM contact_email e WHERE e.contact_id = c.id AND lower(e.email) = lower(@address))
 OR EXISTS (SELECT 1 FROM communication_suppression s WHERE s.contact_id = c.id AND lower(s.address) = lower(@address))
 FROM contact c WHERE c.id = @pinned_id`, pgx.StrictNamedArgs{"pinned_id": subject.ID, "address": recipient.Email}).Scan(&merged, &archived, &holds)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return subjectRef{}, false, nil
		}
		if err != nil {
			return subjectRef{}, false, fmt.Errorf("consent: following a controller message's subject: %w", err)
		}
		if merged != nil {
			subject.ID = merged.String()
			continue
		}
		if promoted != nil {
			subject.Kind, subject.ID = entityContact, promoted.String()
			continue
		}
		return subject, !archived && holds, nil
	}
	return subjectRef{}, false, nil
}

func recipientRequestCountry(ctx context.Context, tx pgx.Tx, recipient connector.Recipient, req commsauthz.Request) (RecipientCountry, error) {
	if req.Evidence.LeadID.IsZero() {
		return recipientCountryOf(ctx, tx, recipient)
	}
	subject, found, err := pinnedSubjectTx(ctx, tx, req.Evidence.LeadID, recipient)
	if err != nil || !found {
		return RecipientCountry{}, err
	}
	return recipientCountryTx(ctx, tx, ruleSubject{Kind: subject.Kind, ID: subject.ID})
}
