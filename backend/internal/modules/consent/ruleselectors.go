// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which country's rules bind one send, chosen by two separate selectors.
//
// Marketing rules (consent or an exception, the advertising ceiling, the
// advertising label, the objection route) follow the recipient's country. Data
// protection (who the controller is, who answers about the data, request
// deadlines) follows the country of the sending legal entity. Each recipient of
// a message is put to the marketing selector on their own.
//
// A recipient's country comes from the record and nothing else. An email
// domain, the sender's country or the most permissive pack never stands in
// for it. A recipient with no usable country, or one whose country no pack
// covers, gets the baseline: no pack obligations. Marketing still needs
// consent, which the verdict asks everywhere. The send path adds the
// unsubscribe surface whatever the country.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// CountrySource names the record a recipient's country was read from.
type CountrySource string

// The sources, in the order the selector asks them.
const (
	// CountryFromContactAddress is the contact's own postal address.
	CountryFromContactAddress CountrySource = "contact_address"
	// CountryFromEmployerAddress is the address of the contact's current
	// primary employer.
	CountryFromEmployerAddress CountrySource = "employer_address"
	// CountryFromLeadCompany is the address of the company a lead is filed
	// under.
	CountryFromLeadCompany CountrySource = "lead_company_address"
)

// RecipientCountry is the marketing selector's answer for one recipient. The
// zero value means the record states no usable country.
type RecipientCountry struct {
	Code   jurisdiction.Code
	Source CountrySource
}

// Known reports whether the record stated a country.
func (c RecipientCountry) Known() bool { return c.Code != "" }

// ruleSubject is the record a recipient resolved to. The zero value is a
// recipient no single record answers for.
type ruleSubject struct {
	Kind string
	ID   string
}

// applicableRules is the one door to a jurisdiction's messaging rules. It takes
// codes the selectors below already chose, and answers ok=false for the
// baseline: no code, or codes no compiled-in pack declares.
func applicableRules(codes ...jurisdiction.Code) (messagingrules.Rules, messagingrules.Applied, bool) {
	named := make([]jurisdiction.Code, 0, len(codes))
	for _, code := range codes {
		if code != "" {
			named = append(named, code)
		}
	}
	if len(named) == 0 {
		return messagingrules.Rules{}, messagingrules.Applied{}, false
	}
	return messagingrules.Strictest(named...)
}

// subjectOfRecipient names the contact behind a recipient, else the lead. An
// address two contacts share answers no subject: the country of either one
// would be a guess about the other.
func subjectOfRecipient(ctx context.Context, tx pgx.Tx, r connector.Recipient) (ruleSubject, error) {
	contactID, found, err := resolveContact(ctx, tx, r)
	if err != nil {
		if errors.Is(err, apperrors.ErrConsentNotGranted) {
			return ruleSubject{}, nil
		}
		return ruleSubject{}, err
	}
	if found {
		return ruleSubject{Kind: entityContact, ID: contactID}, nil
	}
	leadID, found, err := resolveLead(ctx, tx, r)
	if err != nil || !found {
		return ruleSubject{}, err
	}
	return ruleSubject{Kind: entityLead, ID: leadID}, nil
}

// subjectOfDecision names the record a decision was taken about, the same one
// subjectOfRecipient resolves for its recipient.
func subjectOfDecision(d commsauthz.Decision) ruleSubject {
	if d.SubjectID.IsZero() {
		return ruleSubject{}
	}
	return ruleSubject{Kind: d.SubjectKind, ID: d.SubjectID.String()}
}

// recipientCountryOf is the marketing selector for an address.
func recipientCountryOf(ctx context.Context, tx pgx.Tx, r connector.Recipient) (RecipientCountry, error) {
	who, err := subjectOfRecipient(ctx, tx, r)
	if err != nil {
		return RecipientCountry{}, err
	}
	return recipientCountryTx(ctx, tx, who)
}

// recipientCountryTx is the marketing selector: the first usable country the
// subject's own record states. A lead and a contact take the same route, each
// through the record it has.
func recipientCountryTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (RecipientCountry, error) {
	var candidates []RecipientCountry
	var err error
	switch who.Kind {
	case entityContact:
		candidates, err = contactCountryCandidates(ctx, tx, who.ID)
	case entityLead:
		candidates, err = leadCountryCandidates(ctx, tx, who.ID)
	default:
		return RecipientCountry{}, nil
	}
	if err != nil {
		return RecipientCountry{}, err
	}
	return firstUsableCountry(candidates), nil
}

// firstUsableCountry takes the first candidate whose value is a country code.
// A free-text country an older import stored ("Germany") is skipped, not
// mapped. Reading a name as a code is a guess this selector does not make.
func firstUsableCountry(candidates []RecipientCountry) RecipientCountry {
	for _, c := range candidates {
		code := jurisdiction.Code(strings.ToLower(strings.TrimSpace(string(c.Code))))
		if code.Validate() == nil {
			return RecipientCountry{Code: code, Source: c.Source}
		}
	}
	return RecipientCountry{}
}

func contactCountryCandidates(ctx context.Context, tx pgx.Tx, contactID string) ([]RecipientCountry, error) {
	var own, employer *string
	// uq_rel_current_primary_employer allows one current primary employer per
	// contact, so the subquery answers at most one row.
	err := tx.QueryRow(ctx, `
		SELECT c.address_country,
		       (SELECT co.address_country
		          FROM relationship r
		          JOIN company co ON co.id = r.company_id AND co.archived_at IS NULL
		         WHERE r.contact_id = c.id AND r.kind = 'employment' AND r.archived_at IS NULL
		           AND `+employment.CurrentPrimarySQL("r")+`)
		  FROM contact c
		 WHERE c.id = $1`, contactID).Scan(&own, &employer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consent: reading the recipient's country: %w", err)
	}
	return countryCandidates(
		RecipientCountry{Code: codeOf(own), Source: CountryFromContactAddress},
		RecipientCountry{Code: codeOf(employer), Source: CountryFromEmployerAddress},
	), nil
}

func leadCountryCandidates(ctx context.Context, tx pgx.Tx, leadID string) ([]RecipientCountry, error) {
	var company *string
	err := tx.QueryRow(ctx, `
		SELECT co.address_country
		  FROM lead l
		  JOIN company co ON co.id = l.company_id AND co.archived_at IS NULL
		 WHERE l.id = $1`, leadID).Scan(&company)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consent: reading the recipient's country: %w", err)
	}
	return countryCandidates(RecipientCountry{Code: codeOf(company), Source: CountryFromLeadCompany}), nil
}

func countryCandidates(all ...RecipientCountry) []RecipientCountry {
	out := make([]RecipientCountry, 0, len(all))
	for _, c := range all {
		if c.Code != "" {
			out = append(out, c)
		}
	}
	return out
}

func codeOf(column *string) jurisdiction.Code {
	if column == nil {
		return ""
	}
	return jurisdiction.Code(*column)
}

// dataProtectionCountryTx is the data-protection selector: the country of the
// legal entity the sender belongs to. A sender assigned to no entity, or to
// the default one, sends for the installation's default entity, so an
// installation with one entity configures nothing.
//
// The fallback reads installation.country, which is MachineryApplied: this runs
// under whatever principal is sending, and a rep or a worker holds no
// settings-read grant (backend/gates/machineryapplied_test.go).
func (s *Store) dataProtectionCountryTx(ctx context.Context, tx pgx.Tx, sender ids.UUID) (jurisdiction.Code, error) {
	if !sender.IsZero() {
		var isDefault bool
		var country *string
		err := tx.QueryRow(ctx, `
			SELECT le.is_default, le.country
			  FROM legal_entity_member m
			  JOIN legal_entity le ON le.id = m.legal_entity_id AND le.archived_at IS NULL
			 WHERE m.user_id = $1`, sender).Scan(&isDefault, &country)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return "", fmt.Errorf("consent: reading the sending legal entity: %w", err)
		case !isDefault && country != nil:
			return jurisdiction.Code(*country), nil
		}
	}
	return s.defaultEntityCountry(ctx, tx)
}

// senderOf is the user a message goes out for: the human, or the human an
// agent works for. Automatic work names nobody and sends for the default
// entity.
func senderOf(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		return ids.Nil
	}
	if actor.Type == principal.PrincipalHuman {
		return actor.UserID
	}
	return actor.OnBehalfOf
}
