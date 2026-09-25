// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

// The mailbox's own labels, so an owner can pick one to keep out of capture.
//
// Its own file because it is the only read here that serves a CHOICE rather
// than the pipeline: everything else in the client fetches mail or advances a
// cursor, and this answers "what is in your mailbox" for somebody about to
// write a rule about it.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// labelsPage is one users.labels.list response. Gmail returns every label in
// one answer — there is no paging on this endpoint — so there is no cursor to
// carry.
type labelsPage struct {
	Labels []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"labels"`
}

// ListLabels returns the mailbox's labels, system and user alike.
//
// Both kinds, because both are things an owner keeps mail in: CATEGORY_SOCIAL
// is Gmail's and "Privat" is theirs, and a picker that offered only the second
// would leave the noisiest folders unexcludable. What each one IS travels as
// the provider's type so a caller can group them; deciding which are worth
// offering is not this client's to make.
func (a *httpAPI) ListLabels(ctx context.Context, accessToken string) ([]connector.NamedContainer, error) {
	var page labelsPage
	if _, err := a.get(ctx, accessToken, "/labels", nil, &page, maxJSONResponseBytes); err != nil {
		return nil, err
	}
	out := make([]connector.NamedContainer, 0, len(page.Labels))
	for _, l := range page.Labels {
		if l.ID == "" {
			continue
		}
		// The NAME falls back to the id rather than travelling empty: a picker
		// showing a blank row is worse than one showing an opaque token, and a
		// label with no name is a provider answer nobody can act on.
		name := l.Name
		if name == "" {
			name = l.ID
		}
		out = append(out, connector.NamedContainer{ID: l.ID, Name: name})
	}
	return out, nil
}
