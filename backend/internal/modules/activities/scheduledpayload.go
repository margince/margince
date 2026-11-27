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
	PendingDraftID string   `json:"pending_draft_id,omitempty"`
	sendClaims
	// FromConnection is the mailbox the sender chose. A payload with none
	// leaves the choice to the fire, as an unchosen send does.
	FromConnection string `json:"from_connection,omitempty"`
	// ResentFrom is the bounced delivery this message repeats. A repeat that a
	// gate holds and a user later releases is still that bounce's one repeat.
	// The release stamps it, so the bounce cannot be sent again beside it.
	ResentFrom string `json:"resent_from,omitempty"`
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
	// RecipientCountries are the countries the sender stated. A payload
	// written before them has none, which is every recipient on the record's
	// country, so version 2 reads both shapes.
	RecipientCountries []frozenCountry `json:"recipient_countries,omitempty"`
}

// frozenCountry keeps a cleared country as null, apart from no entry.
type frozenCountry struct {
	Address string  `json:"address"`
	Country *string `json:"country"`
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

		RecipientCountries: freezeCountries(e.RecipientCountries),
	}
}

func freezeCountries(stated []commsauthz.CountryOverride) []frozenCountry {
	if len(stated) == 0 {
		return nil
	}
	out := make([]frozenCountry, 0, len(stated))
	for _, o := range stated {
		entry := frozenCountry{Address: o.Address}
		if !o.Cleared() {
			entry.Country = &o.Country
		}
		out = append(out, entry)
	}
	return out
}

func thawCountries(frozen []frozenCountry) []commsauthz.CountryOverride {
	if len(frozen) == 0 {
		return nil
	}
	out := make([]commsauthz.CountryOverride, 0, len(frozen))
	for _, entry := range frozen {
		out = append(out, commsauthz.CountryOverride{Address: entry.Address, Country: orEmpty(entry.Country)})
	}
	return out
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
	out.RecipientCountries = thawCountries(f.RecipientCountries)
	return out, nil
}

// thawOriginLinks reads the records an account-started row froze, for the list,
// the detail and the fire alike. The shape carries no JSON tags, so what holds
// it is every reader decoding it the same way, and a decoding of its own in
// each caller would be three places for that to stop being true. A NULL column
// — a reply row, which the origin-shape CHECK requires it of — reads as no
// records rather than as an error.
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

		sendClaims:     freezeClaims(in),
		FromConnection: idText(in.FromConnection),
		ResentFrom:     idText(in.ResentFrom),
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
	claims, err := p.sendClaims.thaw()
	if err != nil {
		return SendEmailInput{}, err
	}
	from, err := frozenID("sending mailbox", p.FromConnection)
	if err != nil {
		return SendEmailInput{}, err
	}
	resent, err := frozenID("bounced delivery", p.ResentFrom)
	if err != nil {
		return SendEmailInput{}, err
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

		Context:          claims.Context,
		MarketingPurpose: claims.MarketingPurpose,
		OperatorReason:   claims.OperatorReason,
		Evidence:         claims.Evidence,
		SendingEntity:    claims.SendingEntity,
		FromConnection:   from,
		ResentFrom:       resent,
	}, nil
}

// frozenID reads an optional id off a payload; empty is the zero id.
func frozenID(what, raw string) (ids.UUID, error) {
	if raw == "" {
		return ids.UUID{}, nil
	}
	id, err := ids.Parse(raw)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("scheduled send: %s %q: %w", what, raw, err)
	}
	return id, nil
}
