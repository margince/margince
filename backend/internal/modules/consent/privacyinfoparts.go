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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// readHeld fills the parts read about this subject. Unwired, the page says
// nothing about them rather than something it cannot back.
func (s *Store) readHeld(ctx context.Context, subject NoticeSubject, out *PrivacyInformation) error {
	if s.held == nil {
		return nil
	}
	held, err := s.held.NoticeHeld(ctx, subject)
	if err != nil {
		return fmt.Errorf("consent: reading what is held about this reader: %w", err)
	}
	out.DataCategories = append(out.DataCategories, held.Categories...)
	out.Retention = append(out.Retention, held.Retention...)
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return namePurposesTx(ctx, tx, held.Uses, out) })
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
	}
	return nil
}
