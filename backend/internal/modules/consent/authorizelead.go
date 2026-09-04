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

// The two columns person_consent keys a subject on. Compile-time literals, and
// the only values recordedStateFor's caller may pass — nothing off a request
// reaches the format below.
const (
	subjectColumnPerson = "person_id"
	subjectColumnLead   = "lead_id"
)

// recordedStateFor is the same read for either subject arm.
//
// One reader for both, because the STATE is what names the refusal a subject is
// later shown: a withdrawal is Art. 7(3) and an absence is default-deny, and
// they are different things that happened. A lead arm asking only "is it
// granted" could not tell them apart and reported every refusal as an absence —
// a false statement in a record the subject can obtain through Art. 15.
func recordedStateFor(ctx context.Context, tx pgx.Tx, subjectColumn, subjectID, purposeID string, requiresDOI bool) (string, bool, error) {
	var state string
	var granted bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT pc.state,
		       pc.state = 'granted' AND (NOT $3::boolean OR EXISTS (
		         SELECT 1 FROM consent_event ce
		         WHERE ce.%[1]s = pc.%[1]s AND ce.purpose_id = pc.purpose_id
		           AND ce.new_state = 'granted' AND ce.double_opt_in_confirmed_at IS NOT NULL
		           AND ce.issuance_trigger IS NOT NULL))
		FROM person_consent pc
		WHERE pc.%[1]s = $1 AND pc.purpose_id = $2`, subjectColumn),
		subjectID, purposeID, requiresDOI).Scan(&state, &granted)
	if errors.Is(err, pgx.ErrNoRows) {
		return string(StateUnknown), false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read the recorded consent state: %w", err)
	}
	return state, granted, nil
}

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
func (g *Gate) decideLead(ctx context.Context, tx pgx.Tx, r connector.Recipient, req commsauthz.Request, d commsauthz.Decision) (commsauthz.Decision, error) {
	purposeKey := req.LegacyPurposeKey
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
	d.ReasonCode, err = leadRefusalReason(ctx, tx, leadID, purpose)
	if err != nil {
		return commsauthz.Decision{}, err
	}
	return d, nil
}

// leadRefusalReason names WHY a lead's send was refused, in the same vocabulary
// the person arm uses.
//
// A withdrawal and an absence are different things that happened — Art. 7(3)
// against default-deny — and the reason code is what a subject is shown when
// they ask. The lead arm previously called every refusal an absence, because
// grantedForLead answers a bare bool; a lead who had exercised their right to
// withdraw would have been told the installation merely never had consent,
// which is a false statement in a record Art. 15 discloses.
//
// A grant that exists but never completed its round trip is its own answer
// again: the subject did agree, and the mailbox never confirmed it.
func leadRefusalReason(ctx context.Context, tx pgx.Tx, leadID string, purpose PurposeRow) (string, error) {
	state, _, err := recordedStateFor(ctx, tx, subjectColumnLead, leadID, purpose.ID, purpose.RequiresDOI)
	if err != nil {
		return "", err
	}
	switch ConsentState(state) {
	case StateWithdrawn:
		return commsauthz.ReasonConsentWithdrawn, nil
	case StateGranted:
		// Granted and still not authorized means the DOI round trip is what is
		// missing — the only other condition grantedForLead imposes.
		return commsauthz.ReasonUnconfirmedDOI, nil
	default:
		return commsauthz.ReasonNoMarketingConsent, nil
	}
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
