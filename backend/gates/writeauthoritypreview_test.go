// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"fmt"
	"maps"
	"slices"
	"testing"
)

// Preview admissions ask for the eventual write grant while leaving every row unchanged.
func classifyReadOnlyPreviewAdmissions(t *testing.T, packages map[string]map[string]map[string]*writeAuthorityFn) {
	t.Helper()
	for _, group := range []struct {
		module string
		names  []string
	}{
		{"internal/modules/contacts", []string{"PreviewCompanyRead", "PreviewColdStart", "PreviewMergeSurvivorship", "PreviewLeadPromotionWithOptions", "promotionDealsPreview", "promotionCandidateCompany", "InspectMemberHandoffsTx"}},
		{"internal/modules/collections", []string{"PreviewTagMerge"}},
		{"internal/modules/deals", []string{"appendDealWriteScope"}},
		{"internal/modules/consent", []string{"mayLiftOn", "readableOverrideContact", "NoticeSourceReviewsTx", "OpenNoticeCasesDueSoonestTx"}},
	} {
		receivers := packages[group.module]
		visible := visibleWriteAuthorityFns(receivers, "Store")
		purity := previewMutationClosure(receivers)
		for _, name := range group.names {
			if !readOnlyPreviewAdmission(purity, name) {
				t.Errorf("%s:%s must retain its write admission and reach no mutation", group.module, name)
				continue
			}
			visible[name].previewOnly = true
		}
	}
}

func readOnlyPreviewAdmission(functions map[string]*writeAuthorityFn, name string) bool {
	_, exists := functions[name]
	return exists && len(reachableWriteAuthority(functions, name, map[string]bool{}).requires) > 0 && !reachesMutation(functions, name, map[string]bool{})
}

func TestReadOnlyPreviewAdmissionsCannotExcuseAFutureWriter(t *testing.T) {
	t.Parallel()
	for _, mutates := range []bool{false, true} {
		functions := map[string]*writeAuthorityFn{
			"preview": {requires: map[string]bool{"company:update": true}, calls: map[string]bool{"reader": true}},
			"reader":  {mutates: mutates},
		}
		if readOnlyPreviewAdmission(functions, "preview") == mutates {
			t.Errorf("mutation=%v: incorrect preview classification", mutates)
		}
		if readOnlyPreviewAdmission(functions, "missing") {
			t.Fatal("a missing preview must not leave a stale classification")
		}
	}
	if readOnlyPreviewAdmission(map[string]*writeAuthorityFn{"preview": {}}, "preview") {
		t.Fatal("a removed write admission must remove its classification")
	}
}

func TestPreviewAdmissionIncludesPackageFunctions(t *testing.T) {
	t.Parallel()
	receivers := map[string]map[string]*writeAuthorityFn{
		"Store": {"preview": {requires: map[string]bool{"company:update": true}, calls: map[string]bool{"reader": true}}},
		"":      {"reader": {calls: map[string]bool{"write": true}}, "write": {mutates: true}},
	}
	if readOnlyPreviewAdmission(visibleWriteAuthorityFns(receivers, "Store"), "preview") {
		t.Fatal("a mutation in a package function must refuse preview classification")
	}
}

func previewMutationClosure(receivers map[string]map[string]*writeAuthorityFn) map[string]*writeAuthorityFn {
	closure := map[string]*writeAuthorityFn{}
	for _, methods := range receivers {
		closure = visibleWriteAuthorityFns(map[string]map[string]*writeAuthorityFn{"": closure, "receiver": methods}, "receiver")
	}
	return closure
}

func TestPreviewAdmissionIncludesOtherReceiverMutations(t *testing.T) {
	t.Parallel()
	receivers := map[string]map[string]*writeAuthorityFn{
		"Store":  {"preview": {requires: map[string]bool{"company:update": true}, calls: map[string]bool{"save": true}}},
		"Writer": {"save": {mutates: true}},
	}
	if readOnlyPreviewAdmission(previewMutationClosure(receivers), "preview") {
		t.Fatal("a mutation behind another receiver must refuse preview classification")
	}
}

func TestPreviewClassificationSurvivesThePackageFunctionView(t *testing.T) {
	t.Parallel()
	pure := &writeAuthorityFn{previewOnly: true}
	functions := map[string]map[string]*writeAuthorityFn{"": {"preview": pure}, "Writer": {"preview": {}}}
	if !visibleWriteAuthorityFns(functions, "")["preview"].previewOnly {
		t.Fatal("the package function's own view dropped its proven read-only admission")
	}
	if visibleWriteAuthorityFns(functions, "Writer")["preview"].previewOnly {
		t.Fatal("a distinct writer inherited the package function's read-only classification")
	}
}

// cloneWriteAuthorityIndex is a deep copy of a shared index, for the gate that
// marks preview admissions on it.
func cloneWriteAuthorityIndex(index map[string]map[string]map[string]*writeAuthorityFn) map[string]map[string]map[string]*writeAuthorityFn {
	out := make(map[string]map[string]map[string]*writeAuthorityFn, len(index))
	for dir, byReceiver := range index {
		receivers := make(map[string]map[string]*writeAuthorityFn, len(byReceiver))
		for recv, fns := range byReceiver {
			copied := make(map[string]*writeAuthorityFn, len(fns))
			for name, fn := range fns {
				copied[name] = &writeAuthorityFn{
					probes: slices.Clone(fn.probes), requires: maps.Clone(fn.requires), mutates: fn.mutates,
					previewOnly: fn.previewOnly, calls: maps.Clone(fn.calls), writes: maps.Clone(fn.writes),
					dynamicWrite: slices.Clone(fn.dynamicWrite),
				}
			}
			receivers[recv] = copied
		}
		out[dir] = receivers
	}
	return out
}

// Marking a copy of the shared index leaves the shared index as it was.
func TestMarkingACopyOfTheWriteAuthorityIndexLeavesTheSharedOneUnchanged(t *testing.T) {
	t.Parallel()
	shared := readOnlyWriteAuthorityIndex(t)
	before := writeAuthorityIndexLines(shared)
	marked := 0
	for _, byReceiver := range cloneWriteAuthorityIndex(shared) {
		for _, fns := range byReceiver {
			for _, fn := range fns {
				fn.previewOnly, fn.mutates = true, true
				fn.probes = append(fn.probes, probeSite{fn: "planted"})
				fn.dynamicWrite = append(fn.dynamicWrite, "planted")
				for _, set := range []map[string]bool{fn.requires, fn.calls, fn.writes} {
					if set != nil {
						set["planted"] = true
					}
				}
				marked++
			}
		}
	}
	if marked == 0 {
		t.Fatal("the shared write-authority index holds no function, so this check marked nothing")
	}
	if after := writeAuthorityIndexLines(shared); !slices.Equal(before, after) {
		t.Errorf("marking a copy changed the shared write-authority index: %d entries before, %d after",
			len(before), len(after))
	}
}

// writeAuthorityIndexLines spells each function of an index on one line, its
// sets and the lines sorted.
func writeAuthorityIndexLines(index map[string]map[string]map[string]*writeAuthorityFn) []string {
	var lines []string
	for dir, byReceiver := range index {
		for recv, fns := range byReceiver {
			for name, fn := range fns {
				lines = append(lines, fmt.Sprintf("%s %s.%s %v %v %v %v %v %v %v", dir, recv, name, fn.previewOnly, fn.mutates,
					fn.probes, slices.Sorted(maps.Keys(fn.requires)), slices.Sorted(maps.Keys(fn.calls)),
					slices.Sorted(maps.Keys(fn.writes)), fn.dynamicWrite))
			}
		}
	}
	slices.Sort(lines)
	return lines
}
