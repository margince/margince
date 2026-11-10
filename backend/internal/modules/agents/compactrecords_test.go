// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestCompactActivityPagesKeepFactsWithoutFullProse(t *testing.T) {
	body := strings.Repeat("Private email text. ", 1000)
	fields, err := json.Marshal(map[string]string{"kind": "email", "subject": "Project update", "body": body, "transcript": body})
	if err != nil {
		t.Fatal(err)
	}
	page := make([]datasource.Record, 20)
	refs := make([]QueryRef, len(page))
	for i := range page {
		ref := datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()}
		page[i] = datasource.Record{Ref: ref, Fields: fields, Version: 3}
		refs[i] = QueryRef{Type: string(ref.Type), ID: ref.ID}
	}
	provider := &pageProbeProvider{page: page}
	for _, compact := range []bool{false, true} {
		for _, engine := range []string{"list", "query"} {
			t.Run(fmt.Sprintf("%s/compact=%t", engine, compact), func(t *testing.T) {
				var raw json.RawMessage
				var err error
				if engine == "list" {
					raw, err = (listRecords{p: provider}).Handle(t.Context(), fmt.Appendf(nil, `{"record_type":"activity","compact":%t}`, compact))
				} else {
					tool := queryWorkspace{p: provider, run: func(context.Context, json.RawMessage) (QueryAnswer, error) {
						return QueryAnswer{Refs: refs, Coverage: CoverageCompleteExact}, nil
					}}
					raw, err = tool.Handle(t.Context(), fmt.Appendf(nil, `{"plan":{"target":"activity"},"compact":%t}`, compact))
				}
				if err != nil {
					t.Fatal(err)
				}
				if compact {
					if strings.Contains(string(raw), "Private email text") || len(raw) > 10000 {
						t.Fatalf("compact page retained long prose: %d bytes", len(raw))
					}
					if strings.Count(string(raw), fmt.Sprintf(`"body_length":%d`, len(body))) != len(page) || strings.Count(string(raw), `"subject":"Project update"`) != len(page) {
						t.Fatal("compact page lost the subject or body length")
					}
				} else if strings.Count(string(raw), body) != 2*len(page) {
					t.Fatal("full output no longer carries the original prose")
				}
			})
		}
	}
}

func TestCompactOutputLeavesOtherRecordTypesIntact(t *testing.T) {
	record := wireRecord{RecordType: "company", Fields: json.RawMessage(`{"description":"A full company description"}`)}
	original := string(record.Fields)
	if err := compactActivityRecord(&record); err != nil {
		t.Fatal(err)
	}
	if string(record.Fields) != original {
		t.Fatal("compact activity option changed a company")
	}
}

func TestCompactActivityLengthDoesNotInventWithheldContent(t *testing.T) {
	for _, tc := range []struct{ fields, want string }{
		{`{"body":"Grüße 👋","content_state":"readable"}`, `{"body_length":7,"content_state":"readable"}`},
		{`{"body":null,"content_state":"withheld"}`, `{"content_state":"withheld"}`},
	} {
		record := wireRecord{RecordType: "activity", Fields: json.RawMessage(tc.fields)}
		if err := compactActivityRecord(&record); err != nil {
			t.Fatal(err)
		}
		if string(record.Fields) != tc.want {
			t.Fatalf("compact fields = %s, want %s", record.Fields, tc.want)
		}
	}
}
