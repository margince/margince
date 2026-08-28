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
			// Unlike a Tool's or a Job's, an inbound endpoint has no inert
			// form — every declared edge mounts and must serve. A bare
			// `Handle: nil` must be refused HERE, at generation, rather than
			// merely counted as "present" and left for boot's own Validate
			// to refuse in a binary that already shipped.
			"a nil handler",
			strings.Replace(wholeEndpoint, "Handle: receive,", "Handle: nil,", 1),
			"has no inert form",
		},
		{
			"a nil handler spelled through the published conversion",
			strings.Replace(wholeEndpoint, "Handle: receive,", "Handle: extension.InboundHandler(nil),", 1),
			"has no inert form",
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

// The refusals above are about VALUES; these are about SHAPE. A generator that
// reads a declaration statically has no compiler behind it, so every shape it
// cannot read has to be refused by name rather than skipped — a skipped field
// is an endpoint published without the bound it asked for, and nothing
// downstream can tell that from an endpoint that asked for none.
func TestInboundDerivationRefusesAShapeItCannotRead(t *testing.T) {
	tests := []struct {
		name    string
		entries string
		want    string
	}{
		{
			"an entry that is not a literal",
			"\t\t\tspareEndpoint,\n",
			"must be an extension.InboundEndpoint literal",
		},
		{
			"an endpoint field given positionally",
			"\t\t\t{\"capture\", \"inbound\"},\n",
			"InboundEndpoint fields must be keyed",
		},
		{
			"an endpoint field keyed by something other than a name",
			strings.Replace(wholeEndpoint, `Slug:    "capture",`, `"Slug": "capture",`, 1),
			"must be keyed by name",
		},
		{
			"a slug built from an identifier rather than a literal",
			strings.Replace(wholeEndpoint, `Slug:    "capture",`, `Slug:    slugConst,`, 1),
			"InboundEndpoint.Slug",
		},
		{
			"a rate that is not a literal",
			replaceRate(`spareRate`),
			"must be an extension.InboundRate literal",
		},
		{
			"a rate bucket given positionally",
			replaceRate(`extension.InboundRate{extension.Rate{Limit: 60, Window: time.Minute}}`),
			"InboundRate fields must be keyed",
		},
		{
			"a rate field this generator cannot derive",
			strings.Replace(wholeEndpoint, `PerIP:       extension.Rate{Limit: 60, Window: time.Minute},`,
				"PerIP:       extension.Rate{Limit: 60, Window: time.Minute},\n\t\t\t\t\tPerMember:   extension.Rate{Limit: 1, Window: time.Minute},", 1),
			"InboundRate field PerMember is not derivable",
		},
		{
			"a bucket that is not a literal",
			strings.Replace(wholeEndpoint, `PerIP:       extension.Rate{Limit: 60, Window: time.Minute},`, `PerIP:       spareBucket,`, 1),
			"InboundRate.PerIP must be an extension.Rate literal",
		},
		{
			"a bucket field given positionally",
			strings.Replace(wholeEndpoint, `extension.Rate{Limit: 60, Window: time.Minute}`, `extension.Rate{60, time.Minute}`, 1),
			"InboundRate.PerIP fields must be keyed",
		},
		{
			"a bucket field this generator cannot derive",
			strings.Replace(wholeEndpoint, `extension.Rate{Limit: 60, Window: time.Minute}`, `extension.Rate{Limit: 60, Window: time.Minute, Burst: 5}`, 1),
			"InboundRate.PerIP field Burst is not derivable",
		},
		{
			// A shift Go itself accepts can still wrap int64 negative, and a
			// negative cap reads to Validate as "no cap declared" — the exact
			// value an anonymous edge must never be published with.
			"a body cap whose shift leaves int64",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 64 << 70`, 1),
			"shifts out of range",
		},
		{
			"a body cap using an operator outside the grammar",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 65536 - 1`, 1),
			"reads only <<, * and +",
		},
		{
			// The same wraparound the shift guard exists for, reached
			// through * instead: both operands fit int64 on their own, but
			// their product does not, and an unchecked int64 multiply wraps
			// silently rather than failing the way the real constant would.
			"a body cap whose multiplication overflows int64",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 4611686018427387904 * 4`, 1),
			"overflows the 64-bit range",
		},
		{
			"a body cap whose addition overflows int64",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: 9223372036854775807 + 1`, 1),
			"overflows the 64-bit range",
		},
		{
			"a body cap written as a string",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: "64k"`, 1),
			"must be an integer literal",
		},
		{
			"a body cap that is not arithmetic at all",
			strings.Replace(wholeEndpoint, `MaxBody: 64 << 10`, `MaxBody: -someCall()`, 1),
			"arithmetic expression over literals",
		},
		{
			"a skew added rather than multiplied",
			strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   time.Minute + time.Minute`, 1),
			"must be written as N * time.Unit",
		},
		{
			"a skew multiplying two numbers",
			strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   5 * 60`, 1),
			"must multiply a literal by a time unit",
		},
		{
			"a skew naming a constant of another package",
			strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   window.Minute`, 1),
			"only the time package's units",
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

// replaceRate swaps the whole Rate value for one this generator should refuse.
func replaceRate(with string) string {
	const rate = `Rate: extension.InboundRate{
					PerIP:       extension.Rate{Limit: 60, Window: time.Minute},
					PerEndpoint: extension.Rate{Limit: 120, Window: time.Minute},
				},`
	return strings.Replace(wholeEndpoint, rate, "Rate: "+with+",", 1)
}

// A bare unit and a parenthesised expression are the same declarations as `N *
// time.Unit` and `64 << 10`; refusing either would be a style rule wearing a
// grammar's clothes, and the author's cheapest workaround — dropping the bound
// — is the one outcome an anonymous edge cannot afford.
func TestInboundReadsABareUnitAndParentheses(t *testing.T) {
	tests := []struct {
		name string
		skew string
		want string
	}{
		{"a bare time unit", `time.Minute`, `"skew_seconds": 60`},
		{"a parenthesised product", `(5 * time.Minute)`, `"skew_seconds": 300`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries := strings.Replace(wholeEndpoint, `Skew:   5 * time.Minute`, `Skew:   `+tc.skew, 1)
			entries = strings.Replace(entries, `MaxBody: 64 << 10`, `MaxBody: (64 << 10)`, 1)
			derived, err := deriveSynthetic(t, "x", inboundUnitSource(entries))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.want, `"max_body": 65536`} {
				if !strings.Contains(string(derived), want) {
					t.Errorf("%s did not derive %s:\n%s", tc.skew, want, derived)
				}
			}
		})
	}
}

// An Inbound field the generator cannot see into is refused rather than read as
// "no anonymous edge" — the manifest is the operator's only census of the
// installation's unauthenticated surface, and a silently empty census is the
// one failure that looks exactly like a clean one.
func TestInboundRefusesAnIndirectSlice(t *testing.T) {
	source := strings.Replace(inboundUnitSource(""),
		"Inbound: []extension.InboundEndpoint{\n\n\t\t},", "Inbound: declaredEndpoints,", 1)
	_, err := deriveSynthetic(t, "x", source)
	if err == nil {
		t.Fatal("the generator read an Inbound field it cannot see into")
	}
	if !strings.Contains(err.Error(), "must be a slice literal") {
		t.Fatalf("refusal said %q, which does not name the shape", err)
	}
}
