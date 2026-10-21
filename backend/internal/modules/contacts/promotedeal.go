// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Qualify-to-deal: a promote call may ask for a deal to be opened in the
// same transaction, so "this lead is now a contact with an opportunity" is
// one fact or none. The deal itself is the deals module's to write; contacts
// names what it needs through a port compose satisfies (ADR-0054: a module
// never imports a sibling).

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// QualifyDealInput is what a qualify call asks the deal opener for. Nil
// pipeline and stage mean the default pipeline's first open stage.
type QualifyDealInput struct {
	PipelineID  *ids.UUID
	StageID     *ids.UUID
	Name        string
	AmountMinor *int64
	Currency    *string
	OwnerID     *ids.UserID
	// Source is the deal's provenance value; the lead's id, so the deal says
	// where it came from.
	Source string
}

// LeadDealOpener opens one deal inside the caller's transaction and answers
// its id. Compose binds it to the deals store.
type LeadDealOpener interface {
	OpenDealForLead(ctx context.Context, tx pgx.Tx, in QualifyDealInput) (ids.UUID, error)
}

// WithDealOpener wires the deals-side seam the qualify dialog's "also open a
// deal" rides on. Without it a promote that asks for a deal is refused
// rather than quietly promoting without one.
func (s *Store) WithDealOpener(opener LeadDealOpener) *Store {
	s.dealOpener = opener
	return s
}

// DealOpenerNotWiredError maps to 422: the installation cannot open a deal
// from a qualify call.
type DealOpenerNotWiredError struct{}

func (e *DealOpenerNotWiredError) Error() string {
	return "opening a deal while qualifying is not available on this installation; qualify without a deal and open one from the deals screen"
}

// MessageFault names the condition; the remedy is to drop the deal block.
func (e *DealOpenerNotWiredError) MessageFault() (code, message string) {
	return "deal_opener_unavailable", e.Error()
}

// PromoteOutcome is what a promotion produced: the contact, whether it merged
// into an existing one, and the deal opened alongside when one was asked for.
type PromoteOutcome struct {
	Contact crmcontracts.Contact
	Merged  bool
	DealID  *ids.UUID
}

// openQualifiedDeal runs once the contact exists and before the lead is
// marked promoted, under the lead row lock and inside the same transaction:
// the deal opens, the contact is seated on it, and the caller writes the
// deal's id into the promotion's own UPDATE and audit image. A deal failure
// fails the whole promotion.
func (s *Store) openQualifiedDeal(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, lead crmcontracts.Lead, contactID ids.ContactID, in *QualifyDealInput) (*ids.UUID, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // Deal creation is optional during lead promotion.
	}
	if s.dealOpener == nil {
		return nil, &DealOpenerNotWiredError{}
	}
	deal := qualifiedDealInput(leadID, lead, *in)
	dealID, err := s.dealOpener.OpenDealForLead(ctx, tx, deal)
	if err != nil {
		return nil, fmt.Errorf("open deal for qualified lead: %w", err)
	}
	// The contact becomes a stakeholder on the deal the qualify call opened. It
	// is the edge the deal page's committee shows, and it stops an undo of the
	// qualification while the deal is live. The import's contact_ref writes it
	// through the same function.
	seated := ids.From[ids.DealKind](dealID)
	_, err = s.CreateRelationshipTx(ctx, tx, CreateRelationshipInput{
		Kind:      dealStakeholderKind,
		ContactID: &contactID,
		DealID:    &seated,
		Source:    deal.Source,
	})
	if err != nil {
		return nil, fmt.Errorf("seat the contact on the qualified deal: %w", err)
	}
	return &dealID, nil
}

// defaultQualifiedDealName is the deal name when the dialog sends none: the
// company when the lead names one, else the contact.
func defaultQualifiedDealName(lead crmcontracts.Lead) string {
	if lead.CompanyName != nil && strings.TrimSpace(*lead.CompanyName) != "" {
		return strings.TrimSpace(*lead.CompanyName)
	}
	if lead.FullName != nil && strings.TrimSpace(*lead.FullName) != "" {
		return strings.TrimSpace(*lead.FullName)
	}
	if lead.Email != nil {
		return string(*lead.Email)
	}
	return "Qualified lead"
}

func qualifiedDealInput(leadID ids.LeadID, lead crmcontracts.Lead, in QualifyDealInput) QualifyDealInput {
	deal := in
	if strings.TrimSpace(deal.Name) == "" {
		deal.Name = defaultQualifiedDealName(lead)
	}
	deal.OwnerID = idArg[ids.UserKind](lead.OwnerId)
	deal.Source = "lead:" + leadID.String()
	return deal
}

// LeadDealPreviewer shares the deal opener's defaults and admission checks.
type LeadDealPreviewer interface {
	PreviewDealForLead(context.Context, pgx.Tx, QualifyDealInput) (QualifyDealInput, error)
}

func (s *Store) previewQualifiedDeal(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, in *QualifyDealInput) (*QualifyDealInput, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // Deal creation is optional during lead promotion.
	}
	if err := requireRelationshipCreate(ctx, anchorDeal); err != nil {
		return nil, err
	}
	previewer, ok := s.dealOpener.(LeadDealPreviewer)
	if !ok {
		return nil, &DealOpenerNotWiredError{}
	}
	resolved, err := previewer.PreviewDealForLead(ctx, tx, qualifiedDealInput(ids.From[ids.LeadKind](ids.UUID(lead.Id)), lead, *in))
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}
