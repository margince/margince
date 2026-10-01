// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// providerUnitSource declares one provider-signed endpoint with a handshake;
// scheme and challenge are spliced in so each refusal names its own spelling.
func providerUnitSource(scheme, challenge string) string {
	return `package x

import (
	"context"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

func receive(context.Context, extension.Runtime, extension.InboundRequest) (extension.InboundOutcome, error) {
	return extension.InboundAccepted, nil
}

func handshake(context.Context, extension.Runtime, extension.InboundChallengeRequest) (string, extension.InboundOutcome, error) {
	return "", extension.InboundAccepted, nil
}

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		Inbound: []extension.InboundEndpoint{
			{
				Slug:            "messenger",
				Secret:          "inbound",
				Scheme:          ` + scheme + `,
				SignatureHeader: "X-Hub-Signature-256",
				MaxBody:         1 << 20,
				Rate: extension.InboundRate{
					PerIP:       extension.Rate{Limit: 300, Window: time.Minute},
					PerEndpoint: extension.Rate{Limit: 600, Window: time.Minute},
				},
				Handle:    receive,
				Challenge: ` + challenge + `,
			},
		},
	}
}
`
}

func TestAProviderSignedEndpointDerivesIntoTheManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", providerUnitSource("extension.SchemeProviderSigned", "handshake"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{
		`"scheme": "provider_signed"`,
		`"signature_header": "X-Hub-Signature-256"`,
		`"challenge": true`,
		`"skew_seconds": 0`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("derived manifest misses %s:\n%s", want, s)
		}
	}
	if strings.Contains(s, "handshake") {
		t.Errorf("derived manifest names the handshake function:\n%s", s)
	}
}

// Every manifest committed before schemes existed must stay byte-identical,
// so a Margince endpoint publishes none of the three keys.
func TestAMarginceEndpointPublishesNoSchemeKeys(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", inboundUnitSource(wholeEndpoint))
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"scheme"`, `"signature_header"`, `"challenge"`} {
		if strings.Contains(string(derived), absent) {
			t.Errorf("a Margince endpoint's manifest carries %s:\n%s", absent, derived)
		}
	}
}

func TestTheSchemeDerivationRefusesWhatItCannotRead(t *testing.T) {
	cases := map[string]struct{ scheme, challenge, want string }{
		"a numeric scheme": {"1", "handshake", "must be extension.SchemeMargince or extension.SchemeProviderSigned"},
		"a nil challenge":  {"extension.SchemeProviderSigned", "nil", "Challenge: nil"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := deriveSynthetic(t, "x", providerUnitSource(c.scheme, c.challenge))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("derive = %v, want an error naming %q", err, c.want)
			}
		})
	}
}
