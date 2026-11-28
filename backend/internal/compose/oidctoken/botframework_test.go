// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package oidctoken

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/oidctoken/oidctokentest"
)

func TestBotFrameworkRequiresTeamsEndorsementAudienceAndExactServiceURL(t *testing.T) {
	const app = "synthetic-teams-app"
	const service = "https://smba.trafficmanager.net/amer/"
	for _, tc := range []struct {
		name         string
		endorsements []string
		claims       map[string]any
		accepted     bool
	}{
		{name: "valid connector token without issued-at", endorsements: []string{"msteams"}, accepted: true},
		{name: "no channel endorsement"},
		{name: "different channel endorsement", endorsements: []string{"webchat"}},
		{name: "different bot audience", endorsements: []string{"msteams"}, claims: map[string]any{"aud": "other-bot"}},
		{name: "different issuer", endorsements: []string{"msteams"}, claims: map[string]any{"iss": "https://accounts.google.com"}},
		{name: "different service URL", endorsements: []string{"msteams"}, claims: map[string]any{"serviceurl": "https://example.test/"}},
		{name: "missing service URL", endorsements: []string{"msteams"}, claims: map[string]any{"serviceurl": ""}},
		{name: "not valid yet", endorsements: []string{"msteams"}, claims: map[string]any{"nbf": int64(1750000360)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := oidctokentest.NewRig(t)
			rig.Endorsements = tc.endorsements
			claims := map[string]any{"iss": "https://api.botframework.com", "aud": app, "nbf": rig.Base.Add(-time.Minute).Unix(), "iat": nil, "serviceurl": service}
			maps.Copy(claims, tc.claims)
			token := rig.Mint(t, oidctokentest.KID, "RS256", claims)
			verifier := NewBotFramework().WithHTTPClient(rig.Srv.Client()).SetClock(func() time.Time { return rig.Base })
			verifier.jwksURL = rig.JWKSURL()
			_, err := verifier.VerifyAs(context.Background(), token, BotFrameworkIdentity(app, service))
			if (err == nil) != tc.accepted {
				t.Fatalf("accepted=%v wanted=%v error=%v", err == nil, tc.accepted, err)
			}
			if err != nil && !errors.Is(err, ErrRejected) {
				t.Fatalf("lost rejection contract: %v", err)
			}
			if _, err := verifier.Verify(context.Background(), token); err == nil {
				t.Fatal("accepted a token without the request's bot identity proof")
			}
		})
	}
}
