// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestASignalReadsItsPartsInTheReadersLanguage(t *testing.T) {
	for _, c := range []struct {
		kind, params, want string
	}{
		{kindTechnicalChange, `{"field":"mail_provider","kind":"moved","value":"Microsoft 365","value_key":"microsoft365","previous":"Own mail server","previous_key":"self_hosted"}`,
			"Mail hiện chạy trên Microsoft 365 (trước đây là máy chủ mail riêng)"},
		{kindGhostedThread, `{"days":14}`, "Chúng ta đã viết cách đây ít nhất 14 ngày và chưa ai trả lời."},
		{kindProjectGoneQuiet, `{"project":"ERP rollout","days":30}`, "Không có gì được lưu cho ERP rollout trong ít nhất 30 ngày."},
	} {
		if got := signals.SummaryIn(textlang.Vietnamese, c.kind, json.RawMessage(c.params), "stored"); got != c.want {
			t.Errorf("%s reads %q, want %q", c.kind, got, c.want)
		}
	}
}

// Parts the words do not know leave the stored sentence standing. A reader
// never gets an error, or a sentence with a hole in it.
func TestPartsTheWordsDoNotKnowKeepTheStoredSummary(t *testing.T) {
	for name, c := range map[string]struct{ kind, params string }{
		"a kind nobody words":       {"contract_ended", `{"days":14}`},
		"no parts at all":           {kindGhostedThread, ``},
		"parts that are not JSON":   {kindGhostedThread, `{"days":`},
		"a part the kind never had": {kindGhostedThread, `{"days":14,"weeks":2}`},
		"a count of no days":        {kindGhostedThread, `{"days":0}`},
		"a project with no name":    {kindProjectGoneQuiet, `{"days":30}`},
		"a change with no value":    {kindTechnicalChange, `{"field":"technology","kind":"appeared"}`},
		"a move from nowhere":       {kindTechnicalChange, `{"field":"hosting_provider","kind":"moved","value":"Hetzner"}`},
		"a change of no known kind": {kindTechnicalChange, `{"field":"technology","kind":"renamed","value":"nginx"}`},
		"a field no rep acts on":    {kindTechnicalChange, `{"field":"email_security","kind":"appeared","value":"dmarc_reject"}`},
	} {
		if got := signals.SummaryIn(textlang.Vietnamese, c.kind, json.RawMessage(c.params), "stored"); got != "stored" {
			t.Errorf("%s reads %q, want the stored summary", name, got)
		}
	}
}
