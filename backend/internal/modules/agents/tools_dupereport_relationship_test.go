// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEmploymentRelationshipCreateReportsTheContactsPairs(t *testing.T) {
	contact := ids.NewV7()
	edge := ids.NewV7()
	cases := []struct {
		name       string
		recordType string
		fields     string
		wantType   string
		wantID     ids.UUID
	}{
		{"employment edge reports its contact", "relationship",
			`{"kind":"employment","contact_id":"` + contact.String() + `"}`, "contact", contact},
		{"other edge kinds report nothing", "relationship",
			`{"kind":"works_with","contact_id":"` + contact.String() + `"}`, "", ids.UUID{}},
		{"a contact create reports itself", "contact", `{"full_name":"A"}`, "contact", edge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotType string
			var gotID ids.UUID
			tool := createRecord{duplicates: func(_ context.Context, rt string, id ids.UUID) ([]DuplicateCandidate, error) {
				gotType, gotID = rt, id
				return []DuplicateCandidate{{CandidateID: "c", OtherRecordID: ids.NewV7().String()}}, nil
			}}
			found := tool.reportCreateDuplicates(context.Background(), tc.recordType, json.RawMessage(tc.fields), edge)
			if gotType != tc.wantType || gotID != tc.wantID {
				t.Fatalf("asked for %s %s, want %s %s", gotType, gotID, tc.wantType, tc.wantID)
			}
			if (tc.wantType != "") != (len(found) == 1) {
				t.Fatalf("found %d candidates for %q", len(found), tc.name)
			}
		})
	}
}

func TestCreateRecordCopyTellsAnAgentToAskAboutTheSameEmployerContact(t *testing.T) {
	if strings.Contains(createRecordCopy.Instead, "create it") {
		t.Fatal("create_record copy still says to create a same-name contact")
	}
	if !strings.Contains(createRecordCopy.Instead, "same employer, ask the human whether it is the same contact") {
		t.Fatalf("copy does not say to ask the human: %s", createRecordCopy.Instead)
	}
}
