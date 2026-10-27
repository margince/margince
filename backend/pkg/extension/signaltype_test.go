// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "testing"

func validSignalType() SignalTypeDeclaration {
	return SignalTypeDeclaration{
		Kind:  "loss",
		Label: "Loss reported",
		Evidence: []SignalEvidenceField{
			{Name: "amount", Label: "Amount", Type: SignalFieldNumber, Required: true},
			{Name: "reporter", Label: "Reporter", Type: SignalFieldText, Personal: true},
		},
	}
}

func TestSignalKindNameNamesTheDeclaringUnit(t *testing.T) {
	if got := SignalKindName("visit-log", "loss"); got != "ext:visit-log:loss" {
		t.Errorf("SignalKindName = %q", got)
	}
}

func TestSignalTypeValidate(t *testing.T) {
	mutate := func(f func(*SignalTypeDeclaration)) SignalTypeDeclaration {
		d := validSignalType()
		d.Evidence = append([]SignalEvidenceField(nil), d.Evidence...)
		f(&d)
		return d
	}
	cases := map[string]struct {
		decl SignalTypeDeclaration
		ok   bool
	}{
		"valid":           {validSignalType(), true},
		"bad kind":        {mutate(func(d *SignalTypeDeclaration) { d.Kind = "Loss" }), false},
		"no label":        {mutate(func(d *SignalTypeDeclaration) { d.Label = " " }), false},
		"bad field name":  {mutate(func(d *SignalTypeDeclaration) { d.Evidence[0].Name = "Amount" }), false},
		"duplicate field": {mutate(func(d *SignalTypeDeclaration) { d.Evidence[1].Name = "amount" }), false},
		"unknown type":    {mutate(func(d *SignalTypeDeclaration) { d.Evidence[0].Type = "blob" }), false},
		"field no label":  {mutate(func(d *SignalTypeDeclaration) { d.Evidence[0].Label = "" }), false},
	}
	for name, c := range cases {
		if err := c.decl.Validate(); (err == nil) != c.ok {
			t.Errorf("%s: Validate = %v, want ok=%v", name, err, c.ok)
		}
	}
}

func TestSignalTypeCheckFields(t *testing.T) {
	d := validSignalType()
	cases := map[string]struct {
		fields map[string]any
		ok     bool
	}{
		"complete":       {map[string]any{"amount": 12.5, "reporter": "A"}, true},
		"optional left":  {map[string]any{"amount": 3.0}, true},
		"required gone":  {map[string]any{"reporter": "A"}, false},
		"unknown member": {map[string]any{"amount": 1.0, "extra": "x"}, false},
		"wrong type":     {map[string]any{"amount": "12"}, false},
	}
	for name, c := range cases {
		if err := d.CheckFields(c.fields); (err == nil) != c.ok {
			t.Errorf("%s: CheckFields = %v, want ok=%v", name, err, c.ok)
		}
	}
}

func TestSignalTypePersonalFieldsListsOnlyFlaggedFields(t *testing.T) {
	got := validSignalType().PersonalFields()
	if len(got) != 1 || got[0] != "reporter" {
		t.Errorf("PersonalFields = %v, want [reporter]", got)
	}
}
