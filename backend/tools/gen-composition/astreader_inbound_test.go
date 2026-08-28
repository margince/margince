// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// inboundUnitSource is a unit declaring the given Inbound slice elements.
func inboundUnitSource(entries string) string {
	return `package x

import (
	"context"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

func receive(context.Context, extension.Runtime, extension.InboundRequest) (extension.InboundOutcome, error) {
	return extension.InboundAccepted, nil
}

var _ = time.Minute

func New() extension.Extension {
	return extension.Extension{
		Name:    "x",
		Version: "0.1.0",
		Inbound: []extension.InboundEndpoint{
` + entries + `
		},
	}
}
`
}

// wholeEndpoint is a complete declaration; the refusal cases below spoil one
// field of it so each test names exactly the field it is about.
const wholeEndpoint = `			{
				Slug:    "capture",
				Secret:  "inbound",
				MaxBody: 64 << 10,
				Rate: extension.InboundRate{
					PerIP:       extension.Rate{Limit: 60, Window: time.Minute},
					PerEndpoint: extension.Rate{Limit: 120, Window: time.Minute},
				},
				Skew:   5 * time.Minute,
				Handle: receive,
			},
`

// TestInboundDerivesIntoManifest: an anonymous edge is the one capability
// reached by a party the installation never authenticated, so an operator must
// be able to read every one of them, and its bounds, without opening the source.
func TestInboundDerivesIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", inboundUnitSource(wholeEndpoint))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{
		`"slug": "capture"`,
		`"secret": "inbound"`,
		`"max_body": 65536`,
		`"skew_seconds": 300`,
		`"limit": 60`,
		`"limit": 120`,
		`"window_seconds": 60`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("derived manifest misses %s:\n%s", want, s)
		}
	}
}

// The manifest must not carry the handler: a function identifier tells an
// operator nothing, and publishing it would invite reading the document as a
// call graph.
func TestInboundManifestOmitsTheHandler(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", inboundUnitSource(wholeEndpoint))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(derived), "receive") {
		t.Errorf("derived manifest names the handler:\n%s", derived)
	}
}

func TestNoInboundOmitsTheField(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", toolUnitSource("\t\t\tName: \"t\","),
		syntheticVerb("x", "t", "auto_execute", "read"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(derived), `"inbound"`) {
		t.Errorf("a unit declaring no inbound edge carries the key:\n%s", derived)
	}
}

func TestInboundDerivationRefusals(t *testing.T) {
	tests := []struct {
		name    string
		entries string
		want    string
	}{
		{
			"a slug declared twice within one unit",
			wholeEndpoint + strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 1024`, 1),
			"declared twice",
		},
		{
			"a field this generator cannot derive",
			strings.Replace(wholeEndpoint, `Slug:    "capture",`, "Slug:    \"capture\",\n\t\t\t\tFuture:  true,", 1),
			"not derivable by this generator",
		},
		{
			"no handler",
			strings.Replace(wholeEndpoint, "\t\t\t\tHandle: receive,\n", "", 1),
			"declares no Handle",
		},
		{
			"a body cap over the published ceiling",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 8 << 20`, 1),
			"over the",
		},
		{
			"no body cap",
			strings.Replace(wholeEndpoint, "\t\t\t\tMaxBody: 64 << 10,\n", "", 1),
			"no default",
		},
		{
			"a skew that is not whole seconds",
			strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   1500 * time.Millisecond`, 1),
			"whole seconds",
		},
		{
			"a skew naming something that is not a time unit",
			strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   5 * time.Fortnight`, 1),
			"only the time package's units",
		},
		{
			"a cap built from an identifier rather than a literal",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: someConst`, 1),
			"integer literal",
		},
		{
			"a rate window written as a bare number",
			strings.Replace(wholeEndpoint, `Window: time.Minute}`, `Window: 60}`, 1),
			"N * time.Unit",
		},
		{
			"an unmetered endpoint",
			strings.Replace(wholeEndpoint, `PerIP:       extension.Rate{Limit: 60, Window: time.Minute},`, `PerIP:       extension.Rate{Limit: 0, Window: time.Minute},`, 1),
			"per-IP allowance",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := deriveSynthetic(t, "x", inboundUnitSource(tc.entries))
			if err == nil {
				t.Fatalf("the generator derived %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refusal said %q, which does not carry %q", err, tc.want)
			}
		})
	}
}

// Both spellings of a duration are the same declaration, and a generator that
// read one and refused the other would be a style rule wearing a grammar's
// clothes.
func TestInboundReadsEitherOrderOfADuration(t *testing.T) {
	reversed := strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   time.Minute * 5`, 1)
	derived, err := deriveSynthetic(t, "x", inboundUnitSource(reversed))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(derived), `"skew_seconds": 300`) {
		t.Errorf("time.Minute * 5 did not derive as 300 seconds:\n%s", derived)
	}
}
