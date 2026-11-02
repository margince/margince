// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestThreadTopicUsesProvenProjectTagsAndTheSharedPrefixCatalog(t *testing.T) {
	offer := sha256.Sum256([]byte("offer"))
	for _, subject := range []string{"Offer", "Re: [ERP] AW: WG: Offer", "[ERP] Fwd: Offer", "Re: [ERP] Fwd: [ERP] Offer"} {
		if got := threadBoundaryTopic(subject, []string{"ERP"}); got != hex.EncodeToString(offer[:]) {
			t.Errorf("%q: %q", subject, got)
		}
	}
	fyi := sha256.Sum256([]byte("[fyi] offer"))
	if got := threadBoundaryTopic("Re: [FYI] Offer", []string{"ERP"}); got != hex.EncodeToString(fyi[:]) {
		t.Fatal(got)
	}
	erp := sha256.Sum256([]byte("[erp] offer"))
	if got := threadBoundaryTopic("Re: [ERP] Offer", nil); got != hex.EncodeToString(erp[:]) {
		t.Fatal(got)
	}
	for _, subject := range []string{"", " ", "Re:", "AW: Re:"} {
		if got := threadBoundaryTopic(subject, nil); got != "" {
			t.Fatalf("blank topic %q retained %q", subject, got)
		}
	}
}

func TestThreadBoundaryEdgesConvergeWithoutCaptureOrderOrSenderClocks(t *testing.T) {
	cases := []struct {
		name                   string
		topics, parents, roots []string
	}{
		{"returning subject", []string{"x", "y", "x"}, []string{"", "a", "b"}, []string{"a", "b", "c"}},
		{"empty intermediary", []string{"x", "", "y"}, []string{"", "a", "b"}, []string{"a", "a", "c"}},
		{"unnamed branching root", []string{"", "y", "x"}, []string{"", "a", "a"}, []string{"a", "a", "c"}},
		{"one topic cycle", []string{"x", "x", "x"}, []string{"b", "c", "a"}, []string{"a", "a", "a"}},
		{"unnamed cycle", []string{"", "", ""}, []string{"b", "c", "a"}, []string{"a", "a", "a"}},
		{"cycle with boundary", []string{"x", "y", "x"}, []string{"b", "c", "a"}, []string{"a", "b", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
				states := make([]threadBoundaryState, 3)
				for i, topic := range tc.topics {
					states[i] = newThreadBoundaryState(string(rune('a'+i)), topic)
				}
				for range 3 {
					for _, i := range order {
						if parent := tc.parents[i]; parent != "" {
							at := int(parent[0] - 'a')
							states[i] = advanceThreadAncestor(states[i], states[at])
							states[at] = advanceThreadFrontier(states[at], states[i])
						}
					}
				}
				for i := range states {
					var parent *threadBoundaryState
					if key := tc.parents[i]; key != "" {
						p := states[int(key[0]-'a')]
						parent = &p
					}
					states[i] = states[i].rooted(parent)
				}
				for range 3 {
					for _, i := range order {
						if key := tc.parents[i]; key != "" {
							at := int(key[0] - 'a')
							states[i], states[at] = advanceThreadRoots(states[i], states[at])
						}
					}
				}
				for i, want := range tc.roots {
					if states[i].RootKey != want {
						t.Fatalf("order=%v node=%s root=%s want=%s", order, states[i].Key, states[i].RootKey, want)
					}
				}
			}
		})
	}
}
