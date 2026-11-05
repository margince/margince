// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1
//go:build !integration

package gates

// A unit that takes an anonymous inbound webhook refuses a replay of one.
//
// `compose/extinbound.go` validates the signature and the timestamp skew, hands
// the nonce to the unit, and persists nothing. So the defence is the unit's: it
// must hold one row per nonce per endpoint, and refuse the second arrival.
// `openchannel` does, with `ext_openchannel_inbound_one_per_nonce`.
//
// Nothing derived that obligation before this gate. A new unit that forgot it
// would accept replays in silence. There is no error, no log line and no
// failing test, because a replayed request is byte-identical to the first.
//
// The corpus is the units whose manifest declares an inbound endpoint, so a
// unit added tomorrow is asked the same question. One unit declares one today.
// That is too small a corpus to show the rule catching anything, so the
// matcher is held by planted SQL as well.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// extTierDir is the enabled set: presence under it is the enablement.
const extTierDir = "../extensions"

// noncePair matches a UNIQUE over an endpoint column and a nonce column, in
// either order, as a table constraint or a standalone index.
var noncePair = regexp.MustCompile(`(?is)UNIQUE\s*\(\s*(?:[a-z_]*endpoint[a-z_]*\s*,\s*nonce|nonce\s*,\s*[a-z_]*endpoint[a-z_]*)\s*\)`)

// replayUnguarded is the unit that takes inbound traffic and holds no nonce,
// with the reason beside it.
var replayUnguarded = gatekit.Waive(map[string]string{})

// inboundManifest is the part of a unit's manifest this gate reads.
type inboundManifest struct {
	Inbound []struct {
		Slug string `json:"slug"`
	} `json:"inbound"`
}

func TestEveryInboundEndpointRefusesAReplay(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(extTierDir)
	if err != nil {
		t.Fatalf("reading the extension tier: %v", err)
	}
	var withInbound, allUnits []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		allUnits = append(allUnits, e.Name())
		raw, err := os.ReadFile(filepath.Join(extTierDir, e.Name(), "manifest.generated.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("reading %s's manifest: %v", e.Name(), err)
		}
		var m inboundManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("parsing %s's manifest: %v", e.Name(), err)
		}
		if len(m.Inbound) > 0 {
			withInbound = append(withInbound, e.Name())
		}
	}

	// Two floors. A tier that reads empty, or a corpus with no inbound unit in
	// it, means the walk stopped seeing the thing it judges.
	if len(allUnits) == 0 {
		t.Fatalf("%s holds no unit: the walk found nothing to ask", extTierDir)
	}
	if len(withInbound) == 0 {
		t.Fatalf("no unit under %s declares an inbound endpoint, and openchannel does: "+
			"the manifest read is broken rather than the tier being quiet", extTierDir)
	}

	for _, u := range withInbound {
		if replayUnguarded.Waived(t, u) {
			continue
		}
		if !unitHoldsNoncePair(t, u) {
			t.Errorf("%s declares an inbound endpoint and no migration of its own holds "+
				"UNIQUE over an endpoint column and nonce. The edge persists nothing, so a "+
				"replayed request would be accepted twice with no error and no log line. Hold "+
				"one row per nonce per endpoint, as openchannel's "+
				"ext_openchannel_inbound_one_per_nonce does", u)
		}
	}
	t.Logf("%d units, %d taking inbound traffic", len(allUnits), len(withInbound))
	replayUnguarded.AssertAllMatched(t)
}

// unitHoldsNoncePair reports whether any up migration of the unit declares the
// pair. Read off the unit's own migrations, because the obligation is the
// unit's: the core schema cannot carry it for a tier it does not know.
func unitHoldsNoncePair(t *testing.T, unit string) bool {
	t.Helper()
	dir := filepath.Join(extTierDir, unit, "migrations")
	files, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".up.sql") {
			continue
		}
		sql, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatalf("reading %s/%s: %v", unit, f.Name(), err)
		}
		if noncePair.Match(sql) {
			return true
		}
	}
	return false
}

// TestTheReplayMatcherAnswersPlantedSQL holds the matcher itself, because one
// compliant unit cannot show that the rule would catch a defect.
func TestTheReplayMatcherAnswersPlantedSQL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		sql  string
		want bool
	}{
		{"the pair as a table constraint", "CONSTRAINT x_one_per_nonce UNIQUE (endpoint_id, nonce)", true},
		{"the pair reversed", "UNIQUE (nonce, endpoint_id)", true},
		{"the pair across lines", "UNIQUE (\n\tendpoint_id,\n\tnonce\n)", true},
		{"a nonce column with no unique", "nonce text NOT NULL", false},
		{"unique on the endpoint alone", "UNIQUE (endpoint_id)", false},
		{"unique on the nonce alone", "UNIQUE (nonce)", false},
		{"a different pair", "UNIQUE (endpoint_id, received_at)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := noncePair.MatchString(tc.sql); got != tc.want {
				t.Errorf("matcher said %v for %q, want %v", got, tc.sql, tc.want)
			}
		})
	}
}
