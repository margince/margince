// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import "time"

// Credential contains secret material for sealed storage and provider exchanges.
type Credential struct {
	InstallationID string    `json:"installation_id,omitempty"`
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	Scopes         []string  `json:"scopes,omitempty"`
	AppID          string    `json:"app_id,omitempty"`
}
