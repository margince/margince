// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// twoContosos is a provider holding two companies named alike, and the refs
// the engine ranks for them.
func twoContosos() (*queryProbeProvider, datasource.EntityRef, datasource.EntityRef) {
	first, second := aCompany(), aCompany()
	named := func(ref datasource.EntityRef, name string) datasource.Record {
		return datasource.Record{Ref: ref, Fields: json.RawMessage(`{"name":"` + name + `"}`)}
	}
	return &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		first.ID: named(first, "Contoso GmbH"), second.ID: named(second, "Contoso Inc"),
	}}, first, second
}

func pickArgs() anchorArgs { return anchorArgs{RecordType: "company", RecordName: "Contoso"} }

func asking(answers chatAnswers) context.Context {
	return withChatAnswers(searchReader("company"), answers)
}

func TestAnAmbiguousNameIsAskedAboutInTheChatWithEachRecordsName(t *testing.T) {
	records, first, second := twoContosos()
	_, err := resolveAnchor(asking(chatAnswers{}), rankedHits(first, second).search, records, pickArgs())
	ask, ok := errors.AsType[*askInChatError](err)
	if !ok {
		t.Fatalf("err = %v, want the question", err)
	}
	body, err := json.Marshal(ask.question.Schema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			RecordID struct {
				OneOf []map[string]string `json:"oneOf"`
			} `json:"record_id"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatal(err)
	}
	choices := schema.Properties.RecordID.OneOf
	if len(choices) != 2 || choices[0]["title"] != "Contoso GmbH" || choices[1]["const"] != second.ID.String() {
		t.Errorf("the choices are %v, want both companies by name", choices)
	}
}

func TestThePickedRecordIsTheAnchor(t *testing.T) {
	records, first, second := twoContosos()
	answer := json.RawMessage(`{"action":"accept","content":{"record_id":"` + second.ID.String() + `"}}`)
	got, err := resolveAnchor(asking(chatAnswers{anchorPickKey: answer}), rankedHits(first, second).search, records, pickArgs())
	if err != nil || got != second.ID {
		t.Fatalf("resolved to %s, %v; want the picked %s", got, err, second.ID)
	}
}

func TestAPickThatWasNotOfferedIsAskedAgain(t *testing.T) {
	records, first, second := twoContosos()
	answer := json.RawMessage(`{"action":"accept","content":{"record_id":"` + ids.NewV7().String() + `"}}`)
	_, err := resolveAnchor(asking(chatAnswers{anchorPickKey: answer}), rankedHits(first, second).search, records, pickArgs())
	if _, ok := errors.AsType[*askInChatError](err); !ok {
		t.Errorf("an id outside the choices answered %v, want the question again", err)
	}
}

func TestARecordTheCallerCannotReadIsNoChoice(t *testing.T) {
	records, first, second := twoContosos()
	records.fail = map[ids.UUID]error{second.ID: apperrors.ErrNotFound}
	got, err := resolveAnchor(asking(chatAnswers{}), rankedHits(first, second).search, records, pickArgs())
	if err != nil || got != first.ID {
		t.Errorf("resolved to %s, %v; want the one readable record without asking", got, err)
	}
}
