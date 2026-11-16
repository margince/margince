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
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// keptUnderPack reports whether a basis is an Art. 19.1 case on its own, so a
// pack without legitimate interests keeps it: a duty the law imposes, or the
// protection of life or safety. Every other basis needs the subject's consent.
func keptUnderPack(basis commsauthz.Basis) bool {
	return basis == commsauthz.BasisLegalObligation || basis == commsauthz.BasisVitalOrSecurityInterest ||
		basis == commsauthz.BasisConsent
}

// settlePackBasisTx is the one place a supported resolution's basis meets a
// pack without legitimate interests. Preview, staging and transmit reach it
// through resolveAndRecord, and the privacy page through settleUsesTx. A basis
// the law does not name becomes the subject's consent to a purpose covering
// this use, or no basis at all.
func settlePackBasisTx(ctx context.Context, tx pgx.Tx, subject subjectRef, res resolution) (resolution, error) {
	rules, _, found := applicableRules(subject.Country.Code)
	if !found || !rules.NoLegitimateInterests || keptUnderPack(res.Basis) {
		return res, nil
	}
	consented, err := consentCoversTx(ctx, tx, subject, res.Category)
	if err != nil {
		return resolution{}, err
	}
	if consented {
		res.Basis = commsauthz.BasisConsent
		return res, nil
	}
	res.Basis, res.BasisMissing = "", true
	if subject.Kind == entityContact {
		res.ConsentPurpose, err = askablePurposeTx(ctx, tx, subject.ID, res.Category)
	}
	return res, err
}

// askablePurposeTx is the purpose a consent request to this contact would ask
// about for this use. It is the oldest live double-opt-in purpose whose class
// covers the use and that the contact has not withdrawn. Zero when none is.
func askablePurposeTx(ctx context.Context, tx pgx.Tx, contactID string, category commsauthz.Category) (ids.UUID, error) {
	classes := classesCovering(category)
	if len(classes) == 0 {
		return ids.UUID{}, nil
	}
	var purpose ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT cp.id FROM consent_purpose cp
		 WHERE cp.archived_at IS NULL AND cp.requires_double_opt_in AND cp.class = ANY($2)
		   AND NOT EXISTS (SELECT 1 FROM contact_consent cc
		                    WHERE cc.contact_id = $1::uuid AND cc.purpose_id = cp.id AND cc.state = 'withdrawn')
		 ORDER BY cp.created_at, cp.id LIMIT 1`, contactID, classes).Scan(&purpose)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, nil
	}
	if err != nil {
		return ids.UUID{}, wrapEvidenceRead("the purpose a consent request would ask about", err)
	}
	return purpose, nil
}

// consentCoversTx answers whether the subject has granted consent to a
// purpose whose class covers this category. A newsletter consent covers
// advertising and nothing else.
func consentCoversTx(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category) (bool, error) {
	classes := classesCovering(category)
	if len(classes) == 0 {
		return false, nil
	}
	column := subjectColumnContact
	if subject.Kind == entityLead {
		column = subjectColumnLead
	}
	var consented bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact_consent cc
		                 JOIN consent_purpose cp ON cp.id = cc.purpose_id AND cp.archived_at IS NULL
		                WHERE cc.`+column+` = $1::uuid AND cc.state = 'granted' AND cp.class = ANY($2))`,
		subject.ID, classes).Scan(&consented)
	if err != nil {
		return false, wrapEvidenceRead("the recipient's consent to this use", err)
	}
	return consented, nil
}
