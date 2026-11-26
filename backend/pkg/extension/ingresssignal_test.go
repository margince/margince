// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func aSignalRecord() extension.Record {
	return extension.Record{
		System: "crm-x", Key: "event-1",
		Signal: &extension.SignalFields{
			Kind:    "loss",
			Subject: extension.SignalSubject{Type: "company", ID: "0191a000-0000-7000-8000-000000000001"},
		},
	}
}

func TestASignalRecordNamesItsKindAndIsLandable(t *testing.T) {
	rec := aSignalRecord()
	if rec.Kind() != extension.KindSignal {
		t.Fatalf("Kind() = %q, want %q", rec.Kind(), extension.KindSignal)
	}
	if err := rec.Validate(); err != nil {
		t.Fatalf("a well-formed signal record was refused: %v", err)
	}
	source := extension.IngressSource{System: "crm-x", Lands: []extension.RecordKind{extension.KindSignal, extension.KindCompany}}
	if err := source.Validate(); err != nil {
		t.Errorf("a source landing signals was refused: %v", err)
	}
	if err := (extension.IngressSource{System: "crm-x", Lands: []extension.RecordKind{"contact"}}).Validate(); err == nil {
		t.Error("a source landing an unsupported kind was accepted")
	}
}

func TestASignalRecordThatBreaksTheShapeIsRefused(t *testing.T) {
	badKind, noSubject, badID, withDeal := aSignalRecord(), aSignalRecord(), aSignalRecord(), aSignalRecord()
	badKind.Signal.Kind = "Not A Kind"
	noSubject.Signal.Subject.Type = ""
	badID.Signal.Subject.ID = "not-a-uuid"
	withDeal.Deal = aDealRecord().Deal
	for name, rec := range map[string]extension.Record{
		"an invalid signal type": badKind, "no subject type": noSubject, "a subject id that is no UUID": badID, "a deal beside it": withDeal,
	} {
		if err := rec.Validate(); err == nil {
			t.Errorf("%s: the record was accepted", name)
		}
	}
}
