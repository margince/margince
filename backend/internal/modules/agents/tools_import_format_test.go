// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestDescribeImportFormatAnswersWhatTheSeamBuilt(t *testing.T) {
	want := crmcontracts.ImportFormat{Version: crmcontracts.ImportFormatVersionMarginceImport1, SizeLimitMb: 10}
	out, err := describeImportFormat{imports: recordingImports{format: want}}.Handle(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var got ImportFormatResult
	if err := json.Unmarshal(out, &got); err != nil || got.Format.Version != want.Version || got.Format.SizeLimitMb != 10 {
		t.Fatalf("got %+v err %v", got, err)
	}
}
