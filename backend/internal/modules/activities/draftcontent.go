// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// DraftContentFinding is one advertising content rule a draft does not meet.
// Mirrors consent.DraftFinding across the module boundary.
type DraftContentFinding struct {
	Requirement string
	Detail      string
	Words       []string
	Language    string
	Country     string
	Provision   commsauthz.Provision
}

// DraftContentChecker answers what an advertising draft does not meet among
// its recipients' content rules. Declared here, implemented by consent and
// injected in compose/.
type DraftContentChecker interface {
	CheckAdvertisingDraft(ctx context.Context, recipients []string, countries []commsauthz.CountryOverride, subject, body string) ([]DraftContentFinding, error)
}

// WithDraftContent wires the checker onto the preview.
func (s *Store) WithDraftContent(c DraftContentChecker) *Store {
	clone := *s
	clone.draftContent = c
	return &clone
}

// WithDraftContent is the handler-level wiring, matching WithDisclosures.
func (h Handlers) WithDraftContent(c DraftContentChecker) Handlers {
	h.store = h.store.WithDraftContent(c)
	return h
}

// draftContentFindings asks about a draft with text to read that is
// advertising by the sender's claim or by the engine's reading of a recipient.
func (s *Store) draftContentFindings(ctx context.Context, in PreviewSendInput, decisions commsauthz.DecisionSet) ([]DraftContentFinding, error) {
	if s.draftContent == nil || strings.TrimSpace(in.Subject+in.Body) == "" {
		return nil, nil
	}
	advertising := in.Context == commsauthz.CategoryMarketing
	for _, d := range decisions.Decisions {
		advertising = advertising || d.Resolved == commsauthz.CategoryMarketing
	}
	if !advertising {
		return nil, nil
	}
	return s.draftContent.CheckAdvertisingDraft(ctx, in.Recipients, in.Evidence.RecipientCountries, in.Subject, in.Body)
}
