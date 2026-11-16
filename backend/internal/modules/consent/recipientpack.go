// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The packs whose data-protection law follows the recipient.
//
// ruleselectors.go picks data protection by the sending entity's country. A
// pack that sets DataProtectionFollowsRecipient reaches its recipients whatever
// the controller's country is. For them the disclosures, the privacy page, the
// mail language and the request deadlines come from their own pack. Their
// country is the one recipientCountryTx reads off the record, as a stand-in
// for citizenship. A citizen abroad with a foreign address is not detected.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// recipientDataProtection answers the rules of a recipient's country when
// that country's data-protection law follows the recipient.
func recipientDataProtection(code jurisdiction.Code) (messagingrules.Rules, bool) {
	rules, _, found := applicableRules(code)
	if !found || !rules.DataProtectionFollowsRecipient {
		return messagingrules.Rules{}, false
	}
	return rules, true
}

// subjectDataProtectionTx answers the country whose data-protection rules
// bind this subject because they follow the recipient, or empty when the
// sender's selector decides.
func subjectDataProtectionTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (jurisdiction.Code, error) {
	country, err := recipientCountryTx(ctx, tx, who)
	if err != nil {
		return "", err
	}
	if _, follows := recipientDataProtection(country.Code); !follows {
		return "", nil
	}
	return country.Code, nil
}

// subjectPackLanguageTx is the language the subject's pack writes its
// disclosures in. It is empty when no such pack binds them, or when this build
// has no copy in that language.
func subjectPackLanguageTx(ctx context.Context, tx pgx.Tx, who ruleSubject) (string, error) {
	code, err := subjectDataProtectionTx(ctx, tx, who)
	if err != nil || code == "" {
		return "", err
	}
	rules, _, _ := applicableRules(code)
	if !mailcopy.Known(rules.DisclosureLanguage) {
		return "", nil
	}
	return rules.DisclosureLanguage, nil
}

// subjectSendLanguage is the language one controller mail to this subject goes
// out in. The subject's pack language comes first, because the rights text is
// the pack's and not a choice of the sender. Then the sender's pick, then the
// installation's language.
func (s *Store) subjectSendLanguage(ctx context.Context, tx pgx.Tx, who ruleSubject, picked string) (string, error) {
	language, err := subjectPackLanguageTx(ctx, tx, who)
	if err != nil || language != "" {
		return language, err
	}
	return s.sendLanguage(ctx, tx, picked), nil
}

// dataProtectionStampFor is the country one recipient's decision records
// beside its marketing ruleset. That is the recipient's own country when its
// pack follows them, else the sending entity's stamp.
func dataProtectionStampFor(recipient RecipientCountry, entityStamp *string) *string {
	if _, follows := recipientDataProtection(recipient.Code); follows {
		code := string(recipient.Code)
		return &code
	}
	return entityStamp
}

// interestBasisTx is the ground a message rests on where the GDPR model reads
// legitimate interests. A recipient whose pack has no such basis gets the
// consent or the agreement on record instead, or no basis when neither is.
func interestBasisTx(ctx context.Context, tx pgx.Tx, subject subjectRef) (commsauthz.Basis, error) {
	rules, _, found := applicableRules(subject.Country.Code)
	if !found || !rules.NoLegitimateInterests {
		return commsauthz.BasisLegitimateInterests, nil
	}
	return lawfulUseOnRecordTx(ctx, tx, subject)
}

// lawfulUseOnRecordTx answers the basis on record for using this subject's
// data under a pack with no legitimate-interest basis. Consent comes first.
// Then an agreement with the subject (Law 91/2025 Art. 19.1(d)): a live
// contract basis, or the contact working for a customer. A privacy notice the
// subject was sent is neither. Empty means nothing on record.
func lawfulUseOnRecordTx(ctx context.Context, tx pgx.Tx, subject subjectRef) (commsauthz.Basis, error) {
	column := subjectColumnContact
	if subject.Kind == entityLead {
		column = subjectColumnLead
	}
	var consented, agreed bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact_consent WHERE `+column+` = $1::uuid AND state = 'granted'),
		       EXISTS (SELECT 1 FROM communication_basis
		                WHERE `+column+` = $1::uuid AND kind = $2 AND revoked_at IS NULL
		                  AND (valid_until IS NULL OR valid_until > now()))`,
		subject.ID, string(commsauthz.BasisContract)).Scan(&consented, &agreed)
	if err != nil {
		return "", wrapEvidenceRead("the recipient's consent and agreements", err)
	}
	if consented {
		return commsauthz.BasisConsent, nil
	}
	if !agreed && subject.Kind == entityContact {
		if agreed, err = isCustomerTx(ctx, tx, subject.ID); err != nil {
			return "", err
		}
	}
	if agreed {
		return commsauthz.BasisContract, nil
	}
	return "", nil
}
