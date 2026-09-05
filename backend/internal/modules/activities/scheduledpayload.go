// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a scheduled message keeps while it waits, and how it comes back.
//
// Its own file because freeze and thaw are one concept and scheduledsend.go had
// outgrown the size cap. They are inverses, and the only place a send's own
// description can narrow without anything failing: a field added to
// SendEmailInput and not carried here is dropped silently, and the message
// still goes out — just describing itself as less than the rep said it was.
// scheduledpayload_test.go compares the round trip field by field for that
// reason, rather than asserting the handful somebody remembered.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

type scheduledPayload struct {
	Recipients     []string `json:"recipients"`
	Cc             []string `json:"cc,omitempty"`
	Bcc            []string `json:"bcc,omitempty"`
	Subject        string   `json:"subject"`
	Body           string   `json:"body"`
	HTMLBody       string   `json:"html_body,omitempty"`
	AttachmentIDs  []string `json:"attachment_ids,omitempty"`
	ConsentPurpose string   `json:"consent_purpose"`
	DraftRef       string   `json:"draft_ref,omitempty"`
	// The four context fields the send door decoded. Frozen with the message
	// for the reason everything else here is: what the rep claimed at 17:00
	// today is what the decision row must say when the message goes out at
	// 09:00 tomorrow. A payload that dropped them recorded a scheduled
	// marketing send as claiming nothing, and made its proof row strictly
	// worse than the same message sent immediately.
	Context          string         `json:"context,omitempty"`
	MarketingPurpose string         `json:"marketing_purpose,omitempty"`
	OperatorReason   string         `json:"operator_reason,omitempty"`
	Evidence         frozenEvidence `json:"evidence,omitzero"`
}

// frozenEvidence is the evidence block as it sits on a scheduled row. Ids as
// strings, because a payload outlives the code that wrote it and a typed id is
// the code's shape rather than the row's.
type frozenEvidence struct {
	ActivityID     string `json:"activity_id,omitempty"`
	DealID         string `json:"deal_id,omitempty"`
	InvoiceID      string `json:"invoice_id,omitempty"`
	ContractID     string `json:"contract_id,omitempty"`
	ConsentEventID string `json:"consent_event_id,omitempty"`
	BasisID        string `json:"basis_id,omitempty"`
}

// freezeEvidence and thawEvidence are inverses. An id that is zero stays empty
// rather than becoming the zero UUID's text, so a round trip through the row
// gives back exactly what went in.
func freezeEvidence(e commsauthz.Evidence) frozenEvidence {
	return frozenEvidence{
		ActivityID:     idText(e.ActivityID),
		DealID:         idText(e.DealID),
		InvoiceID:      idText(e.InvoiceID),
		ContractID:     idText(e.ContractID),
		ConsentEventID: idText(e.ConsentEventID),
		BasisID:        idText(e.BasisID),
	}
}

func (f frozenEvidence) thaw() (commsauthz.Evidence, error) {
	var out commsauthz.Evidence
	for _, field := range []struct {
		raw  string
		into *ids.UUID
	}{
		{f.ActivityID, &out.ActivityID},
		{f.DealID, &out.DealID},
		{f.InvoiceID, &out.InvoiceID},
		{f.ContractID, &out.ContractID},
		{f.ConsentEventID, &out.ConsentEventID},
		{f.BasisID, &out.BasisID},
	} {
		if field.raw == "" {
			continue
		}
		id, err := ids.Parse(field.raw)
		if err != nil {
			return commsauthz.Evidence{}, fmt.Errorf("scheduled send: evidence id %q: %w", field.raw, err)
		}
		*field.into = id
	}
	return out, nil
}

// thawOriginLinks reads the records an account-started row froze. The ONE
// reader for the list, the detail and the fire: the shape carries no JSON tags,
// so it is held only by everybody decoding it the same way, and three decodings
// of one column would be three places for that to stop being true. A NULL
// column — a reply row, which the origin-shape CHECK requires it of — reads as
// no records rather than as an error.
func thawOriginLinks(raw []byte) ([]ActivityLinkInput, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var links []ActivityLinkInput
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, fmt.Errorf("scheduled send: reading the frozen record links: %w", err)
	}
	return links, nil
}

// idText renders an id for the row, and an unnamed record as the empty string.
func idText(id ids.UUID) string {
	if id == (ids.UUID{}) {
		return ""
	}
	return id.String()
}

func freezePayload(in SendEmailInput) scheduledPayload {
	files := make([]string, 0, len(in.AttachmentIDs))
	for _, id := range in.AttachmentIDs {
		files = append(files, id.String())
	}
	return scheduledPayload{
		Recipients:     in.Recipients,
		Cc:             in.Cc,
		Bcc:            in.Bcc,
		Subject:        in.Subject,
		Body:           in.Body,
		HTMLBody:       in.HTMLBody,
		AttachmentIDs:  files,
		ConsentPurpose: in.ConsentPurpose,
		DraftRef:       in.DraftRef,

		Context:          string(in.Context),
		MarketingPurpose: in.MarketingPurpose,
		OperatorReason:   in.OperatorReason,
		Evidence:         freezeEvidence(in.Evidence),
	}
}

func (p scheduledPayload) thaw() (SendEmailInput, error) {
	files := make([]ids.UUID, 0, len(p.AttachmentIDs))
	for _, raw := range p.AttachmentIDs {
		id, err := ids.Parse(raw)
		if err != nil {
			return SendEmailInput{}, fmt.Errorf("scheduled send: attachment id %q: %w", raw, err)
		}
		files = append(files, id)
	}
	evidence, err := p.Evidence.thaw()
	if err != nil {
		return SendEmailInput{}, err
	}
	// A category reserved for the installation's own notices can never
	// legitimately be in a scheduled payload: the door that wrote it refuses
	// one, and freezePayload has a single caller behind that door. Refusing it
	// again here costs a live send nothing and removes the assumption — a
	// second payload writer added later cannot smuggle one past the fire.
	if commsauthz.Category(p.Context).ServesTheSubject() {
		return SendEmailInput{}, &CommunicationContextError{
			Reason: "that category is reserved for the installation's own notices and cannot be claimed by a send",
		}
	}
	return SendEmailInput{
		Recipients:     p.Recipients,
		Cc:             p.Cc,
		Bcc:            p.Bcc,
		Subject:        p.Subject,
		Body:           p.Body,
		HTMLBody:       p.HTMLBody,
		AttachmentIDs:  files,
		ConsentPurpose: p.ConsentPurpose,
		DraftRef:       p.DraftRef,

		Context:          commsauthz.Category(p.Context),
		MarketingPurpose: p.MarketingPurpose,
		OperatorReason:   p.OperatorReason,
		Evidence:         evidence,
	}, nil
}
