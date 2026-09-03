// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the engine answers about a recipient who is a LEAD rather than a person.
//
// Its own file because authorizetransmit.go had reached the size cap, and
// because this is one concept: a lead is a subject the engine can identify, and
// everything here is about telling that case apart from the one where nobody
// could be identified at all. The difference matters more than it looks — a
// no-subject verdict is absolute and denies in every rollout mode, so recording
// an identified lead as nobody would refuse the send whatever the installation
// had configured.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// entityLead is the subject kind a decision records for an unpromoted lead,
// the sibling of entityPerson.
const entityLead = "lead"

// purposeRowFor reads the live purpose a delivery was staged under, or nil when
// nothing defines that key.
//
// One reader for both subject arms. A second copy in the lead arm would be a
// second answer to "what is this purpose", and the one that stopped matching —
// after an archived_at check moved, say — would look exactly like the one that
// still did.
func purposeRowFor(ctx context.Context, tx pgx.Tx, purposeKey string) (PurposeRow, bool, error) {
	var purpose PurposeRow
	err := tx.QueryRow(ctx,
		`SELECT id, key, label, class, requires_double_opt_in
		   FROM consent_purpose WHERE key = $1 AND archived_at IS NULL`,
		normalizedPurposeKey(purposeKey)).Scan(
		&purpose.ID, &purpose.Key, &purpose.Label, &purpose.Class, &purpose.RequiresDOI)
	if errors.Is(err, pgx.ErrNoRows) {
		// Not an error: an undefined purpose is an ANSWER about this send, and
		// the caller records it as a denial rather than failing the dispatch.
		return PurposeRow{}, false, nil
	}
	if err != nil {
		return PurposeRow{}, false, err
	}
	return purpose, true, nil
}

// decideLead answers about a recipient who is a lead rather than a person.
//
// It asks the same question the legacy lead arm asks — grantedForLead — rather
// than a second one of its own. Two implementations of "may we write to this
// lead" would be two answers, and the one that stopped matching would look
// exactly like the one that still did.
//
// A lead nothing resolves to stays `review` with no subject: that is the
// honest answer, and it is the one shape here that must not become an allow,
// because no suppression, objection or consent state can be read for a subject
// nobody identified.
func (g *Gate) decideLead(ctx context.Context, tx pgx.Tx, r connector.Recipient, purposeKey string, d commsauthz.Decision) (commsauthz.Decision, error) {
	leadID, found, err := resolveLead(ctx, tx, r)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !found {
		d.Verdict = commsauthz.VerdictReview
		d.ReasonCode = commsauthz.ReasonNoSubject
		return d, nil
	}
	parsed, err := ids.Parse(leadID)
	if err != nil {
		return commsauthz.Decision{}, fmt.Errorf("consent: the resolved lead is not an id: %w", err)
	}
	d.SubjectKind, d.SubjectID = entityLead, parsed

	// A suppression binds a lead exactly as it binds a person: the row may
	// name a lead_id or bare address, and liveSuppression already reads both.
	suppressed, kind, err := liveSuppression(ctx, tx, leadID, r)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if suppressed {
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = kind
		d.Suppression = kind
		return d, nil
	}

	purpose, defined, err := purposeRowFor(ctx, tx, purposeKey)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if !defined {
		d.Verdict = commsauthz.VerdictDeny
		d.ReasonCode = commsauthz.ReasonUnknownPurpose
		return d, nil
	}
	d.Resolved = categoryForClass(purpose.Class)
	granted, err := grantedForLead(ctx, tx, r, purpose.ID, purpose.RequiresDOI)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	if granted {
		d.Verdict = commsauthz.VerdictAllow
		d.ReasonCode = commsauthz.ReasonAllowed
		return d, nil
	}
	d.Verdict = commsauthz.VerdictDeny
	d.ReasonCode = commsauthz.ReasonNoMarketingConsent
	return d, nil
}

// resolveLead names the live lead behind an address, or reports that none does.
// Ambiguity is not possible to resolve here and is not guessed at: two live
// leads on one address answer `found=false`, which decideLead records as a
// recipient with no single subject.
func resolveLead(ctx context.Context, tx pgx.Tx, r connector.Recipient) (string, bool, error) {
	if r.Channel != nil {
		// A channel identity binds a Person and nothing else, so there is no
		// lead behind one.
		return "", false, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM lead
		 WHERE lower(email) = lower($1) AND archived_at IS NULL
		 LIMIT 2`, r.Email)
	if err != nil {
		return "", false, fmt.Errorf("consent: resolve the lead: %w", err)
	}
	defer rows.Close()
	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", false, fmt.Errorf("consent: resolve the lead: %w", err)
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("consent: resolve the lead: %w", err)
	}
	if len(matches) != 1 {
		return "", false, nil
	}
	return matches[0], true, nil
}
