// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/filedpairs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
		{
			"employment edge reports its contact", "relationship",
			`{"kind":"employment","contact_id":"` + contact.String() + `"}`, "contact", contact,
		},
		{
			"other edge kinds report nothing", "relationship",
			`{"kind":"works_with","contact_id":"` + contact.String() + `"}`, "",
			ids.UUID{},
		},
		{"a contact create reports itself", "contact", `{"full_name":"A"}`, "contact", edge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotType string
			var gotID ids.UUID
			tool := createRecord{duplicates: func(_ context.Context, rt string, id ids.UUID, _ []ids.UUID) ([]DuplicateCandidate, error) {
				gotType, gotID = rt, id
				return []DuplicateCandidate{{CandidateID: "c", OtherRecordID: ids.NewV7().String()}}, nil
			}}
			found := tool.reportCreateDuplicates(context.Background(), tc.recordType, json.RawMessage(tc.fields), edge, []ids.UUID{ids.NewV7()})
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

type edgeCreateProvider struct {
	stubRecordProvider
	filedByThisCreate ids.UUID
	concurrentFiler   func()
}

func (p edgeCreateProvider) Create(ctx context.Context, _ datasource.CreateInput) (datasource.EntityRef, error) {
	p.concurrentFiler()
	filedpairs.Note(ctx, p.filedByThisCreate)
	return datasource.EntityRef{Type: datasource.EntityRelationship, ID: ids.NewV7()}, nil
}

func TestEmploymentEdgeCreateReportsOnlyThePairsItNotedAsFiled(t *testing.T) {
	contact, mine, theirs := ids.NewV7(), ids.NewV7(), ids.NewV7()
	queue := map[ids.UUID]bool{}
	provider := edgeCreateProvider{
		rec:               stagedRecord(datasource.EntityRelationship, ids.NewV7(), true),
		filedByThisCreate: mine,
		concurrentFiler:   func() { queue[theirs] = true },
	}
	queue[mine] = true
	tool := createRecord{p: provider, duplicates: func(_ context.Context, _ string, _ ids.UUID, only []ids.UUID) ([]DuplicateCandidate, error) {
		var out []DuplicateCandidate
		for _, id := range only {
			if queue[id] {
				out = append(out, DuplicateCandidate{CandidateID: id.String()})
			}
		}
		return out, nil
	}}
	args := json.RawMessage(`{"record_type":"relationship","fields":{"kind":"employment","contact_id":"` + contact.String() + `"}}`)

	got, err := tool.Handle(context.Background(), args)
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var answer createdRecord
	if err := json.Unmarshal(got, &answer); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	if len(answer.DuplicateCandidates) != 1 || answer.DuplicateCandidates[0].CandidateID != mine.String() {
		t.Fatalf("reported %+v, want only %s", answer.DuplicateCandidates, mine)
	}
}
