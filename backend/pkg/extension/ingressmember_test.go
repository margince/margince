// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

const aRowID = "01a211a6-7d11-71d3-91c0-842a44b49592"

func TestAPinnedPostureIsOnlyEverNarrow(t *testing.T) {
	base := extension.IngressSource{System: "chat", Lands: []extension.RecordKind{extension.KindActivity}}
	for posture, ok := range map[extension.Posture]bool{
		"": true, extension.PostureClassified: true, extension.PostureHeld: true, "shared": false, "workspace": false,
	} {
		src := base
		src.PinnedPosture = posture
		if err := src.Validate(); (err == nil) != ok {
			t.Errorf("pin %q: %v, want accepted = %v", posture, err, ok)
		}
	}
	noActivity := extension.IngressSource{System: "deals", Lands: []extension.RecordKind{extension.KindDeal}, PinnedPosture: extension.PostureHeld}
	if err := noActivity.Validate(); err == nil {
		t.Error("a pin on a source that lands no activities was accepted")
	}
}

func TestAChosenSubjectIsAContactOrALeadByCanonicalID(t *testing.T) {
	for name, tc := range map[string]struct {
		subject *extension.ChosenSubject
		ok      bool
	}{
		"none":         {nil, true},
		"a contact":    {&extension.ChosenSubject{Kind: extension.SubjectContact, ID: aRowID}, true},
		"a lead":       {&extension.ChosenSubject{Kind: extension.SubjectLead, ID: aRowID}, true},
		"a company":    {&extension.ChosenSubject{Kind: "company", ID: aRowID}, false},
		"a bad id":     {&extension.ChosenSubject{Kind: extension.SubjectContact, ID: "not-an-id"}, false},
		"an empty one": {&extension.ChosenSubject{}, false},
	} {
		rec := aValidRecord()
		rec.ChosenSubject = tc.subject
		if err := rec.Validate(); (err == nil) != tc.ok {
			t.Errorf("%s: %v, want accepted = %v", name, err, tc.ok)
		}
	}
}

func TestPrecisionStatesHowWellATimeIsKnown(t *testing.T) {
	for name, tc := range map[string]struct {
		damage func(*extension.Record)
		want   extension.OccurredPrecision
		ok     bool
	}{
		"nothing stated":           {func(*extension.Record) {}, extension.PrecisionInstant, true},
		"a day":                    {func(r *extension.Record) { r.OccurredPrecision = extension.PrecisionDay }, extension.PrecisionDay, true},
		"the activity's date flag": {func(r *extension.Record) { r.Activity.DateOnly = true }, extension.PrecisionDay, true},
		"a date flag and an instant": {func(r *extension.Record) {
			r.Activity.DateOnly = true
			r.OccurredPrecision = extension.PrecisionInstant
		}, "", false},
		"unknown with no time": {func(r *extension.Record) {
			r.OccurredPrecision = extension.PrecisionUnknown
			r.Activity.OccurredAt = time.Time{}
		}, extension.PrecisionUnknown, true},
		"an instant with no time": {func(r *extension.Record) { r.Activity.OccurredAt = time.Time{} }, "", false},
		"a made-up precision":     {func(r *extension.Record) { r.OccurredPrecision = "hour" }, "", false},
	} {
		rec := aValidRecord()
		tc.damage(&rec)
		err := rec.Validate()
		if (err == nil) != tc.ok {
			t.Errorf("%s: %v, want accepted = %v", name, err, tc.ok)
		} else if tc.ok && rec.Precision() != tc.want {
			t.Errorf("%s: precision %q, want %q", name, rec.Precision(), tc.want)
		}
	}
}

func TestFilesAreHeldToTheInboundBounds(t *testing.T) {
	file := func(ordinal int) extension.InboundFile {
		return extension.InboundFile{Ordinal: ordinal, Filename: "a.txt", Body: []byte("x")}
	}
	tooMany := make([]extension.InboundFile, extension.MaxInboundFiles+1)
	for i := range tooMany {
		tooMany[i] = file(i)
	}
	for name, tc := range map[string]struct {
		parts []extension.InboundFile
		drops []extension.FileDrop
		ok    bool
	}{
		"one file":                   {[]extension.InboundFile{file(0)}, nil, true},
		"too many":                   {tooMany, nil, false},
		"a repeated ordinal":         {[]extension.InboundFile{file(1), file(1)}, nil, false},
		"a negative ordinal":         {[]extension.InboundFile{file(-1)}, nil, false},
		"an empty file":              {[]extension.InboundFile{{Ordinal: 0, Filename: "a"}}, nil, false},
		"a file over its cap":        {[]extension.InboundFile{{Ordinal: 0, Filename: "a", Body: make([]byte, extension.MaxInboundFileBytes+1)}}, nil, false},
		"a drop with a reason":       {nil, []extension.FileDrop{{Reason: "too_large", Count: 2}}, true},
		"a drop with no count":       {nil, []extension.FileDrop{{Reason: "too_large"}}, false},
		"a drop with no reason":      {nil, []extension.FileDrop{{Count: 1}}, false},
		"a drop reason over its cap": {nil, []extension.FileDrop{{Reason: strings.Repeat("r", extension.MaxDisplayNameRunes+1), Count: 1}}, false},
	} {
		rec := aValidRecord()
		rec.Parts, rec.PartDrops = tc.parts, tc.drops
		if err := rec.Validate(); (err == nil) != tc.ok {
			t.Errorf("%s: %v, want accepted = %v", name, err, tc.ok)
		}
	}
}

// A company or deal has no chat to file, so the new fields are refused there.
func TestTheChatFieldsBelongToAnActivityOnly(t *testing.T) {
	rec := extension.Record{System: "crm", Key: "c-1", Company: &extension.CompanyFields{DisplayName: "Acme"}}
	if err := rec.Validate(); err != nil {
		t.Fatalf("the fixture company was refused: %v", err)
	}
	rec.ChosenSubject = &extension.ChosenSubject{Kind: extension.SubjectContact, ID: aRowID}
	if err := rec.Validate(); err == nil {
		t.Error("a company carrying a chosen subject was accepted")
	}
}

func TestAStalePositionSaveIsAConflict(t *testing.T) {
	if !errors.Is(extension.ErrPositionConflict, extension.ErrConflict) {
		t.Error("ErrPositionConflict is not an ErrConflict, so a unit matching the class would miss it")
	}
}
