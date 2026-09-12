// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

func deepReadFields(fields []evidencedField) []contacts.DeepReadField {
	out := make([]contacts.DeepReadField, len(fields))
	for i, field := range fields {
		out[i] = contacts.DeepReadField{
			Field: field.Field, Value: field.Value, EvidenceSnippet: field.EvidenceSnippet,
			SourceURL: field.SourceURL, Confidence: field.Confidence,
		}
	}
	return out
}

func siteReadContacts(found []siteContact) []contacts.SiteReadContact {
	out := make([]contacts.SiteReadContact, len(found))
	for i, contact := range found {
		out[i] = contacts.SiteReadContact{
			Name: contact.Name, Role: contact.Role, PublishedEmail: contact.PublishedEmail,
			LinkedinURL: contact.LinkedinURL, EvidenceSnippet: contact.EvidenceSnippet, SourceURL: contact.SourceURL,
		}
	}
	return out
}

func siteReadPages(pages []crawlPage) []contacts.SiteReadPage {
	out := make([]contacts.SiteReadPage, len(pages))
	for i, page := range pages {
		out[i] = contacts.SiteReadPage{URL: page.URL, Kind: string(page.Kind)}
	}
	return out
}

func siteReadProposalHash(fields []contacts.DeepReadField, facts []contacts.DeepReadFact, found []contacts.SiteReadContact, entities []contacts.SiteReadLegalEntity) (string, error) {
	raw, err := json.Marshal(struct {
		Fields   []contacts.DeepReadField       `json:"fields"`
		Facts    []contacts.DeepReadFact        `json:"facts"`
		Contacts []contacts.SiteReadContact     `json:"contacts"`
		Entities []contacts.SiteReadLegalEntity `json:"legal_entities"`
	}{Fields: fields, Facts: facts, Contacts: found, Entities: entities})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}
