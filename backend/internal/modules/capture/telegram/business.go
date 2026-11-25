// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

// The business_connection update: Telegram's report that the owner of a Telegram
// Business account linked this bot to their account, or withdrew it. It carries
// the owner and the link and no message or customer, so the poller can act on it
// before any payload is stored.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// BusinessConnection is one business_connection update, narrowed to what the
// connection row keeps.
type BusinessConnection struct {
	// ID is the connection id Telegram issued, the value a later
	// business_message names.
	ID string
	// OwnerID is the owner's Telegram user id. It is the stable account: the ID
	// above changes when the owner links the bot again.
	OwnerID string
	// OwnerLabel is the owner's name as Telegram shows it, for the admin who
	// is asked to approve. A name is mutable and identifies nothing.
	OwnerLabel string
	Enabled    bool
}

type businessUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type businessConnectionPayload struct {
	ID        string       `json:"id"`
	User      businessUser `json:"user"`
	IsEnabled bool         `json:"is_enabled"`
}

type businessConnectionEnvelope struct {
	BusinessConnection *businessConnectionPayload `json:"business_connection"`
}

// businessLabelLimit is channel_connection.channel_label's bound.
const businessLabelLimit = 120

// ParseBusinessConnection reports whether update is a business_connection and
// decodes it. An update of another kind is not an error. A business_connection
// naming no connection or no owner is refused, because a row keyed on an empty
// account would be shared by every such update.
func ParseBusinessConnection(update []byte) (BusinessConnection, bool, error) {
	var env businessConnectionEnvelope
	if err := json.Unmarshal(update, &env); err != nil {
		return BusinessConnection{}, false, fmt.Errorf("telegram: decoding the update to find a business connection: %w", err)
	}
	payload := env.BusinessConnection
	if payload == nil {
		return BusinessConnection{}, false, nil
	}
	if payload.ID == "" || payload.User.ID <= 0 {
		return BusinessConnection{}, true, errors.New("telegram: a business connection names no connection id or no owner")
	}
	return BusinessConnection{
		ID:         payload.ID,
		OwnerID:    fmt.Sprintf("%d", payload.User.ID),
		OwnerLabel: businessOwnerLabel(payload.User),
		Enabled:    payload.IsEnabled,
	}, true, nil
}

// businessOwnerLabel is the name an admin recognises the owner by: the full
// name, else the handle, else the account id.
func businessOwnerLabel(user businessUser) string {
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	switch {
	case name != "":
	case user.Username != "":
		name = "@" + user.Username
	default:
		name = fmt.Sprintf("%d", user.ID)
	}
	return textcut.Runes(name, businessLabelLimit)
}
