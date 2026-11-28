// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oidctoken

import "time"

// NewBotFramework uses the Bot Framework issuer and audience verification rules.
func NewBotFramework() *Verifier {
	verifier := New("https://login.botframework.com/v1/.well-known/keys", func(claims Claims) error {
		if claims.Iss != "https://api.botframework.com" || claims.Nbf == 0 {
			return ErrRejected
		}
		return nil
	}, IdentityResolvedPerRequest)
	verifier.skew = 5 * time.Minute
	verifier.requiredEndorsement = "msteams"
	verifier.allowMissingIssuedAt = true
	return verifier
}

// BotFrameworkIdentity checks the application and service binding of a Bot Framework token.
func BotFrameworkIdentity(appID, serviceURL string) func(Claims) error {
	return func(claims Claims) error {
		if appID == "" || serviceURL == "" || !claims.Aud.Is(appID) || claims.ServiceURL != serviceURL {
			return ErrRejected
		}
		return nil
	}
}
