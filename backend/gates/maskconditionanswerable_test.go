// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A mask is conditioned only where the condition can be ANSWERED.
//
// MaskOutsideWriteAuthority resolves through auth.WritableSubset, which needs
// an owner and a grant — things only a shareable record's rows carry. Named on
// anything else the condition never lifts, so an operator who asked for "hidden
// on the rows they cannot change" gets the field hidden on every row and no
// word saying so. The code fails closed already; what nothing held is the
// CATALOG, which is where such a pair becomes configurable in the first place.
//
// A crossing carries the second half of the same rule. Write authority is over
// the mask's OWN record, and "the partners you may write" cannot name which
// commission entries follow, so a group reaching another object may hang only
// off a mask no condition lifts.

import (
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// unconditionedOffers ratifies a catalog pair whose object cannot answer write
// authority, each saying what the total withholding buys.
//
// The default is over-recognition: an object added to the catalog owes this
// declaration until somebody judges it. That direction costs a sentence; the
// other one is an operator configuring per-row behaviour and getting none.
var unconditionedOffers = gatekit.Waive(map[string]string{
	"partner margin_tier": "the partner register carries no owner and no grant, " +
		"so write authority over a partner is not a question the database can answer and the tier " +
		"is withheld on every row whatever condition names it. contacts/partnerfieldmask.go builds " +
		"the pass with no authority resolver for exactly that reason, and the crossing onto the " +
		"commission entry rests on it: a per-row lift on the partner could not say which ledger " +
		"rows follow it",
})

func TestEveryOfferedPairSitsWhereItsConditionCanBeAnswered(t *testing.T) {
	t.Parallel()
	for _, offered := range catalogPairs(t) {
		if auth.MaskConditionAnswerable(offered.object) {
			continue
		}
		if unconditionedOffers.Waived(t, offered.String()) {
			continue
		}
		t.Errorf("%s offers %q, and write authority over %q is not a question that can be "+
			"answered: an operator conditioning the mask on it asked for the field hidden on the "+
			"rows they cannot change and gets it hidden on all of them. Declare why total "+
			"withholding is the right answer here, or take the pair out of the catalog",
			maskableCatalog, offered, offered.object)
	}
	unconditionedOffers.AssertAllMatched(t)
}

func TestACrossObjectGroupHangsOffAMaskNoConditionLifts(t *testing.T) {
	t.Parallel()
	liftable, err := crossingsOnLiftableObjects(auth.MaskGroupCrossings(), auth.MaskConditionAnswerable)
	if err != nil {
		t.Fatalf("reading auth's mask closure: %v", err)
	}
	for _, finding := range liftable {
		t.Error(finding)
	}
}

// crossingsOnLiftableObjects reports every group reaching another object from a
// pair whose own condition could lift, and refuses a closure that crosses
// nothing — a sweep over an empty closure agrees with any group at all.
func crossingsOnLiftableObjects(crossings map[string][]string, answerable func(string) bool) ([]string, error) {
	if len(crossings) == 0 {
		return nil, fmt.Errorf("the closure crosses no object, so this rule holds nothing")
	}
	var liftable []string
	for configured, consequences := range crossings {
		object, _, err := catalogPair(configured)
		if err != nil {
			return nil, fmt.Errorf("the closure spells a crossing this rule cannot read: %w", err)
		}
		if answerable(object) {
			liftable = append(liftable, fmt.Sprintf("a mask on %q can be conditioned on write "+
				"authority and withholds %v on another record: authority over one record says "+
				"nothing about which of another's rows follow, so the lift would hand back a fact "+
				"the mask was set to withhold", configured, consequences))
		}
	}
	slices.Sort(liftable)
	return liftable, nil
}

// TestTheConditionRuleRefusesAClosureItCannotSweep holds the direction this
// rule may not fail in, beside the readings that keep the refusal from being
// the whole answer. The catalog half of the corpus is refused by the reader
// both gates share, which TestTheCrossingCensusRefusesACorpusItCannotSweep
// drives.
func TestTheConditionRuleRefusesAClosureItCannotSweep(t *testing.T) {
	t.Parallel()
	answerable := func(object string) bool { return object == "deal" }
	for _, c := range []struct {
		name      string
		crossings map[string][]string
		want      int
	}{
		{name: "a closure crossing nothing"},
		{
			name:      "a crossing off an object whose rows answer write authority",
			crossings: map[string][]string{"deal amount_minor": {"commission amount_minor"}},
			want:      1,
		},
		{
			name:      "a crossing off an object whose rows cannot",
			crossings: map[string][]string{"partner margin_tier": {"commission rate_bps"}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			liftable, err := crossingsOnLiftableObjects(c.crossings, answerable)
			if len(c.crossings) == 0 {
				if err == nil {
					t.Fatal("an empty closure swept clean rather than being refused")
				}
				return
			}
			if err != nil {
				t.Fatalf("sweeping the fixture closure: %v", err)
			}
			if len(liftable) != c.want {
				t.Fatalf("reported %d finding(s), want %d: %v", len(liftable), c.want, liftable)
			}
		})
	}
}
