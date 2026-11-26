// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"regexp"
	"strings"
	"testing"
)

func TestEveryBuilderSpellsTheOneQualifiedForm(t *testing.T) {
	const unit Name = "acme-crm"
	want := "ext:acme-crm:visit"
	for name, got := range map[string]string{
		"RecordKindName":   RecordKindName(unit, "visit"),
		"SignalKindName":   SignalKindName(unit, "visit"),
		"ReportName":       ReportName(unit, "visit"),
		"LinkTypeValue":    LinkTypeValue(unit, "visit"),
		"CompanyTypeValue": CompanyTypeValue(unit, "visit"),
	} {
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestTheQualifiedPatternTakesWhatTheValidatorsTake(t *testing.T) {
	pattern := regexp.MustCompile(QualifiedNamePattern)
	longest := "v" + strings.Repeat("x", maxShortNameLength-1)
	for _, short := range []string{"visit", "site_visit", "v2", longest} {
		if err := validRecordKindName(short); err != nil {
			t.Fatalf("the validator refused %q: %v", short, err)
		}
		if name := QualifiedName("acme", short); !pattern.MatchString(name) {
			t.Errorf("QualifiedNamePattern refuses %q, which the validator accepts", name)
		}
	}
	for _, name := range []string{
		"ext::visit",
		"ext:acme:Visit",
		"ext:acme:2visit",
		"ext:acme:" + longest + "x",
		"ext:Acme:visit",
		"acme:visit",
	} {
		if pattern.MatchString(name) {
			t.Errorf("QualifiedNamePattern accepts %q", name)
		}
	}
}

func TestTheCompanyTypePatternKeepsItsOwnShortRule(t *testing.T) {
	pattern := regexp.MustCompile(CompanyTypeNamePattern)
	for _, typ := range []string{"reseller", "2nd_tier", "a_b_c"} {
		if err := (CompanyType{Type: typ, Label: "x"}).Validate(); err != nil {
			t.Fatalf("the validator refused %q: %v", typ, err)
		}
		if name := CompanyTypeValue("acme", typ); !pattern.MatchString(name) {
			t.Errorf("CompanyTypeNamePattern refuses %q, which the validator accepts", name)
		}
	}
	for _, name := range []string{"ext:acme:a__b", "ext:acme:_a", "ext:acme:Reseller"} {
		if pattern.MatchString(name) {
			t.Errorf("CompanyTypeNamePattern accepts %q", name)
		}
	}
}

func TestTheUnitPartOfThePatternsIsTheUnitNameRule(t *testing.T) {
	for _, unit := range []string{"acme", "acme-crm", "1crm"} {
		if err := Name(unit).Validate(); err != nil {
			t.Fatalf("Name(%q).Validate() = %v", unit, err)
		}
		if !regexp.MustCompile(QualifiedNamePattern).MatchString(QualifiedName(Name(unit), "visit")) {
			t.Errorf("QualifiedNamePattern refuses the valid unit %q", unit)
		}
	}
}
