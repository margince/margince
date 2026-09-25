// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"context"
	"strings"
	"testing"
	"time"
)

// providerEndpoint is a complete provider-signed declaration; each refusal
// below spoils exactly one field of it.
func providerEndpoint() InboundEndpoint {
	return InboundEndpoint{
		Slug:            "messenger",
		Secret:          "app-secret",
		Scheme:          SchemeProviderSigned,
		SignatureHeader: "X-Hub-Signature-256",
		MaxBody:         1 << 20,
		Rate: InboundRate{
			PerIP:       Rate{Limit: 300, Window: time.Minute},
			PerEndpoint: Rate{Limit: 600, Window: time.Minute},
		},
		Handle: func(context.Context, Runtime, InboundRequest) (InboundOutcome, error) {
			return InboundAccepted, nil
		},
		Challenge: func(context.Context, Runtime, InboundChallengeRequest) (string, InboundOutcome, error) {
			return "", InboundAccepted, nil
		},
	}
}

func TestAProviderSignedEndpointValidates(t *testing.T) {
	if err := providerEndpoint().Validate(); err != nil {
		t.Fatalf("a complete provider-signed endpoint was refused: %v", err)
	}
}

func TestTheZeroSchemeIsMargince(t *testing.T) {
	var e InboundEndpoint
	if e.Scheme != SchemeMargince {
		t.Fatal("the zero scheme must be SchemeMargince, or every declaration written before schemes existed changes meaning")
	}
}

func TestProviderSchemeRefusals(t *testing.T) {
	cases := map[string]struct {
		spoil func(*InboundEndpoint)
		want  string
	}{
		"a clock skew":            {func(e *InboundEndpoint) { e.Skew = time.Minute }, "clock skew"},
		"no signature header":     {func(e *InboundEndpoint) { e.SignatureHeader = "" }, "no usable signature header"},
		"a header with a space":   {func(e *InboundEndpoint) { e.SignatureHeader = "X Hub" }, "no usable signature header"},
		"a Margince header":       {func(e *InboundEndpoint) { e.SignatureHeader = "X-Margince-Signature" }, "Margince header"},
		"a scheme nobody defined": {func(e *InboundEndpoint) { e.Scheme = InboundScheme(7) }, "scheme 7"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := providerEndpoint()
			c.spoil(&e)
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Validate() = %v, want an error naming %q", err, c.want)
			}
		})
	}
}

func TestTheMarginceSchemeRefusesProviderFields(t *testing.T) {
	e := providerEndpoint()
	e.Scheme = SchemeMargince
	e.Skew = 5 * time.Minute
	e.Challenge = nil
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "signature header under the Margince scheme") {
		t.Fatalf("a Margince endpoint naming a provider header: Validate() = %v", err)
	}
	e.SignatureHeader = ""
	e.Challenge = providerEndpoint().Challenge
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "Challenge under the Margince scheme") {
		t.Fatalf("a Margince endpoint declaring a Challenge: Validate() = %v", err)
	}
	e.Challenge = nil
	if err := e.Validate(); err != nil {
		t.Fatalf("a plain Margince endpoint was refused: %v", err)
	}
}

func TestAMarginceEndpointStillNeedsItsSkew(t *testing.T) {
	e := providerEndpoint()
	e.Scheme, e.SignatureHeader, e.Challenge = SchemeMargince, "", nil
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "sets no clock skew") {
		t.Fatalf("a Margince endpoint with no skew: Validate() = %v", err)
	}
}
