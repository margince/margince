// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The rest of what a privacy page owes its reader beside the source and the
// rights. That is who is responsible, what is held and why, who sees it, and
// how long it stays. Each part is read
// from what the installation holds about this reader, never from the catalog.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// NoticeSubject is the contact or website lead a notice link names.
type NoticeSubject struct {
	ContactID ids.ContactID
	LeadID    ids.LeadID
}

// NoticeUse is one use of the reader's data on record, with its recorded basis.
type NoticeUse struct {
	PurposeKey string
	Basis      string
	// Since is when a consent use was granted; nil for any other basis.
	Since *time.Time
}

// LawFloor is a statutory period that keeps some data past a rule's window.
type LawFloor struct {
	Name        string
	Keep        string
	FromYearEnd bool
}

// RetentionRule is one retention rule that acts on the reader's data.
type RetentionRule struct {
	Scope      string
	RetainDays int
	Action     string
	KeptByLaw  *LawFloor
}

// NoticeHeld is what the privacy module reads about one subject for the page.
type NoticeHeld struct {
	Categories []string
	Uses       []NoticeUse
	Retention  []RetentionRule
}

// NoticeHeldReader reads the held kinds, the uses on record and the retention
// that reaches one subject. Compose supplies it from the privacy module, which
// owns the subject-access export these come from.
type NoticeHeldReader interface {
	NoticeHeld(ctx context.Context, subject NoticeSubject) (NoticeHeld, error)
}

// WithNoticeHeldReader wires what the privacy page reads the subject's data from.
func (s *Store) WithNoticeHeldReader(r NoticeHeldReader) *Store {
	s.held = r
	return s
}

// WithNoticeHeldReader wires what the privacy page reads the subject's data from.
func (h Handlers) WithNoticeHeldReader(r NoticeHeldReader) Handlers {
	h.store = h.store.WithNoticeHeldReader(r)
	return h
}

// disclosureRecipients is who sees the reader's data: the controller's own
// staff, and the providers who host the CRM and deliver its mail for it.
// Both are true of every installation, so the list is fixed like the rights.
func disclosureRecipients() []string {
	return []string{"colleagues", "service_providers"}
}

// readControllerTx is who answers for the record's data: the installation's
// particulars, with the legal name and address of the further entity that
// answers for the record (recordEntityTx) in place of its own.
func readControllerTx(ctx context.Context, tx pgx.Tx, out *PrivacyInformation, who ruleSubject) error {
	identity, err := config.StoredApplyTx(ctx, tx, config.Bound(), ControllerIdentity)
	if err != nil {
		return fmt.Errorf("consent: reading who answers for this data: %w", err)
	}
	entity, err := recordEntityTx(ctx, tx, who)
	if err != nil {
		return err
	}
	facts, err := sendingEntityFactsTx(ctx, tx, entity)
	if err != nil {
		return err
	}
	out.Controller = controllerOf(identity, facts)
	return nil
}

// controllerParticulars reads the particulars a confirm page names for the
// record its link is about, in a transaction of its own.
func (s *Store) controllerParticulars(ctx context.Context, ref ConfirmRef) (ControllerParticulars, error) {
	var out PrivacyInformation
	err := s.db.Tx(ctx, func(tx pgx.Tx) error { return readControllerTx(ctx, tx, &out, confirmRuleSubject(ref)) })
	return out.Controller, err
}

// confirmRuleSubject is the record a confirm link is about: the lead, else the
// contact.
func confirmRuleSubject(ref ConfirmRef) ruleSubject {
	if !ref.LeadID.IsZero() {
		return ruleSubject{Kind: entityLead, ID: ref.LeadID.String()}
	}
	return ruleSubject{Kind: entityContact, ID: ref.ContactID.String()}
}

// readHeld fills the parts read about this subject. Unwired, the page names no
// categories and no retention rather than something it cannot back, and still
// names the purpose settleUsesTx gives every reader.
func (s *Store) readHeld(ctx context.Context, subject NoticeSubject, out *PrivacyInformation) error {
	var held NoticeHeld
	if s.held != nil {
		read, err := s.held.NoticeHeld(ctx, subject)
		if err != nil {
			return fmt.Errorf("consent: reading what is held about this reader: %w", err)
		}
		held = read
	}
	out.DataCategories = append(out.DataCategories, held.Categories...)
	out.Retention = append(out.Retention, held.Retention...)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		sender, err := s.recordCountryTx(ctx, tx, noticeRuleSubject(subject))
		if err != nil {
			return err
		}
		uses, err := settleUsesTx(ctx, tx, subject, held.Uses, sender)
		if err != nil {
			return err
		}
		return namePurposesTx(ctx, tx, uses, out)
	})
}

// settleUsesTx is the uses the page lists, judged as the send engine judges
// them, under the recipient's pack and the sender's. Under a pack without
// legitimate interests, a use is shown only on a basis that pack accepts
// (keptUnderPack). A reader with no use left is shown
// business correspondence, so the page always names a purpose
// (GDPR Art. 14(1)(c)). Its basis is defaultUseBasis.
func settleUsesTx(ctx context.Context, tx pgx.Tx, subject NoticeSubject, uses []NoticeUse, sender jurisdiction.Code) ([]NoticeUse, error) {
	rule := noticeRuleSubject(subject)
	who := subjectRef{Kind: rule.Kind, ID: rule.ID}
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: who.Kind, ID: who.ID})
	if err != nil {
		return nil, err
	}
	country.Sender = sender
	rules, _, found := applicableRules(country.dataProtectionCodes()...)
	noInterests := found && rules.NoLegitimateInterests
	kept := make([]NoticeUse, 0, len(uses))
	for _, use := range uses {
		if noInterests && !keptUnderPack(commsauthz.Basis(use.Basis)) {
			continue
		}
		kept = append(kept, use)
	}
	if len(kept) > 0 {
		return kept, nil
	}
	var onRecord commsauthz.Basis
	if noInterests {
		consented, err := consentCoversTx(ctx, tx, who, commsauthz.CategoryReplyToInbound)
		if err != nil {
			return nil, err
		}
		if consented {
			onRecord = commsauthz.BasisConsent
		}
	}
	return []NoticeUse{{PurposeKey: PurposeBusinessCorrespondence, Basis: defaultUseBasis(noInterests, onRecord)}}, nil
}

// noBasisRecorded is the basis code the page shows when nothing lawful is on
// record. The page says so plainly instead of naming an agreement that does
// not exist.
const noBasisRecorded = "none"

// defaultUseBasis is the basis of the purpose a reader with no use on record
// is shown. Outside a pack without legitimate interests it is legitimate
// interests. Under one it is the consent or agreement on record, else none.
func defaultUseBasis(noInterests bool, onRecord commsauthz.Basis) string {
	switch {
	case !noInterests:
		return string(commsauthz.BasisLegitimateInterests)
	case onRecord == "":
		return noBasisRecorded
	}
	return string(onRecord)
}

// namePurposesTx names each use by its purpose's published label. The key
// stays beside it, so a page can name a seeded purpose in its reader's language.
func namePurposesTx(ctx context.Context, tx pgx.Tx, uses []NoticeUse, out *PrivacyInformation) error {
	for _, use := range uses {
		label := use.PurposeKey
		err := tx.QueryRow(ctx, `SELECT label FROM consent_purpose WHERE key = $1`, use.PurposeKey).Scan(&label)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("consent: naming what this reader's data is used for: %w", err)
		}
		out.Purposes = append(out.Purposes, label)
		out.PurposeKeys = append(out.PurposeKeys, use.PurposeKey)
		out.PurposeBases = append(out.PurposeBases, use.Basis)
		out.PurposeSince = append(out.PurposeSince, use.Since)
	}
	return nil
}

// noticeRuleSubject is the record a notice page is about: the contact, else
// the lead.
func noticeRuleSubject(subject NoticeSubject) ruleSubject {
	if subject.ContactID.IsZero() {
		return ruleSubject{Kind: entityLead, ID: subject.LeadID.String()}
	}
	return ruleSubject{Kind: entityContact, ID: subject.ContactID.String()}
}
