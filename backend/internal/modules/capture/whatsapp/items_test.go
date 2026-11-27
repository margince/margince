// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func splitFixture(t *testing.T, name string) Split {
	t.Helper()
	out, err := SplitItems(fixture(t, name))
	if err != nil {
		t.Fatalf("SplitItems(%s): %v", name, err)
	}
	return out
}

func TestSplitItemsKeepsTheGoodItemsOfAMixedBatch(t *testing.T) {
	out := splitFixture(t, "mixed_batch.json")
	var keys []string
	for _, item := range out.Items {
		keys = append(keys, item.PhoneNumberID+" "+item.Key)
	}
	want := []string{"PNID1 inbound:wamid.OK1", "PNID1 inbound:wamid.OK2", "PNID2 outbound:wamid.ECHO3"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("items = %v, want %v", keys, want)
	}
	var paths []string
	for _, e := range out.Errors {
		paths = append(paths, e.Path)
		if !errors.Is(e.Err, ErrMalformedItem) {
			t.Errorf("item error %s cause = %v", e.Path, e.Err)
		}
	}
	wantPaths := []string{
		"entry[0].changes[0].value.messages[1]",
		"entry[0].changes[0].value.messages[2]",
		"entry[0].changes[2].value",
		"entry[1]",
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("error paths = %v, want %v", paths, wantPaths)
	}
	if len(out.UnhandledFields) != 3 {
		t.Errorf("unhandled = %v, want 3 fields", out.UnhandledFields)
	}
}

func TestSplitItemBodyDecodesToTheSameMessage(t *testing.T) {
	whole := decodeFixture(t, "inbound_text.json")
	out := splitFixture(t, "inbound_text.json")
	if len(out.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(out.Items))
	}
	again, err := Decode(out.Items[0].Body)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Messages, whole.Messages) {
		t.Errorf("decoded item = %+v, want %+v", again.Messages, whole.Messages)
	}
}

func TestSplitStatusKeysSeparateTheStatesOfOneMessage(t *testing.T) {
	out := splitFixture(t, "statuses.json")
	var keys []string
	for _, item := range out.Items {
		keys = append(keys, item.Key)
		again, err := Decode(item.Body)
		if err != nil || len(again.Statuses) != 1 {
			t.Fatalf("item %s decodes to %+v, %v", item.Key, again, err)
		}
	}
	want := []string{"status:wamid.OUT1:delivered", "status:wamid.OUT2:failed"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestSplitItemKeepsOnlyTheCustomersContact(t *testing.T) {
	body := `{"object":"whatsapp_business_account","entry":[{"id":"W","changes":[{"field":"messages","value":{
	"metadata":{"display_phone_number":"1","phone_number_id":"P"},
	"contacts":[{"profile":{"name":"Ada"},"wa_id":"491701234567"},{"profile":{"name":"Bob"},"wa_id":"4930999"}],
	"messages":[{"from":"491701234567","id":"wamid.A","timestamp":"1764000300","type":"text","text":{"body":"hi"}}]}}]}]}`
	out, err := SplitItems([]byte(body))
	if err != nil || len(out.Items) != 1 {
		t.Fatalf("SplitItems = %+v, %v", out, err)
	}
	stored := string(out.Items[0].Body)
	if !strings.Contains(stored, "Ada") || strings.Contains(stored, "Bob") || strings.Contains(stored, "4930999") {
		t.Errorf("stored item = %s, want Ada's contact only", stored)
	}
}

func TestSplitItemsRejectsAnotherPayload(t *testing.T) {
	for _, body := range []string{`{"object":"page","entry":[]}`, `nope`} {
		if _, err := SplitItems([]byte(body)); !errors.Is(err, ErrMalformedPayload) {
			t.Errorf("SplitItems(%q) error = %v, want ErrMalformedPayload", body, err)
		}
	}
}

func TestCustomerIdentityPrefersThePhoneAndScopesABSUID(t *testing.T) {
	cases := []struct {
		name              string
		msg               Message
		wantScope, wantID string
	}{
		{"phone", Message{WABAID: "W", CounterpartyPhone: "4917", CounterpartyUserID: "B"}, "", "4917"},
		{"bsuid only", Message{WABAID: "W", CounterpartyUserID: "B"}, "W", "B"},
		{"neither", Message{WABAID: "W"}, "", ""},
	}
	for _, tc := range cases {
		if scope, id := CustomerIdentity(tc.msg); scope != tc.wantScope || id != tc.wantID {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tc.name, scope, id, tc.wantScope, tc.wantID)
		}
	}
}
