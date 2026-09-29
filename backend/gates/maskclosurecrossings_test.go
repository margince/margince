// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A fact withheld on another record as a CONSEQUENCE may not also be offered
// for configuration.
//
// auth's group closure takes a commission entry's rate and amount out with the
// partner's margin tier, because the tier is recoverable from the arithmetic.
// Offering those pairs in the maskable-field catalog as well would make one
// fact two switches: an operator sets the one on the ledger, reads the
// configuration back unchanged, and the tier goes on leaving the partner
// register.
//
// Crossings only. A consequence INSIDE one object is that object's own field —
// the deal's amount and its ARR each drag the other, and each is a figure an
// administrator configures in its own right.

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// offeredPair is one maskable-field catalog line: the RBAC object an
// administrator configures under, and the wire field a mask names.
type offeredPair struct{ object, field string }

func (p offeredPair) String() string { return p.object + " " + p.field }

// catalogPairs is everything the catalog offers, and the reader every gate over
// that catalog shares.
//
// It refuses rather than narrows: a catalog that is absent, unreadable or
// parses to nothing would leave each of those gates sweeping an empty subject
// and reporting the clean result it never obtained.
func catalogPairs(t *testing.T) []offeredPair {
	t.Helper()
	offered, err := catalogPairsIn(maskableCatalog)
	if err != nil {
		t.Fatalf("reading the maskable-field catalog: %v", err)
	}
	return offered
}

func catalogPairsIn(catalog string) ([]offeredPair, error) {
	body, err := os.ReadFile(catalog) // #nosec G304 -- a fixture path this package spells itself
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", catalog, err)
	}
	var offered []offeredPair
	for number, line := range strings.Split(string(body), "\n") {
		object, field, lineErr := catalogPair(line)
		if lineErr != nil {
			return nil, fmt.Errorf("%s line %d: %w", catalog, number+1, lineErr)
		}
		if object != "" {
			offered = append(offered, offeredPair{object: object, field: field})
		}
	}
	if len(offered) == 0 {
		return nil, fmt.Errorf("%s offers no pair at all", catalog)
	}
	return offered, nil
}

func TestNoCrossObjectConsequenceIsAlsoOfferedForConfiguration(t *testing.T) {
	t.Parallel()
	offered := make(map[string]bool)
	for _, pair := range catalogPairs(t) {
		offered[pair.String()] = true
	}
	twice, err := consequencesOfferedTwice(offered, auth.MaskGroupCrossings())
	if err != nil {
		t.Fatalf("reading auth's mask closure: %v", err)
	}
	for _, finding := range twice {
		t.Error(finding)
	}
}

// consequencesOfferedTwice reports every crossing consequence the catalog also
// offers, and refuses a closure that crosses nothing: a sweep over an empty
// closure agrees with a catalog offering every consequence there is.
func consequencesOfferedTwice(offered map[string]bool, crossings map[string][]string) ([]string, error) {
	if len(crossings) == 0 {
		return nil, fmt.Errorf("the closure crosses no object, so there is nothing to hold %s to", maskableCatalog)
	}
	var twice []string
	for configured, consequences := range crossings {
		for _, pair := range consequences {
			if offered[pair] {
				twice = append(twice, fmt.Sprintf("%s offers %q, which a mask on %q already withholds "+
					"as a consequence: one fact behind two switches, and an operator setting the "+
					"consequence leaves the record that owns the fact reading it", maskableCatalog, pair, configured))
			}
		}
	}
	slices.Sort(twice)
	return twice, nil
}

// TestTheCrossingCensusRefusesACorpusItCannotSweep holds the one direction this
// gate may not fail in, and the admissions that keep the refusal from being the
// whole answer.
//
// Both corpora are here: the catalog reader every mask gate shares, and the
// closure this one walks. Either coming back empty would sweep the same tree
// and report PASS with no assertion left to notice.
func TestTheCrossingCensusRefusesACorpusItCannotSweep(t *testing.T) {
	t.Parallel()
	t.Run("a closure crossing nothing", func(t *testing.T) {
		t.Parallel()
		if _, err := consequencesOfferedTwice(map[string]bool{"partner margin_tier": true}, nil); err == nil {
			t.Fatal("an empty closure swept clean rather than being refused")
		}
	})
	t.Run("a crossing the catalog also offers", func(t *testing.T) {
		t.Parallel()
		twice, err := consequencesOfferedTwice(
			map[string]bool{"commission rate_bps": true},
			map[string][]string{"partner margin_tier": {"commission rate_bps"}})
		if err != nil {
			t.Fatalf("sweeping the fixture closure: %v", err)
		}
		if len(twice) != 1 {
			t.Fatalf("a consequence the catalog also offers went unreported: %v", twice)
		}
	})
	t.Run("a crossing the catalog leaves alone", func(t *testing.T) {
		t.Parallel()
		twice, err := consequencesOfferedTwice(
			map[string]bool{"partner margin_tier": true},
			map[string][]string{"partner margin_tier": {"commission rate_bps"}})
		if err != nil {
			t.Fatalf("sweeping the fixture closure: %v", err)
		}
		if len(twice) != 0 {
			t.Fatalf("the closure's own configured pair was read as a consequence offered twice: %v", twice)
		}
	})
	for _, c := range []struct{ name, body string }{
		{"a catalog that is not there", ""},
		{"a catalog of comments alone", "# what this build can withhold\n\n"},
		{"a line that is not a pair", "deal amount.minor\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			catalog := writtenCatalog(t, c.body)
			if offered, err := catalogPairsIn(catalog); err == nil {
				t.Fatalf("this catalog read as %v rather than being refused", offered)
			}
		})
	}
	t.Run("a catalog naming two objects", func(t *testing.T) {
		t.Parallel()
		offered, err := catalogPairsIn(writtenCatalog(t, "# a comment\ndeal\tamount_minor\n  partner   margin_tier  \n"))
		if err != nil {
			t.Fatalf("reading the fixture catalog: %v", err)
		}
		want := []offeredPair{{object: "deal", field: "amount_minor"}, {object: "partner", field: "margin_tier"}}
		if !slices.Equal(offered, want) {
			t.Fatalf("read %v, want %v — a pair dropped here is a pair every mask gate stops holding", offered, want)
		}
	})
}

// writtenCatalog is a fixture catalog on disk, or a path holding none when the
// body is empty — which is the absent-catalog case.
func writtenCatalog(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/maskable_fields.txt"
	if body == "" {
		return path
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the catalog fixture: %v", err)
	}
	return path
}
