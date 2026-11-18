// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The member's own LinkedIn account (ADR-0078 §2.1b).
//
// Onboarding asks for a profile URL and an authorization; this is where that
// answer lives, and where the integrations tab reads it back so a member can
// see and correct what the CRM believes about their own account.
//
// It is always the CALLER's row. There is no path here to read or write
// somebody else's LinkedIn account: a colleague's professional network is
// theirs, and an admin has no more business editing that URL than editing
// their personal address book.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// LinkedInAccount is what one member's LinkedIn connection amounts to.
type LinkedInAccount struct {
	ProfileURL  *string
	ConnectedAt *time.Time
	// Connections is how many ghosts this member's import produced, so the tab
	// can say what the authorization actually yielded rather than only that it
	// happened.
	Connections int
	// Confirmed and Suggested are this member's TOTAL match state, not one
	// import's delta.
	Confirmed int
	Suggested int
	// ReachVisibleToTeam is whether colleagues who may read a company see how
	// many of this member's connections work there. True for a member who has
	// never answered, including one with no row here at all.
	ReachVisibleToTeam bool
}

// MyLinkedInMatchTotals counts where this member's whole network stands.
//
// The import reports what THAT pass decided, which is the honest number for a
// progress line and the wrong one for a status card: the matcher only
// considers ghosts nobody has decided on, so a second import of the same file
// truthfully reports zero new matches while twenty-six matches sit in the
// database. A card labelled "Matched to a contact" showing 0 in that state is
// simply wrong.
// profileURLRequestField is the request field a LinkedIn profile URL arrives in.
const profileURLRequestField = "profile_url"

func (s *Store) MyLinkedInMatchTotals(ctx context.Context) (confirmed, suggested int, err error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return 0, 0, apperrors.ErrPermissionDenied
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE match_status = 'confirmed'),
			       count(*) FILTER (WHERE match_status = 'suggested')
			  FROM linkedin_connection
			 WHERE owner_user_id = $1 AND tombstoned_at IS NULL`, actor.UserID).
			Scan(&confirmed, &suggested)
	})
	return confirmed, suggested, err
}

// GetMyLinkedInAccount reads the caller's own row. A member who has never been
// through the act has no row, and that is not an error — it is the honest
// "not connected" the tab renders.
func (s *Store) GetMyLinkedInAccount(ctx context.Context) (LinkedInAccount, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return LinkedInAccount{}, apperrors.ErrPermissionDenied
	}
	var out LinkedInAccount
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT a.profile_url, a.connected_at, a.reach_visible_to_team,
			       (SELECT count(*) FROM linkedin_connection c
			         WHERE c.owner_user_id = $1 AND c.tombstoned_at IS NULL)
			  FROM linkedin_account a
			 WHERE a.user_id = $1`, actor.UserID).
			Scan(&out.ProfileURL, &out.ConnectedAt, &out.ReachVisibleToTeam, &out.Connections)
		if errors.Is(err, pgx.ErrNoRows) {
			// No row yet. The connection count is still worth reading: a
			// member may have uploaded an export from the settings tab without
			// ever going through the onboarding act.
			//
			// Visible, because no row is nobody's answer and the default is to
			// be listed. Left at Go's zero this reads as withheld, hiding a
			// member who never opened the setting.
			out.ReachVisibleToTeam = true
			return tx.QueryRow(ctx, `
				SELECT count(*) FROM linkedin_connection
				 WHERE owner_user_id = $1 AND tombstoned_at IS NULL`, actor.UserID).
				Scan(&out.Connections)
		}
		return err
	})
	if err != nil {
		return LinkedInAccount{}, fmt.Errorf("contacts: reading the caller's LinkedIn account: %w", err)
	}
	return out, nil
}

// SaveMyLinkedInAccountInput is the member's own answer about themselves.
type SaveMyLinkedInAccountInput struct {
	// ProfileURL empty CLEARS the stored URL — a member correcting a typo by
	// emptying the field means "I do not want this recorded", not "leave it".
	//
	// Nil is different: absent leaves the URL as it stands. That matters now
	// that a save can carry only the reach setting. Taken as a plain string,
	// a save of one field cleared the other.
	ProfileURL *string
	// Connected records the authorization. False leaves an existing
	// connected_at alone rather than revoking it: this endpoint edits a
	// profile, and disconnecting is its own deliberate act.
	Connected bool
	// ReachVisibleToTeam is the member's answer, or nil to leave it as it
	// stands. A pointer because false is an answer and omission is not. A save
	// that only corrected a profile URL must not switch the listing off.
	ReachVisibleToTeam *bool
}

// SaveMyLinkedInAccount upserts the caller's own row.
func (s *Store) SaveMyLinkedInAccount(ctx context.Context, in SaveMyLinkedInAccountInput) (LinkedInAccount, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return LinkedInAccount{}, apperrors.ErrPermissionDenied
	}
	// Validated only when the save carries one. A save that says nothing about
	// the URL is not a claim about its shape.
	var profileURL string
	if in.ProfileURL != nil {
		normalized, err := normalizeMyLinkedInProfileURL(strings.TrimSpace(*in.ProfileURL))
		if err != nil {
			return LinkedInAccount{}, err
		}
		profileURL = normalized
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := readLinkedInAccountTx(ctx, tx, actor.UserID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO linkedin_account (user_id, profile_url, connected_at, reach_visible_to_team)
			VALUES ($1, NULLIF($2, ''), CASE WHEN $3 THEN now() END, coalesce($4, true))
			ON CONFLICT (user_id) DO UPDATE SET
			    profile_url = CASE
			      WHEN $5 THEN NULLIF($2, '')
			      ELSE linkedin_account.profile_url END,
			    connected_at = CASE
			      WHEN $3 THEN coalesce(linkedin_account.connected_at, now())
			      ELSE linkedin_account.connected_at END,
			    reach_visible_to_team = coalesce($4, linkedin_account.reach_visible_to_team),
			    updated_at = now()`, actor.UserID, profileURL, in.Connected, in.ReachVisibleToTeam, in.ProfileURL != nil)
		if err != nil {
			return err
		}
		after, err := readLinkedInAccountTx(ctx, tx, actor.UserID)
		if err != nil {
			return err
		}
		// The write shape, in one transaction with the row. Consent to read a
		// professional network is exactly the kind of fact that must be
		// auditable — a member has to be able to ask when they agreed and what
		// changed, and an unaudited consent write cannot answer either.
		auditID, err := storekit.Audit(ctx, tx, "update", "user", actor.UserID,
			auditLinkedInAccount(before), auditLinkedInAccount(after))
		if err != nil {
			return err
		}
		// The URL itself stays OUT of the payload: it is the member's own
		// identifier, and a subscriber needs to know the authorization moved,
		// not what their LinkedIn address is.
		return storekit.EmitEvent(ctx, tx, auditID, actor.UserID,
			crmcontracts.PublicEventLinkedinAccountChanged{
				Connected:     after.ConnectedAt != nil,
				HasProfileUrl: after.ProfileURL != nil,
			})
	})
	if err != nil {
		return LinkedInAccount{}, fmt.Errorf("contacts: saving the caller's LinkedIn account: %w", err)
	}
	return s.GetMyLinkedInAccount(ctx)
}

// maxLinkedInProfileURLLength bounds the stored address: a profile URL is a
// host and one name, and a longer value is a paste of something else.
const maxLinkedInProfileURLLength = 300

// linkedInProfileURLInvalid is the field code of a value that is not a
// LinkedIn profile; the client words it in the reader's language.
const linkedInProfileURLInvalid = "linkedin_profile_url_invalid"

// normalizeMyLinkedInProfileURL takes the empty clear or a LinkedIn profile in
// the contact slot's spelling, since the address is drawn as the member's own.
func normalizeMyLinkedInProfileURL(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if len(s) > maxLinkedInProfileURLLength {
		return "", &values.ParseError{
			Field: profileURLRequestField, Code: "too_long",
			Message: "the profile URL is too long", Max: maxLinkedInProfileURLLength,
		}
	}
	normalized, err := NormalizeContactLinkedInURL(s)
	if err != nil {
		return "", &values.ParseError{
			Field: profileURLRequestField, Code: linkedInProfileURLInvalid,
			Message: "Enter a LinkedIn profile address such as https://www.linkedin.com/in/your-name.",
		}
	}
	return normalized, nil
}

// readLinkedInAccountTx reads one member's row inside an open transaction, for
// the before/after images the audit row carries.
func readLinkedInAccountTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (LinkedInAccount, error) {
	var out LinkedInAccount
	err := tx.QueryRow(ctx,
		`SELECT profile_url, connected_at FROM linkedin_account WHERE user_id = $1`, user).
		Scan(&out.ProfileURL, &out.ConnectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LinkedInAccount{}, nil
	}
	return out, err
}

// auditLinkedInAccount is the field image the audit row stores. The profile URL
// is recorded here and not in the event: the audit log is the member's own
// history of their own account, while the event fans out to subscribers.
func auditLinkedInAccount(a LinkedInAccount) map[string]any {
	return map[string]any{
		profileURLRequestField: a.ProfileURL,
		"connected_at":         a.ConnectedAt,
		// Withholding a reach is a privacy choice, so the images have to show
		// it. Left out, a save that only switched it writes an audit row whose
		// before and after read alike, and the record says nothing happened.
		"reach_visible_to_team": a.ReachVisibleToTeam,
	}
}
