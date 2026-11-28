// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// RefusedRecipient is one recipient's half of a refusal, as the engine gave it.
//
// The ADDRESS is stored, which is why the privacy engine clears this column
// with the subject. A reason code without an address says a message was refused
// and not who for, which is the question the rep is asking.
type RefusedRecipient struct {
	PolicyRevision    string                     `json:"policy_revision,omitempty"`
	FooterDigest      string                     `json:"footer_digest,omitempty"`
	FooterPreview     []string                   `json:"footer_preview,omitempty"`
	FooterFindings    []commsauthz.FooterFinding `json:"footer_findings,omitempty"`
	SendingEntity     *ids.UUID                  `json:"sending_entity,omitempty"`
	SendingEntityName string                     `json:"-"`
	Address           string                     `json:"address"`
	SubjectKind       string                     `json:"subject_kind,omitempty"`
	SubjectID         string                     `json:"subject_id,omitempty"`
	ReasonCode        string                     `json:"reason_code"`
	Category          string                     `json:"category,omitempty"`
	// Requested is what the sender claimed the message is, which an unproven
	// claim never writes into Category. Empty on a snapshot from before it.
	Requested string `json:"requested,omitempty"`
	// DealID is the deal the message named as its evidence, so an answer to
	// this refusal is recorded about that deal. Nil when it named none.
	DealID *ids.UUID `json:"deal_id,omitempty"`
	// PurposeID is the purpose the SEND resolved to. It is nil for a broad send,
	// and for a snapshot written before this field existed.
	PurposeID *ids.UUID `json:"purpose_id,omitempty"`
	// Country is the recipient's country the refusal was judged under, empty
	// for the baseline, so a confirmation never outlives a changed country.
	Country       string `json:"country,omitempty"`
	CountrySource string `json:"country_source,omitempty"`
	// Provision is the law cited for ReasonCode at assessment, zero when none
	// was shown. Stored, so a later pack never puts new law beside it.
	Provision commsauthz.Provision `json:"provision,omitzero"`
	// Provisions holds each law cited for ReasonCode at assessment, Provision
	// first. Empty on a snapshot from before it, which then shows Provision.
	Provisions []commsauthz.Provision `json:"provisions,omitempty"`
	// CanBeOverruled is the preview's own answer for this refusal. It is told
	// to the sender and never stored: a stored copy would outlive the rule.
	CanBeOverruled bool `json:"-"`
	// RequestedCategory is the reason for contact the sender picked. It is told
	// to the sender and never stored: the decision row already records it, and
	// Category stays the engine's own reading. An unproven claim never
	// resolves, so this is the only place the sender's pick travels. The
	// remedy for a failed active-deal claim is named from it.
	RequestedCategory string `json:"-"`
}

// claim is what the sender said the message is: the claim when one was made,
// otherwise the engine's reading.
func (r RefusedRecipient) claim() commsauthz.Category {
	if r.Requested != "" {
		return commsauthz.Category(r.Requested)
	}
	return commsauthz.Category(r.Category)
}

// dealID is the deal the refused message named, or zero.
func (r RefusedRecipient) dealID() ids.UUID {
	if r.DealID == nil {
		return ids.UUID{}
	}
	return *r.DealID
}

func refusedRecipientOf(d commsauthz.Decision) RefusedRecipient {
	refusal := RefusedRecipient{
		PolicyRevision: d.Evidence.PolicyRevision,
		FooterFindings: d.Evidence.FooterFindings,
		FooterPreview:  d.Evidence.FooterPreview,
		FooterDigest:   d.Evidence.FooterDigest,
		// Normalized on the way in, because the erasure sweeps match on it.
		// A lowercased, trimmed comparison walks past an address stored as
		// typed. What it walks past is an erased subject's mailbox.
		Address:       correspondence.Fold(d.Recipient.Email),
		SubjectKind:   d.SubjectKind,
		ReasonCode:    d.ReasonCode,
		Provision:     d.ProvisionFor(d.ReasonCode),
		Provisions:    d.ProvisionsFor(d.ReasonCode),
		Category:      string(d.Resolved),
		Requested:     string(d.Requested),
		PurposeID:     d.PurposeID,
		Country:       d.Country,
		CountrySource: d.CountrySource,
		DealID:        zeroUUIDAsNull(d.Evidence.DealID),
		SendingEntity: zeroUUIDAsNull(d.SendingEntity),
		// The same answer the preview sends, so the composer picks the same
		// deciding recipient before and after Send.
		CanBeOverruled:    d.OverrulableForOneMessage(),
		RequestedCategory: string(d.Requested),
	}
	if !d.SubjectID.IsZero() {
		refusal.SubjectID = d.SubjectID.String()
	}
	return refusal
}
