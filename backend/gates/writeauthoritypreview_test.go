// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import "testing"

// Preview admissions ask for the eventual write grant while leaving every row unchanged.
func classifyReadOnlyPreviewAdmissions(t *testing.T, packages map[string]map[string]map[string]*writeAuthorityFn) {
	t.Helper()
	receivers := packages["internal/modules/contacts"]
	visible := visibleWriteAuthorityFns(receivers, "Store")
	purity := previewMutationClosure(receivers)
	for _, name := range []string{"PreviewCompanyRead", "PreviewColdStart", "PreviewMergeSurvivorship"} {
		if !readOnlyPreviewAdmission(purity, name) {
			t.Errorf("%s must retain its write admission and reach no mutation", name)
			continue
		}
		visible[name].previewOnly = true
	}
}

func readOnlyPreviewAdmission(functions map[string]*writeAuthorityFn, name string) bool {
	_, exists := functions[name]
	return exists && len(reachableWriteAuthority(functions, name, map[string]bool{}).requires) > 0 && !reachesMutation(functions, name, map[string]bool{})
}

func TestReadOnlyPreviewAdmissionsCannotExcuseAFutureWriter(t *testing.T) {
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
	receivers := map[string]map[string]*writeAuthorityFn{
		"Store":  {"preview": {requires: map[string]bool{"company:update": true}, calls: map[string]bool{"save": true}}},
		"Writer": {"save": {mutates: true}},
	}
	if readOnlyPreviewAdmission(previewMutationClosure(receivers), "preview") {
		t.Fatal("a mutation behind another receiver must refuse preview classification")
	}
}
