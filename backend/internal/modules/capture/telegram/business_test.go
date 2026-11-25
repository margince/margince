// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

import (
	"strings"
	"testing"
)

func TestParseBusinessConnectionDecodesTheOwnerAndTheLink(t *testing.T) {
	update := `{"update_id":7,"business_connection":{"id":"bc-1","user":{"id":4001,"first_name":"Mia","last_name":"Owner","username":"mia"},"user_chat_id":4001,"date":1785000000,"is_enabled":true}}`

	got, isBusiness, err := ParseBusinessConnection([]byte(update))
	if err != nil || !isBusiness {
		t.Fatalf("ParseBusinessConnection = %v, %v, want a decoded business connection", isBusiness, err)
	}
	want := BusinessConnection{ID: "bc-1", OwnerID: "4001", OwnerLabel: "Mia Owner", Enabled: true}
	if got != want {
		t.Errorf("decoded %+v, want %+v", got, want)
	}
}

func TestParseBusinessConnectionLabelsTheOwnerByTheBestNameItHas(t *testing.T) {
	for name, user := range map[string]string{
		"@mia": `{"id":4001,"username":"mia"}`,
		"4001": `{"id":4001}`,
	} {
		update := `{"business_connection":{"id":"bc-1","user":` + user + `,"is_enabled":false}}`
		got, _, err := ParseBusinessConnection([]byte(update))
		if err != nil || got.OwnerLabel != name || got.Enabled {
			t.Errorf("user %s: label %q enabled=%v err=%v, want label %q and a disabled link", user, got.OwnerLabel, got.Enabled, err, name)
		}
	}
	long := `{"business_connection":{"id":"bc-1","user":{"id":4001,"first_name":"` + strings.Repeat("é", 300) + `"},"is_enabled":true}}`
	if got, _, _ := ParseBusinessConnection([]byte(long)); len([]rune(got.OwnerLabel)) != businessLabelLimit {
		t.Errorf("a %d character name was kept at %d characters, want it cut to the %d the column holds",
			300, len([]rune(got.OwnerLabel)), businessLabelLimit)
	}
}

func TestParseBusinessConnectionLeavesOtherUpdatesAlone(t *testing.T) {
	if _, isBusiness, err := ParseBusinessConnection([]byte(`{"update_id":8,"message":{"text":"hi"}}`)); isBusiness || err != nil {
		t.Errorf("a message was read as a business connection (%v, %v)", isBusiness, err)
	}
}

func TestParseBusinessConnectionRefusesALinkWithoutAnAccount(t *testing.T) {
	for _, update := range []string{
		`{"business_connection":{"id":"","user":{"id":4001},"is_enabled":true}}`,
		`{"business_connection":{"id":"bc-1","user":{"id":0},"is_enabled":true}}`,
		`{"business_connection":{"id":"bc-1","is_enabled":true}}`,
	} {
		if _, isBusiness, err := ParseBusinessConnection([]byte(update)); err == nil || !isBusiness {
			t.Errorf("%s was accepted (%v, %v): a row keyed on no account would be shared by every such update", update, isBusiness, err)
		}
	}
}

func TestABusinessConnectionNamesNoSubject(t *testing.T) {
	update := `{"update_id":9,"business_connection":{"id":"bc-1","user":{"id":4001},"is_enabled":true}}`
	if subjects, err := InScopeSubjects([]byte(update)); err != nil || len(subjects) != 0 {
		t.Errorf("subjects = %v, %v, want none: the link names no customer to store", subjects, err)
	}
}
