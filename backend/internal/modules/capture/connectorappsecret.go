// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type appSecrets struct {
	Client  string `json:"client_secret"`
	Signing string `json:"signing_secret"`
}

func appSecretPayload(provider AppProvider, client, signing string) ([]byte, error) {
	if signing == "" {
		return []byte(client), nil
	}
	if provider != AppProviderSlack || len(signing) > maxAppFieldBytes {
		return nil, fmt.Errorf("only a Slack app accepts a signing secret: %w", apperrors.ErrInvalidArgument)
	}
	if len(signing) < 32 {
		return nil, fmt.Errorf("the Slack signing secret must contain at least 32 characters: %w", apperrors.ErrInvalidArgument)
	}
	return json.Marshal(appSecrets{Client: client, Signing: signing}) // #nosec G117 -- Encoded only for encrypted application-secret storage.
}

func resolveAppSecret(app ConnectorApp, secret []byte) (ConnectorApp, error) {
	if !app.SigningSecretSet {
		app.ClientSecretRef = string(secret)
		return app, nil
	}
	var payload appSecrets
	if json.Unmarshal(secret, &payload) != nil || payload.Client == "" || payload.Signing == "" {
		return ConnectorApp{}, ErrNoVault
	}
	app.ClientSecretRef, app.SigningSecret = payload.Client, payload.Signing
	return app, nil
}
