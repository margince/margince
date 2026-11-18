// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCompanyHistoryGroupsByContactAndNeverNamesAWithheldRow(t *testing.T) {
	pat, sam := ids.NewV7(), ids.NewV7()
	colleagues := []colleague{{ID: pat, Name: "Pat"}, {ID: sam, Name: "Sam"}}
	rows := []HistoryIn{
		{ID: "m1", Kind: "email", Subject: "Old terms", At: at(9)},
		{ID: "m2", Kind: "email", Subject: "Lunch", At: at(8)},
		{ID: "m3", Kind: "email", Subject: "Secret", At: at(7), Withheld: true},
		{ID: "m4", Kind: "note", Subject: "Kickoff", At: at(6)},
		{ID: "m5", Kind: "email", Subject: "Third topic", At: at(5)},
	}
	whose := map[string]ids.UUID{"m1": pat, "m2": sam, "m3": sam, "m4": pat, "m5": pat}
	got := foldCompanyHistory(rows, colleagues, whose)

	var order []string
	for _, topic := range got {
		order = append(order, topic.Contact+":"+topic.ID)
	}
	want := "Pat:m1 Pat:m4 Sam:m2"
	if strings.Join(order, " ") != want {
		t.Errorf("topics = %v, want %q: grouped by contact, newest first, two per contact, no withheld row", order, want)
	}
	if withheldCount(rows) != 1 {
		t.Errorf("withheld = %d, want 1 for the omission", withheldCount(rows))
	}
}

func TestEarlierAtTheCompanyIsAnOwnCitedSentence(t *testing.T) {
	in := Input{
		Language: "en", Company: "Acme",
		CompanyHistory: []CompanyTopicIn{{ID: "m1", Contact: "Pat", Subject: "Old terms", At: Occurred{At: at(9)}}},
	}
	section := companyContextSection(in)
	if len(section) != 1 {
		t.Fatalf("sentences = %d, want 1", len(section))
	}
	if !strings.Contains(section[0].Text, "Acme") || !strings.Contains(section[0].Text, "Pat") || !strings.Contains(section[0].Text, "Old terms") {
		t.Errorf("text = %q, want company, contact and topic", section[0].Text)
	}
	if len(section[0].Evidence) != 1 || section[0].Evidence[0].EntityID != "m1" {
		t.Errorf("evidence = %+v, want the conversation", section[0].Evidence)
	}
	known := knownRecords(in)
	if _, ok := known[Evidence{EntityType: citeActivity, EntityID: "m1"}]; !ok {
		t.Error("the company conversation is not a citable record")
	}
	prompt := planPromptOf(in, Plan{})
	if len(prompt.EarlierAtCompany) != 1 || prompt.EarlierAtCompany[0].ActivityID != "m1" {
		t.Errorf("plan prompt earlier_at_company = %+v, want the topic", prompt.EarlierAtCompany)
	}
}
