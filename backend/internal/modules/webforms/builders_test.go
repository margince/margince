// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"errors"
	"testing"
)

func valueOf(fields []Field, name string) string {
	for _, f := range fields {
		if f.Name == name {
			return f.Value
		}
	}
	return ""
}

func TestAWebflowSubmissionReadsEachFieldAndItsID(t *testing.T) {
	got, err := readBuilder(SenderWebflow, []byte(`{"triggerType":"form_submission","payload":{"id":"wf-9",
		"data":{"Email":"wim@visitor.example","Budget":12000,"Newsletter":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "wf-9" || valueOf(got.Fields, "Email") != "wim@visitor.example" ||
		valueOf(got.Fields, "Budget") != "12000" || valueOf(got.Fields, "Newsletter") != "true" {
		t.Fatalf("webflow payload = %+v", got)
	}
}

func TestATypeformAnswerReadsAsTextWhateverItsType(t *testing.T) {
	got, err := readBuilder(SenderTypeform, []byte(`{"event_id":"tf-1","form_response":{
		"definition":{"fields":[{"id":"a","title":"Seats"},{"id":"b","title":"Agree"},{"id":"c","title":"Plan"},{"id":"d","title":"Tools"}]},
		"answers":[
			{"type":"number","number":12,"field":{"id":"a"}},
			{"type":"boolean","boolean":false,"field":{"id":"b"}},
			{"type":"choice","choice":{"label":"Team"},"field":{"id":"c"}},
			{"type":"choices","choices":{"labels":["CRM","Mail"],"other":"Fax"},"field":{"id":"d"}},
			{"type":"phone_number","phone_number":"+4930123","field":{"id":"e"}},
			{"type":"text","text":"by ref","field":{"id":"f","ref":"note_ref"}}],
		"hidden":{"utm_source":"ad"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"Seats": "12", "Agree": "false", "Plan": "Team", "Tools": "CRM, Mail, Fax",
		fieldPhone: "+4930123", "note_ref": "by ref", "utm_source": "ad",
	} {
		if got := valueOf(got.Fields, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestATallyResponseNamesChosenOptionsByTheirText(t *testing.T) {
	got, err := readBuilder(SenderTally, []byte(`{"eventId":"ta-1","eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"","type":"INPUT_EMAIL","value":"tom@visitor.example"},
		{"key":"q2","label":"Phone","type":"INPUT_PHONE_NUMBER","value":"+4930999"},
		{"key":"q3","label":"","type":"INPUT_TEXT","value":"by key"},
		{"key":"q4","label":"Tools","type":"CHECKBOXES","value":["o1","o3"],
		 "options":[{"id":"o1","text":"CRM"},{"id":"o2","text":"Mail"},{"id":"o3","text":"Chat"}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		fieldEmail: "tom@visitor.example", fieldPhone: "+4930999", "q3": "by key", "Tools": "CRM, Chat",
	} {
		if got := valueOf(got.Fields, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got.ID != "ta-1" {
		t.Errorf("ID = %q, want the Tally event id", got.ID)
	}
}

func TestABuilderBodyOfAnotherEventIsUnreadable(t *testing.T) {
	for sender, body := range map[string]string{
		SenderWebflow:  `{"triggerType":"site_publish"}`,
		SenderTypeform: `{"form_response":{}}`,
		SenderTally:    `{"eventType":"FORM_CREATED"}`,
	} {
		if _, err := readBuilder(sender, []byte(body)); !errors.Is(err, errUnreadable) {
			t.Errorf("%s: err = %v, want the unreadable refusal", sender, err)
		}
	}
}
