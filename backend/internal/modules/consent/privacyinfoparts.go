// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The rest of what a privacy page owes its reader beside the source, the
// purposes and the rights: who is responsible, why each purpose is allowed,
// which kinds of data are held, who sees them, and how long they are kept.
// Each one is read from what the installation holds or has set, never typed
// in as a number or a name.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RetentionRule is one retention setting that acts today: its scope in the
// retention settings' own spelling, its window in days, and its action.
type RetentionRule struct {
	Scope      string
	RetainDays int
	Action     string
}

// RetentionReader lists the retention rules that act today: switched on and
// not held back by the retain-only setting. Compose supplies it, because the
// rules and that setting belong to the privacy module.
type RetentionReader interface {
	ActingRetention(ctx context.Context) ([]RetentionRule, error)
}

// WithRetentionReader wires what the privacy page reads its retention from.
func (s *Store) WithRetentionReader(r RetentionReader) *Store {
	s.retention = r
	return s
}

// WithRetentionReader wires what the privacy page reads its retention from.
func (h Handlers) WithRetentionReader(r RetentionReader) Handlers {
	h.store = h.store.WithRetentionReader(r)
	return h
}

// Retention scopes that hold data about a person. Every other scope (deals,
// report editions, daily risk records) holds the installation's own figures.
var (
	personRetentionScopes = []string{
		"activity", "activity/transcript", "raw_capture", "ai_call_payload/content",
	}
	contactRetentionScopes = append(slices.Clone(personRetentionScopes), "contact/no_consent_no_deal")
	leadRetentionScopes    = append(slices.Clone(personRetentionScopes), "lead/unconverted")
)

// disclosureRecipients is who sees a person's data: the controller's own
// people, and the providers who host the CRM and deliver its mail for it.
// Both are true of every installation, so the list is fixed like the rights.
func disclosureRecipients() []string {
	return []string{"colleagues", "service_providers"}
}

// disclosedBasis is why a purpose of this class may use somebody's data.
//
// Not basisForClass, which answers what lets one MESSAGE go: there business
// correspondence rests on the subject having written first. A privacy notice
// goes to somebody whose data came from elsewhere, so for them that purpose
// rests on the installation's weighed interest instead.
func disclosedBasis(class Class) string {
	switch class {
	case ClassBusinessCorrespondence:
		return "legitimate_interests"
	case ClassTransactional:
		return "contract"
	default:
		return "consent"
	}
}

// readControllerTx is who answers for the data: the installation's own
// particulars. The installation sends the notice, so its default legal entity
// is the sending entity whose rules the data follows.
func readControllerTx(ctx context.Context, tx pgx.Tx, out *PrivacyInformation) error {
	identity, err := config.StoredApplyTx(ctx, tx, config.Bound(), ControllerIdentity)
	if err != nil {
		return fmt.Errorf("consent: reading who answers for this data: %w", err)
	}
	out.Controller = identity
	return nil
}

// readContactCategoriesTx names the kinds of data a contact's record holds.
// Presence only: the page names a kind and never shows a value. Unlike the
// confirm card it also answers for an archived record, which is still held.
func readContactCategoriesTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *PrivacyInformation) error {
	var held [6]bool
	err := tx.QueryRow(ctx, `
		SELECT btrim(coalesce(p.full_name, '')) <> '',
		       btrim(coalesce(p.title, '')) <> '',
		       EXISTS (SELECT 1 FROM relationship r
		                WHERE r.contact_id = p.id AND r.kind = 'employment'
		                  AND `+employment.IsCurrentSQL("r.ended_at")+` AND r.archived_at IS NULL),
		       EXISTS (SELECT 1 FROM contact_email e WHERE e.contact_id = p.id AND e.archived_at IS NULL),
		       EXISTS (SELECT 1 FROM contact_phone f WHERE f.contact_id = p.id AND f.archived_at IS NULL),
		       EXISTS (SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
		                WHERE l.contact_id = p.id AND `+correspondenceSQL+`)
		  FROM contact p WHERE p.id = $1`, contactID).
		Scan(&held[0], &held[1], &held[2], &held[3], &held[4], &held[5])
	if err != nil {
		return fmt.Errorf("consent: reading which kinds of data this contact's record holds: %w", err)
	}
	out.DataCategories = heldCategories(held)
	return nil
}

// readLeadCategoriesTx is readContactCategoriesTx for a lead's own columns.
func readLeadCategoriesTx(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, out *PrivacyInformation) error {
	var held [6]bool
	err := tx.QueryRow(ctx, `
		SELECT btrim(coalesce(d.full_name, '')) <> '',
		       btrim(coalesce(d.title, '')) <> '',
		       btrim(coalesce(d.company_name, '')) <> '' OR d.company_id IS NOT NULL,
		       btrim(coalesce(d.email, '')) <> '',
		       btrim(coalesce(d.phone, '')) <> '',
		       EXISTS (SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
		                WHERE l.lead_id = d.id AND `+correspondenceSQL+`)
		  FROM lead d WHERE d.id = $1`, leadID).
		Scan(&held[0], &held[1], &held[2], &held[3], &held[4], &held[5])
	if err != nil {
		return fmt.Errorf("consent: reading which kinds of data this lead holds: %w", err)
	}
	out.DataCategories = heldCategories(held)
	return nil
}

// correspondenceSQL is an exchange with the person. The installation's own
// notices are left out: the notice that led here is not why data is held.
const correspondenceSQL = `a.kind IN ('email', 'meeting', 'call', 'message')
	AND a.origin <> 'system_notice' AND a.archived_at IS NULL`

// heldCategories turns the presence flags, in the order both reads select
// them, into the codes the page names.
func heldCategories(held [6]bool) []string {
	codes := [6]string{"name", "job_title", "employer", "email", "phone", "correspondence"}
	out := []string{}
	for i, present := range held {
		if present {
			out = append(out, codes[i])
		}
	}
	return out
}

// readRetention lists the acting rules that reach a person of this kind.
func (s *Store) readRetention(ctx context.Context, scopes []string, out *PrivacyInformation) error {
	out.Retention = []RetentionRule{}
	if s.retention == nil {
		return nil
	}
	rules, err := s.retention.ActingRetention(ctx)
	if err != nil {
		return fmt.Errorf("consent: reading how long data is kept: %w", err)
	}
	for _, rule := range rules {
		if slices.Contains(scopes, rule.Scope) {
			out.Retention = append(out.Retention, rule)
		}
	}
	return nil
}
