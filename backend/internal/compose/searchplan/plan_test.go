// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package searchplan

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

func TestSearchPlansCannotChooseWritesOrSmuggleAnExtraRequest(t *testing.T) {
	for _, reply := range []string{
		`{"engine":"create_record","arguments":"{}","count":false}`,
		`{"engine":"records","arguments":"[]","count":false}`,
		`{"engine":"records","arguments":"{}","count":true}`,
		`{"engine":"unsupported","arguments":"{}","extra":"write"}`,
		`{"engine":"unsupported","arguments":"{}"} {"engine":"records"}`,
	} {
		if _, err := Decode(reply); err == nil {
			t.Errorf("accepted %s", reply)
		}
	}
}

func TestSearchCertificationRejectsADifferentPeriodAndCount(t *testing.T) {
	factory := Cases{}
	expected := `{"engine":"reporting","arguments":"{\"scope\":{\"kind\":\"workspace\"},\"period\":\"this_month\",\"target_basis\":\"month\",\"close_window\":\"all_open\",\"metrics\":[\"bookings_won\"],\"blocks\":[]}","count":true}`
	prepared, err := factory.Prepare([]byte(`{"question":"How many deals won this month?","vocabulary":"Sales won: bookings_won"}`), []byte(expected))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := prepared.Evaluate(aitasks.Trace{Output: expected}); outcome.Result != aitasks.OutcomeAccepted {
		t.Fatal(outcome)
	}
	if outcome := prepared.Evaluate(aitasks.Trace{Output: `{"engine":"unsupported","arguments":"{}","count":false}`}); outcome.Result == aitasks.OutcomeAccepted {
		t.Fatal("a missing data answer passed certification")
	}
}

func TestSearchPlansResolveOnlySuppliedIdentityRefs(t *testing.T) {
	const id = "019f0000-0000-7000-8000-000000000001"
	const question = "Which contacts belong to me?"
	vocabulary := `{"self_user_id":"` + id + `"}`
	request := Request(question, vocabulary)
	if strings.Contains(request.Messages[0].Content, id) {
		t.Fatal("the planner was shown a record UUID")
	}
	reply := `{"engine":"records","count":false,"arguments":"{\"plan\":{\"version\":\"v1\",\"target\":\"contact\",\"where\":[{\"field\":\"owner_id\",\"op\":\"eq\",\"value\":\"r1\"},{\"field\":\"full_name\",\"op\":\"eq\",\"value\":\"r1\"}]}}"}`
	plan, err := Read(reply, question, vocabulary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Arguments, id) || !strings.Contains(plan.Arguments, `"full_name","op":"eq","value":"r1"`) {
		t.Fatalf("identity resolution changed an ordinary name: %s", plan.Arguments)
	}
	if _, err := Read(strings.ReplaceAll(reply, `r1`, `r99`), question, vocabulary); err == nil {
		t.Fatal("a record ref absent from the prompt was accepted")
	}
}

func TestSearchAbstainsBeforeApplyingReaderSpecificConstraints(t *testing.T) {
	const question = "How many deals did I win for a category I cannot identify?"
	plan, err := Read(`{"engine":"unsupported","arguments":"{}","count":false,"unsupported_parts":[]}`, question, "{}")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != EngineUnsupported || plan.Arguments != "{}" || plan.Count {
		t.Fatalf("the abstention was not preserved: %+v", plan)
	}
}
