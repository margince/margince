// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// inRefs writes a reply a test scripted in record ids as the model writes it:
// in the refs the request showed.
func inRefs(in Input, reply string) string {
	return promptref.Rewrite(reply, refsOf(in).tables...)
}

func parseIDs(reply string, companyID ids.CompanyID, in Input) ([]crmcontracts.Company360Suggestion, []string, error) {
	return ParseFindings(inRefs(in, reply), companyID, in, "en")
}

func TestTheScanShowsNoRecordIDAndReadsAMessageRefBack(t *testing.T) {
	in, nudge := scanInput()
	req := ScanRequest(in, "en")
	if leaked := promptref.Mint(req.Messages[0].Content+string(req.ResponseSchema), nil, nil); len(leaked) > 0 {
		t.Fatalf("the request shows %d record ids", len(leaked[""].All()))
	}
	ref, ok := refsOf(in).messages.RefOf(nudge.ID.String())
	if !ok || !strings.Contains(string(req.ResponseSchema), `"`+ref+`"`) {
		t.Fatalf("the citation enum does not offer the message's ref %q: %s", ref, req.ResponseSchema)
	}
	raw := strings.Replace(reply(finding(nudge, "did the sample reports get held up somewhere?")), nudge.ID.String(), ref, 1)
	kept, refused, err := ParseFindings(raw, ids.New[ids.CompanyKind](), in, "en")
	if err != nil || len(kept) != 1 || kept[0].Evidence[0].EntityId.String() != nudge.ID.String() {
		t.Fatalf("kept %+v, refused %v, err %v; want the finding citing %s", kept, refused, err, nudge.ID)
	}
}

func TestAScanFindingCitingARefNeverShownOrARawIDIsRefused(t *testing.T) {
	in, nudge := scanInput()
	for _, cited := range []string{"m99", nudge.ID.String()} {
		raw := strings.Replace(reply(finding(nudge, "did the sample reports get held up somewhere?")), nudge.ID.String(), cited, 1)
		if kept, _, _ := ParseFindings(raw, ids.New[ids.CompanyKind](), in, "en"); len(kept) != 0 {
			t.Errorf("a finding citing %q was kept", cited)
		}
	}
}

func TestAScanFindingSpellingARefInItsReasonIsRefused(t *testing.T) {
	in, nudge := scanInput()
	raw := reply(finding(nudge, "did the sample reports get held up somewhere?"))
	raw = strings.Replace(inRefs(in, raw), `"reason":"`, `"reason":"As m1 shows, `, 1)
	if kept, _, _ := ParseFindings(raw, ids.New[ids.CompanyKind](), in, "en"); len(kept) != 0 {
		t.Error("a finding spelling a ref in its reason was kept")
	}
}
