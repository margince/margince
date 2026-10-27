// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A write's result names what it changed in a reader's words. The view a chat
// host draws for it has no other way to say it.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// wordsSeen records what a write asked to have named. It names each value by
// its JSON text, so a test can see which value reached which side.
type wordsSeen struct {
	current, changes map[string]json.RawMessage
	err              error
}

func (w *wordsSeen) words(_ context.Context, current, changes map[string]json.RawMessage) ([]crmcontracts.ApprovalPreviewChange, error) {
	w.current, w.changes = current, changes
	if w.err != nil {
		return nil, w.err
	}
	out := []crmcontracts.ApprovalPreviewChange{}
	for _, field := range slices.Sorted(maps.Keys(changes)) {
		out = append(out, crmcontracts.ApprovalPreviewChange{
			Field:  field,
			Before: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: string(current[field])},
			After:  crmcontracts.ApprovalPreviewValue{Kind: "text", Text: string(changes[field])},
		})
	}
	return out, nil
}

// patchingProvider holds one record and applies each patch to it, so a read
// after a write sees what the write left.
type patchingProvider struct {
	datasource.SystemOfRecordProvider
	record datasource.Record
}

func (p *patchingProvider) Read(context.Context, datasource.EntityRef) (datasource.Record, error) {
	return p.record, nil
}

func (p *patchingProvider) Update(_ context.Context, in datasource.UpdateInput) (datasource.EntityRef, error) {
	var fields, patch map[string]json.RawMessage
	if err := json.Unmarshal(p.record.Fields, &fields); err != nil {
		return datasource.EntityRef{}, err
	}
	raw, ok := in.Patch.(json.RawMessage)
	if !ok {
		return datasource.EntityRef{}, errors.New("patch is not raw JSON")
	}
	if err := json.Unmarshal(raw, &patch); err != nil {
		return datasource.EntityRef{}, err
	}
	maps.Copy(fields, patch)
	merged, err := json.Marshal(fields)
	if err != nil {
		return datasource.EntityRef{}, err
	}
	p.record.Fields = merged
	p.record.Version++
	return in.Ref, nil
}

func contactWith(fields string) *patchingProvider {
	return &patchingProvider{record: nativeRecord(datasource.Record{
		Ref:     datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()},
		Fields:  json.RawMessage(fields),
		Version: 3,
	})}
}

// promoteToCTO is the patch every test here sends: one field, title.
func promoteToCTO(t *testing.T, p *patchingProvider) json.RawMessage {
	t.Helper()
	in, err := json.Marshal(map[string]any{
		"record_type": "contact", "id": p.record.Ref.ID, "fields": map[string]any{"title": "CTO"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func decodeUpdate(t *testing.T, raw json.RawMessage) UpdateResult {
	t.Helper()
	var got UpdateResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decoding %s: %v", raw, err)
	}
	return got
}

func TestAnUpdateNamesEachFieldFromWhatItReplaced(t *testing.T) {
	p := contactWith(`{"full_name":"Ada Lovelace","title":"Analyst"}`)
	seen := &wordsSeen{}
	tool := updateRecord{p: p, words: seen.words}

	out, err := tool.Handle(context.Background(), promoteToCTO(t, p))
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	got := decodeUpdate(t, out).Changes
	want := []FieldChange{{
		Field: "title", Label: "Job title",
		Before: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: `"Analyst"`},
		After:  crmcontracts.ApprovalPreviewValue{Kind: "text", Text: `"CTO"`},
	}}
	if !slices.Equal(got, want) {
		t.Errorf("changes = %+v, want %+v", got, want)
	}
	if string(seen.current["full_name"]) != `"Ada Lovelace"` {
		t.Errorf("the renderer saw %s, want the whole record so an amount can find its currency", seen.current)
	}
}

func TestACreateNamesEveryFieldItSetFromNothing(t *testing.T) {
	ref := datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()}
	p := &fakeSoR{createRef: ref, records: map[datasource.EntityRef]datasource.Record{
		ref: nativeRecord(datasource.Record{Ref: ref, Fields: json.RawMessage(`{"full_name":"View Plain","currency":"EUR"}`)}),
	}}
	seen := &wordsSeen{}
	tool := createRecord{p: p, words: seen.words}

	out, err := tool.Handle(context.Background(), json.RawMessage(`{"record_type":"contact","fields":{"full_name":"View Plain"}}`))
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	var got createdRecord
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := []FieldChange{{
		Field: "full_name", Label: "Name",
		Before: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: "null"},
		After:  crmcontracts.ApprovalPreviewValue{Kind: "text", Text: `"View Plain"`},
	}}
	if !slices.Equal(got.Changes, want) {
		t.Errorf("changes = %+v, want %+v", got.Changes, want)
	}
	if string(seen.current["currency"]) != `"EUR"` {
		t.Errorf("the renderer saw %s, want the created record's other fields beside the empty ones", seen.current)
	}
}

func TestAFieldWithheldFromTheReaderIsNotNamedAsEmpty(t *testing.T) {
	current, changes, err := changesOf(
		json.RawMessage(`{"title":"CTO","email":"a@example.com"}`),
		json.RawMessage(`{"title":"Analyst","email":null,"masked_fields":["email"]}`),
		json.RawMessage(`{"title":"CTO","email":null,"masked_fields":["email"]}`),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, named := changes["email"]; named {
		t.Errorf("changes = %s, want the withheld email left out", changes)
	}
	if string(current["title"]) != `"Analyst"` {
		t.Errorf("current title = %s, want the value the write replaced", current["title"])
	}
}

// The write has landed by the time its changes are named. A failure to name
// them must not turn a success into a refusal the agent would retry.
func TestChangesThatCannotBeNamedLeaveTheWriteStandingAndSaySo(t *testing.T) {
	p := contactWith(`{"full_name":"Ada Lovelace","title":"Analyst"}`)
	seen := &wordsSeen{err: errors.New("the names are unreachable")}
	r := NewRegistry(&recordingApprovals{}, auth.NewGate(fullSeatAuthority{}))
	r.Register(updateRecord{p: p, words: seen.words})

	out, err := r.Invoke(agentCtx(), "update_record", promoteToCTO(t, p))
	if err != nil {
		t.Fatalf("Invoke err = %v, want the write to stand", err)
	}
	env := sealedEnvelope(t, out)
	if !slices.ContainsFunc(env.Warnings, func(w Warning) bool { return w.Code == warningChangesUnnamed }) {
		t.Errorf("warnings = %+v, want %s", env.Warnings, warningChangesUnnamed)
	}
	if got := decodeUpdate(t, env.Data).Changes; got != nil {
		t.Errorf("changes = %+v, want none", got)
	}
}

func TestReadApprovalLabelsTheChangeAndNamesWhoItIsFor(t *testing.T) {
	seat := ids.NewV7()
	inbox := &fakeInbox{one: StagedApproval{
		StagedActionID: ids.NewV7(), Kind: "update_record", Status: "pending",
		OnBehalfOf: &RecordOwner{ID: seat},
		Changes:    []FieldChange{{Field: "owner_id"}},
	}}
	names := func(context.Context, []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{seat: "Sam Rep"}, nil
	}
	tool := readApprovalTool{inbox: inbox, seats: names}

	out, err := tool.Handle(context.Background(), json.RawMessage(`{"staged_action_id":"`+ids.NewV7().String()+`"}`))
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var got StagedApproval
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Changes[0].Label != "Owner" {
		t.Errorf("label = %q, want Owner", got.Changes[0].Label)
	}
	if got.OnBehalfOf == nil || got.OnBehalfOf.Name != "Sam Rep" {
		t.Errorf("on_behalf_of = %+v, want Sam Rep", got.OnBehalfOf)
	}
}

func TestDuplicateEvidenceNamesEachFieldItCompared(t *testing.T) {
	tool := createRecord{duplicates: func(context.Context, string, ids.UUID, []ids.UUID) ([]DuplicateCandidate, error) {
		return []DuplicateCandidate{{
			CandidateID: "pair", OtherRecordID: ids.NewV7().String(),
			Evidence: []DuplicateEvidence{{Field: "full_name"}, {Field: "phone"}},
		}}, nil
	}}

	found := tool.reportDuplicates(context.Background(), "contact", ids.NewV7(), nil)

	var labels []string
	for _, e := range found[0].Evidence {
		labels = append(labels, e.Label)
	}
	if !slices.Equal(labels, []string{"Name", "Phone"}) {
		t.Errorf("labels = %q, want the fields' names", labels)
	}
}
