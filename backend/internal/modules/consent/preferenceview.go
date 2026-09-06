// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the public preference page is allowed to know.
//
// The page shows the recipient THEIR OWN DECISION and nothing about how
// the product would treat them. That line matters: a verdict's reason
// names relationship and timeline facts ("they wrote to you on the 3rd"),
// and this surface sits behind a link that lives in a mailbox for thirty
// days. So the wire carries a choice and a permission, never a verdict.
//
// It also decides the question the raw consent state cannot answer.
// person_consent.state is 'unknown' both for a marketing lane nobody has
// opted into and for business correspondence nobody has objected to —
// the first is off, the second is on, and a page that rendered the raw
// value called a live lane "not subscribed". The class decides which.

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entityPerson is the subject table the preference centre speaks for. The
// public surfaces only ever answer for a person: a lead has no mailbox
// this link could have reached.
const entityPerson = "person"

// Choice is what the RECIPIENT decided, which is what the checkbox edits.
type Choice string

const (
	// ChoiceOptedIn means they asked for this.
	ChoiceOptedIn Choice = "opted_in"
	// ChoiceOptedOut means they asked us to stop. It overrides every basis.
	ChoiceOptedOut Choice = "opted_out"
	// ChoiceNoObjection means they have neither asked for it nor objected.
	// The honest name for a lane that does not run on consent — direct
	// business correspondence — where silence is not refusal. Calling it
	// "subscribed" would misstate the lawful basis to the person it
	// belongs to.
	ChoiceNoObjection Choice = "no_objection"
)

// PreferenceView is the whole public read.
type PreferenceView struct {
	// MaskedEmail is the address the link was delivered to, masked — the token
	// records which one it was minted for, so a person holding several sees the
	// mailbox that actually received the message.
	//
	// A token minted before that was recorded, or one whose address the subject
	// has since erased, falls back to the person's primary. That is the older
	// behaviour and the reason the page's copy still presents this as account
	// context rather than claiming "this address".
	MaskedEmail string
	// WorkspaceName can be empty on an unnamed installation; every string
	// that interpolates it needs an omission variant.
	WorkspaceName string
	Purposes      []PurposeChoice
}

// PublicPreferenceView reads everything the page needs in one transaction.
//
// Takes the token's whole resolution rather than the person id: which address
// the link went to is part of what the page reports, and a signature carrying
// only the person is one a caller cannot answer that from.
func (s *Store) PublicPreferenceView(ctx context.Context, ref PreferenceRef) (PreferenceView, error) {
	personID := ref.PersonID
	if err := auth.Require(ctx, entityPerson, principal.ActionRead); err != nil {
		return PreferenceView{}, err
	}
	var view PreferenceView
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, entityPerson, personID.UUID); err != nil {
			return err
		}
		// Asked once for the whole view: an archived subject cannot take a
		// grant on ANY purpose, and a page that offered the switch anyway
		// would invite a choice the save is bound to refuse.
		grantable, err := subjectTakesAGrantTx(ctx, tx, subject{
			entityType: entityPerson, id: personID.UUID, column: personIDKey,
		})
		if err != nil {
			return err
		}
		purposes, err := purposeStatesTx(ctx, tx, personID, grantable)
		if err != nil {
			return err
		}
		email, err := deliveredEmailTx(ctx, tx, personID, ref.EmailID)
		if err != nil {
			return err
		}
		view = PreferenceView{MaskedEmail: MaskEmail(email), Purposes: purposes}
		return nil
	})
	if err != nil {
		return PreferenceView{}, err
	}
	view.WorkspaceName = s.workspaceName(ctx)
	return view, nil
}

// primaryEmailSQL selects a person's primary live address, correlated on
// the column named by ref.
//
// A fragment rather than a query, because its two callers need it in
// different shapes: the preference centre reads the address alone, while
// the confirm card reads it as one column of the card's single
// round-trip, and splitting that into a second query to share a helper
// would cost a round-trip to buy nothing. Sharing the TEXT is what keeps
// the ordering — is_primary first, then oldest — spelled once. Drop the
// tiebreak in one copy and two surfaces show the same person two
// different addresses, each looking right on its own screen.
//
// Held by: TestThePreferenceCentreResolvesOnePrimaryAddress (backend/gates/preferencecentrewriters_test.go)
func primaryEmailSQL(ref string) string {
	return `coalesce((SELECT pe.email FROM person_email pe
	                    WHERE pe.person_id = ` + ref + ` AND pe.archived_at IS NULL
	                    ORDER BY pe.is_primary DESC, pe.created_at LIMIT 1), '')`
}

// deliveredEmailTx reads the address the link was sent to, falling back to the
// person's primary when the token does not name one.
//
// The named address is still checked against the person and against
// archived_at: the token is a bearer credential thirty days long, and a row
// that has since moved to another person or been erased must not be read back
// out of it. A named address that no longer qualifies falls back the same way a
// missing one does, which keeps the page working rather than failing a reader
// who did nothing wrong.
func deliveredEmailTx(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID, emailID *ids.UUID,
) (string, error) {
	if emailID != nil {
		var email string
		err := tx.QueryRow(ctx, `
			SELECT email FROM person_email
			 WHERE id = $1 AND person_id = $2 AND archived_at IS NULL`,
			*emailID, personID).Scan(&email)
		if err == nil {
			return email, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}
	return primaryEmailTx(ctx, tx, personID)
}

// primaryEmailTx reads the address on its own, for the preference centre.
func primaryEmailTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (string, error) {
	var email string
	err := tx.QueryRow(ctx, `SELECT `+primaryEmailSQL("$1"), personID).Scan(&email)
	return email, err
}

// purposeStatesTx reads the catalog with the recipient's decision on each
// purpose.
//
// phone_outreach is excluded: the verdict blocks it unconditionally
// because no call path is configured, so offering its switch would offer
// a control over something that cannot happen.
func purposeStatesTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID, grantable bool) ([]PurposeChoice, error) {
	rows, err := tx.Query(ctx, `
		SELECT cp.key, cp.label, coalesce(pc.state, 'unknown'),
		       cp.requires_double_opt_in, cp.class
		  FROM consent_purpose cp
		  LEFT JOIN person_consent pc ON pc.purpose_id = cp.id AND pc.person_id = $1
		 WHERE cp.archived_at IS NULL AND cp.class <> $2
		 ORDER BY cp.key`, personID, ClassPhoneOutreach)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PurposeChoice
	for rows.Next() {
		var c PurposeChoice
		var state, class string
		if err := rows.Scan(&c.Key, &c.Label, &state, &c.GrantNeedsConfirmation, &class); err != nil {
			return nil, err
		}
		c.State = state
		c.Locked = LockedPurpose(c.Key)
		c.Choice = choiceOf(state, Class(class))
		// A locked purpose is not editable at all; otherwise a grant is
		// offered only where the engine would accept one — without a
		// confirmation round-trip this token cannot evidence, and against a
		// subject still live enough to claim a lawful basis.
		c.CanOptIn = !c.Locked && !c.GrantNeedsConfirmation && grantable
		out = append(out, c)
	}
	return out, rows.Err()
}

// choiceOf reads the recipient's decision out of the stored state, which
// needs the class to be read correctly.
//
// 'unknown' means opposite things either side of the consent line: on a
// marketing lane nobody has opted in, so it is off; on a lane that does
// not run on consent nobody has objected, so it is on. Transactional is
// always on and cannot be changed.
func choiceOf(state string, class Class) Choice {
	switch ConsentState(state) {
	case StateWithdrawn:
		return ChoiceOptedOut
	case StateGranted:
		return ChoiceOptedIn
	}
	switch class {
	case ClassTransactional, ClassBusinessCorrespondence:
		return ChoiceNoObjection
	default:
		return ChoiceOptedOut
	}
}

// maskedLocalRunes is how many bullets stand in for the local part —
// FIXED, so the mask does not leak how long the address is.
const maskedLocalRunes = 5

// MaskEmail shows the first character and the domain: enough for the
// holder to recognise their own mailbox, not enough to hand the address
// to somebody who found the link.
func MaskEmail(addr string) string {
	addr = strings.TrimSpace(addr)
	at := strings.LastIndex(addr, "@")
	if at <= 0 || at == len(addr)-1 {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(addr[:at])
	if first == utf8.RuneError {
		return ""
	}
	return string(first) + strings.Repeat("•", maskedLocalRunes) + addr[at:]
}
