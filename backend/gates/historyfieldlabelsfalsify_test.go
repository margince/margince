// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

// The label census, falsified.
//
// It passes on this tree, which is the state it exists to hold — and a gate
// that passes says nothing about what it would catch. These plant each of the
// three findings it reports, and each of the four image shapes it has to
// follow to see a key at all. The image shapes are the half that rots: a
// writer moving its map one line further from the call is an ordinary edit,
// and a reader that stopped following it would report the site as unreadable
// rather than wrong — a finding, but the wrong one, and one a ratification
// would then silence.

import (
	"slices"
	"strings"
	"testing"
)

func TestAKeyNoMapNamesIsReported(t *testing.T) {
	t.Parallel()
	findings := labelFindings(
		map[string]string{"stops_carried": "consent/stopcarry.go:CarryStopsTx"},
		map[string]bool{"name": true}, map[string]bool{"note": true}, neverRatified)

	if !mentionsAll(findings, "stops_carried", "no map names it") {
		t.Errorf("an unlabelled key was not reported: %v", findings)
	}
}

func TestAWordNoWriterEmitsIsReported(t *testing.T) {
	t.Parallel()
	findings := labelFindings(
		map[string]string{"note": "consent/qualifyingevent.go:RecordTx"},
		map[string]bool{}, map[string]bool{"note": true, "retired_key": true}, neverRatified)

	if !mentionsAll(findings, "retired_key", "no projected audit image carries it") {
		t.Errorf("a stale label was not reported: %v", findings)
	}
}

// A key both maps claim shows the CONTRACT column's word, which is right only
// while the two mean the same thing. Unratified, it is a finding; ratified, it
// is not — and the second half is what stops the register being decorative.
func TestAKeyBothMapsClaimIsReportedUntilItIsRatified(t *testing.T) {
	t.Parallel()
	emitted := map[string]string{"source": "consent/withdrawalpress.go:StopForCredentialTx"}
	contract := map[string]bool{"source": true}

	findings := labelFindings(emitted, contract, map[string]bool{}, neverRatified)
	if !mentionsAll(findings, "source", "already claims") {
		t.Errorf("an unratified collision was not reported: %v", findings)
	}

	if ratified := labelFindings(emitted, contract, map[string]bool{}, alwaysRatified); len(ratified) != 0 {
		t.Errorf("a ratified collision was still reported: %v", ratified)
	}
}

// The four shapes an audit image takes, each planted as the source a writer
// would actually have written.
func TestEveryImageShapeTheTreeUsesIsFollowed(t *testing.T) {
	t.Parallel()
	for _, shape := range []struct {
		name, body string
		want       string
	}{
		{
			name: "spelled at the call",
			body: `func w() { _ = k.AuditEvent(ctx, tx, "update", "contact", 1, map[string]any{"spelled": 1}) }`,
			want: "spelled",
		},
		{
			name: "a local a few lines up",
			body: `func w() {
				after := map[string]any{"local": 1}
				_ = k.AuditEvent(ctx, tx, "update", "contact", 1, after)
			}`,
			want: "local",
		},
		{
			name: "a parameter its own callers fill",
			body: `func w(after map[string]any) {
				_ = k.AuditEvent(ctx, tx, "update", "contact", 1, after)
			}
			func caller() { w(map[string]any{"from_caller": 1}) }`,
			want: "from_caller",
		},
		{
			name: "a small builder's return",
			body: `func w() { _ = k.AuditEvent(ctx, tx, "update", "contact", 1, image()) }
			func image() map[string]any { return map[string]any{"from_builder": 1} }`,
			want: "from_builder",
		},
	} {
		t.Run(shape.name, func(t *testing.T) {
			t.Parallel()
			if keys := keysOfPlantedWriter(t, shape.body); !slices.Contains(keys, shape.want) {
				t.Errorf("the reader did not follow this image: read %v, want %q", keys, shape.want)
			}
		})
	}
}

// keysOfPlantedWriter runs the gate's own reader over one planted package,
// read through the census the gate reads.
func keysOfPlantedWriter(t *testing.T, body string) []string {
	t.Helper()
	census := fixtureCensus(t, fixtureSeed, map[string]string{"a.go": fixtureHeader + body})
	sites, _ := census.doorSites()
	var keys []string
	for _, site := range sites {
		keys = append(keys, site.imageKeys(t, callsWithin(site.use.pkg))...)
	}
	return keys
}

func neverRatified(string) bool  { return false }
func alwaysRatified(string) bool { return true }

func mentionsAll(findings []string, parts ...string) bool {
	for _, finding := range findings {
		matched := true
		for _, part := range parts {
			matched = matched && strings.Contains(finding, part)
		}
		if matched {
			return true
		}
	}
	return false
}
