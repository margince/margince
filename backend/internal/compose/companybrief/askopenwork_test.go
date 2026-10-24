// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// "What is still open?" must name the open records, not summarise them as a
// count. The completeness check refuses a model reply that names fewer records
// than the input holds. The floor that replaces it passes the same check.

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type cannedLane struct{ reply string }

func (l cannedLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}

func citing(text, entityType, id string) string {
	return fmt.Sprintf(`{"text":%q,"evidence":[{"entity_type":%q,"entity_id":%q}]}`, text, entityType, id)
}

func replyOf(sentences ...string) cannedLane {
	return cannedLane{reply: `{"sentences":[` + strings.Join(sentences, ",") + `]}`}
}

const (
	taskActivity       = "019fac6c-60d1-732b-ab2b-d93611745625"
	commitmentActivity = "019fac6c-60c8-745f-8c5f-1503a9a4ea48"
)

func taskAndCommitment() Input {
	return Input{
		Name:      "Brandt Automotive GmbH",
		OpenTasks: []TaskIn{{ID: taskActivity, Name: "Send the quote", Holder: "Mira Vogt", Due: "2026-07-21T09:00:00Z"}},
		OpenCommitments: []CommitmentIn{{
			ID: "019fac6c-60f8-77d4-a267-dabced904009", ActivityID: commitmentActivity,
			Claim: "We will share the security audit", Side: sideOurs, Due: "2026-07-28T09:00:00Z",
		}},
	}
}

func answerOpen(t *testing.T, lane Completer, in Input) ([]Sentence, crmcontracts.WrittenBy) {
	t.Helper()
	answered, by, err := Answer(t.Context(), lane, crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, string(textlang.English))
	if err != nil {
		t.Fatalf("answer: %v", err)
	}
	return answered, by
}

func TestAOneSentenceReplyOverTwoKindsOfOpenWorkServesTheFloor(t *testing.T) {
	in := taskAndCommitment()
	lane := replyOf(citing("You have one open task.", "activity", taskActivity))
	answered, by := answerOpen(t, lane, in)
	if by != crmcontracts.WrittenByDeterministic {
		t.Fatalf("generated_by = %q, want the floor: the reply left the commitment out", by)
	}
	joined := strings.Join(texts(answered), " ")
	for _, want := range []string{"Send the quote", "We will share the security audit"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the floor %q does not name %q", joined, want)
		}
	}
}

func TestAReplyCitingEveryOpenRecordIsKept(t *testing.T) {
	in := taskAndCommitment()
	lane := replyOf(
		citing("Send the quote is open.", "activity", taskActivity),
		citing("We promised the security audit.", "activity", commitmentActivity))
	_, by := answerOpen(t, lane, in)
	if by != crmcontracts.WrittenByModel {
		t.Errorf("generated_by = %q, want the model: the reply cited both records", by)
	}
}

func TestAReplyMissingTheCommitmentIsRefusedWhateverItSaysAboutTasks(t *testing.T) {
	in := taskAndCommitment()
	lane := replyOf(
		citing("Send the quote is open.", "activity", taskActivity),
		citing("Brandt is an automotive account.", "company", askCompanyID))
	if _, by := answerOpen(t, lane, in); by != crmcontracts.WrittenByDeterministic {
		t.Errorf("generated_by = %q, want the floor", by)
	}
}

func TestACappedListNeedsListedRecordsCitationsNotAll(t *testing.T) {
	in := Input{Name: "Brandt Automotive GmbH"}
	for i := range listedRecords + 2 {
		in.OpenDeals = append(in.OpenDeals, DealIn{ID: fmt.Sprintf("019fac6c-60d1-732b-ab2b-d3611745%04d", i), Name: fmt.Sprintf("Deal %d", i)})
	}
	cite := func(n int) cannedLane {
		var sentences []string
		for _, deal := range in.OpenDeals[:n] {
			sentences = append(sentences, citing(deal.Name+" is open.", "deal", deal.ID))
		}
		return replyOf(sentences...)
	}
	if _, by := answerOpen(t, cite(listedRecords), in); by != crmcontracts.WrittenByModel {
		t.Errorf("a reply citing %d of %d deals was refused (%q)", listedRecords, len(in.OpenDeals), by)
	}
	if _, by := answerOpen(t, cite(listedRecords-1), in); by != crmcontracts.WrittenByDeterministic {
		t.Errorf("a reply citing %d deals was kept (%q)", listedRecords-1, by)
	}
}

func TestTheFloorPassesItsOwnCompletenessCheck(t *testing.T) {
	in := taskAndCommitment()
	for i := range listedRecords + 3 {
		in.OpenCommitments = append(in.OpenCommitments, CommitmentIn{
			ID: fmt.Sprintf("019fac6c-60f8-77d4-a267-dabced90%04d", i), ActivityID: fmt.Sprintf("019fac6c-60c8-745f-8c5f-1503a9a4%04d", i),
			Claim: fmt.Sprintf("Promise %d", i), Side: sideTheirs, Holder: "Dana Buyer",
		})
	}
	floorAnswer := deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, "en")
	if missing := uncitedOpenWork(floorAnswer, in); missing != "" {
		t.Errorf("the floor leaves out open %s", missing)
	}
}

func TestTwoPromisesFromOneMessageEachNameTheirOwnClaim(t *testing.T) {
	in := Input{OpenCommitments: []CommitmentIn{
		{ID: "c1", ActivityID: commitmentActivity, Claim: "Send the price list", Side: sideOurs},
		{ID: "c2", ActivityID: commitmentActivity, Claim: "Book the site visit", Side: sideOurs},
	}}
	answered := deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, "en")
	names := map[string]bool{}
	for _, sentence := range answered {
		for _, cite := range sentence.Evidence {
			names[cite.Name] = true
		}
	}
	if !names["Send the price list"] || !names["Book the site visit"] {
		t.Errorf("chip names %v: each promise needs its own claim as the chip name", names)
	}
}

func TestTheFloorNamesClaimHolderAndDateInEveryLanguage(t *testing.T) {
	in := Input{OpenCommitments: []CommitmentIn{
		{ID: "c1", ActivityID: commitmentActivity, Claim: "OURS-CLAIM", Side: sideOurs, Due: "2026-07-28T09:00:00Z"},
		{ID: "c2", ActivityID: taskActivity, Claim: "THEIRS-CLAIM", Side: sideTheirs, Holder: "Dana Buyer", Due: "2026-07-29T09:00:00Z"},
	}, OpenTasks: []TaskIn{{ID: "t1", Name: "TASK-NAME", Holder: "Mira Vogt", Due: "2026-07-30T09:00:00Z"}}}
	for _, lang := range []string{"en", "de", "vi"} {
		joined := strings.Join(texts(deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, lang)), "\n")
		for _, want := range []string{"OURS-CLAIM", "THEIRS-CLAIM", "Dana Buyer", "TASK-NAME", "Mira Vogt", "28", "29", "30"} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: the answer lacks %q:\n%s", lang, want, joined)
			}
		}
		if claims.SpellsRecordID(joined) {
			t.Errorf("%s: the answer spells an id:\n%s", lang, joined)
		}
	}
}

func TestACommitmentOfOursHasNoHolder(t *testing.T) {
	view := crmcontracts.Company360{}
	view.OpenCommitments = &struct {
		Data []crmcontracts.Company360Commitment `json:"data"`
		Page crmcontracts.PageInfo               `json:"page"`
	}{Data: []crmcontracts.Company360Commitment{{
		Kind: crmcontracts.Company360CommitmentKindOurs, Body: "Send the quote", ContactName: "Dana Buyer",
		ClaimId: openapi_types.UUID{1}, ActivityId: openapi_types.UUID{2},
	}, {
		Kind: crmcontracts.Company360CommitmentKindTheirs, Body: "Pay the invoice", ContactName: "Dana Buyer",
		ClaimId: openapi_types.UUID{3}, ActivityId: openapi_types.UUID{4},
	}}}
	folded := FromView(view).OpenCommitments
	if len(folded) != 2 || folded[0].Holder != "" || folded[1].Holder != "Dana Buyer" {
		t.Errorf("holders = %+v, want none for ours and the contact for theirs", folded)
	}
}

func TestACutOrNarrowedReadNeverReadsAsEverything(t *testing.T) {
	for name, in := range map[string]Input{
		"withheld and empty": {OpenWorkWithheld: true},
		"capped":             {OpenWorkCapped: true, OpenTasks: []TaskIn{{ID: taskActivity, Name: "Send the quote"}}},
	} {
		answered := deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, "en")
		last := answered[len(answered)-1]
		if !strings.Contains(last.Text, "may not show everything") || last.Evidence[0].EntityID != askCompanyID {
			t.Errorf("%s: last sentence %q, want the incomplete-list sentence citing the company", name, last.Text)
		}
	}
	if got := deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, Input{}, "en"); len(got) != 0 {
		t.Errorf("a complete empty read answered %q", texts(got))
	}
}

func TestAModelAnswerOverACutReadStillSaysSo(t *testing.T) {
	in := taskAndCommitment()
	in.OpenWorkCapped = true
	lane := replyOf(
		citing("Send the quote is open.", "activity", taskActivity),
		citing("We promised the security audit.", "activity", commitmentActivity))
	answered, by := answerOpen(t, lane, in)
	if by != crmcontracts.WrittenByModel {
		t.Fatalf("generated_by = %q", by)
	}
	if last := answered[len(answered)-1]; !strings.Contains(last.Text, "may not show everything") {
		t.Errorf("last sentence %q does not say the list may be incomplete", last.Text)
	}
}

// A slice added to Input is either a kind of open record, which the answer must
// cite, or not one. A new unclassified slice fails here.
func TestEveryInputListIsClassifiedAsOpenWorkOrNot(t *testing.T) {
	notOpenWork := map[string]bool{
		"Contacts": true, "Recent": true, "Profile": true, "SectionsOmitted": true,
	}
	required := map[string]openGroup{}
	for _, group := range requiredOpenCitations(taskAndCommitment()) {
		required[group.Field] = group
	}
	typ := reflect.TypeFor[Input]()
	for field := range typ.Fields() {
		if field.Type.Kind() != reflect.Slice || notOpenWork[field.Name] {
			continue
		}
		if _, ok := required[field.Name]; !ok {
			t.Errorf("Input.%s is a list nobody classified: add it to requiredOpenCitations or to notOpenWork", field.Name)
		}
	}
}

func TestEveryOpenRecordKindIsCitedAndNamed(t *testing.T) {
	in := taskAndCommitment()
	in.OpenDeals = []DealIn{{ID: "019fac6c-60d1-732b-ab2b-d3611745aaaa", Name: "Fleet deal"}}
	answered := deterministicAnswer(crmcontracts.CompanyQuestionWhatsOpen, askCompanyID, in, "en")
	names := recordNames(in)
	for _, group := range requiredOpenCitations(in) {
		if len(group.Cites) == 0 {
			t.Errorf("%s: no records to cite in the fixture", group.Field)
		}
		for _, cite := range group.Cites {
			if names[recordKey{cite.EntityType, cite.EntityID}] == "" {
				t.Errorf("%s: %v has no chip name", group.Field, cite)
			}
		}
	}
	if missing := uncitedOpenWork(answered, in); missing != "" {
		t.Errorf("the floor leaves out open %s", missing)
	}
}

func TestAViewWithItsOpenWorkCutFoldsTheGapIntoTheInput(t *testing.T) {
	withheld := true
	view := crmcontracts.Company360{AttentionWithheld: &withheld}
	view.NextSteps = &struct {
		Data []crmcontracts.Company360NextStep `json:"data"`
		Page crmcontracts.PageInfo             `json:"page"`
	}{Page: crmcontracts.PageInfo{HasMore: true}}
	in := FromView(view)
	if !in.OpenWorkWithheld || !in.OpenWorkCapped {
		t.Errorf("gaps = capped %v withheld %v, want both", in.OpenWorkCapped, in.OpenWorkWithheld)
	}
	due := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	name := "Mira Vogt"
	view.NextSteps.Data = []crmcontracts.Company360NextStep{{Subject: "Send", DueAt: &due, AssigneeName: &name}}
	if got := FromView(view).OpenTasks[0].Holder; got != name {
		t.Errorf("task holder = %q, want %q", got, name)
	}
}

func TestTwoPromisesFromOneMessageNeedASentenceEach(t *testing.T) {
	in := Input{OpenCommitments: []CommitmentIn{
		{ID: "c1", ActivityID: commitmentActivity, Claim: "Send the price list", Side: sideOurs},
		{ID: "c2", ActivityID: commitmentActivity, Claim: "Book the site visit", Side: sideOurs},
	}}
	cite := []Evidence{{EntityType: citeActivity, EntityID: commitmentActivity}}
	one := []Sentence{{Text: "You send the price list.", Evidence: cite}}
	if uncitedOpenWork(one, in) != "OpenCommitments" {
		t.Error("one sentence passed for two promises")
	}
	two := append(slices.Clone(one), Sentence{Text: "You book the site visit.", Evidence: cite})
	if missing := uncitedOpenWork(two, in); missing != "" {
		t.Errorf("a sentence per promise still leaves out %s", missing)
	}
}

func TestTheAskPromptCarriesTheOpenWorkRule(t *testing.T) {
	system := AskRequest(crmcontracts.CompanyQuestionWhatsOpen, Input{}, "en").System
	if !strings.Contains(system, openWorkRule) {
		t.Error("the ask prompt lacks the open-work rule")
	}
}
