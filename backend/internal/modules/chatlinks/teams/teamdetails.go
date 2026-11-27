// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamDetails identifies the installed team available to the channel connector.
//
//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type TeamDetails struct {
	ID    string `json:"id"`
	Group string `json:"aadGroupId"`
	Name  string `json:"name"`
}

// TeamDetails identifies the installed team available to the channel connector.
func (b *Bot) TeamDetails(ctx context.Context) (TeamDetails, error) {
	if b.nativeTeam == "" {
		return TeamDetails{}, messaging.ErrInvalid
	}
	raw, err := b.connector.Call(ctx, http.MethodGet, "/v3/teams/"+url.PathEscape(b.nativeTeam), nil)
	if err != nil {
		return TeamDetails{}, err
	}
	var details TeamDetails
	if json.Unmarshal(raw, &details) != nil || details.ID != b.nativeTeam || b.team != "" && b.team != details.Group {
		return TeamDetails{}, messaging.ErrUnauthorized
	}
	if _, err := ids.Parse(details.Group); err != nil {
		return TeamDetails{}, messaging.ErrUnavailable
	}
	return details, nil
}
