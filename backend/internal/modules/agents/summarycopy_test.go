// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// summaryFields lists one set's sentences beside the formatting verbs each must
// carry, in the order the arguments are passed.
func summaryFields(said summaryCopy) []struct {
	name, got string
	verbs     []string
} {
	return []struct {
		name, got string
		verbs     []string
	}{
		{"archive", said.archive, []string{"%s", "%s"}},
		{"createHead", said.createHead, []string{"%s"}},
		{"updateHead", said.updateHead, []string{"%s"}},
		{"logHead", said.logHead, []string{"%s"}},
		{"settingFields", said.settingFields, []string{"%s"}},
		{"moreFields", said.moreFields, []string{"%d"}},
		{"overwriteHuman", said.overwriteHuman, []string{"%s", "%s", "%s"}},
		{"merge", said.merge, []string{"%s", "%s", "%s"}},
		{"enrich", said.enrich, []string{"%s", "%s", "%s"}},
		{"ownDomain", said.ownDomain, nil},
		{"onDeal", said.onDeal, []string{"%s", "%s"}},
		{"applyTag", said.applyTag, []string{"%s"}},
		{"addLineItem", said.addLineItem, []string{"%s"}},
		{"updateLineItem", said.updateLineItem, []string{"%s", "%s"}},
		{"removeLineItem", said.removeLineItem, []string{"%s", "%s"}},
		{"retireCustomField", said.retireCustomField, []string{"%s"}},
		{"customFieldOptions", said.customFieldOptions, []string{"%s"}},
		{"setStakeholder", said.setStakeholder, []string{"%s"}},
		{"removeStakeholder", said.removeStakeholder, []string{"%s", "%s"}},
		{"setCompany", said.setCompany, []string{"%s"}},
		{"removeCompany", said.removeCompany, []string{"%s", "%s"}},
		{"confirmFact", said.confirmFact, []string{"%s", "%s"}},
		{"updateFact", said.updateFact, []string{"%s", "%s"}},
		{"createFact", said.createFact, []string{"%s"}},
		{"deleteFact", said.deleteFact, []string{"%s", "%s"}},
		{"confirmProfileField", said.confirmProfileField, []string{"%s", "%s"}},
		{"updateProfileField", said.updateProfileField, []string{"%s", "%s"}},
		{"mergeTags", said.mergeTags, []string{"%q", "%q", "%q"}},
		{"promoteLead", said.promoteLead, []string{"%s", "%s"}},
		{"disqualifyLead", said.disqualifyLead, []string{"%s"}},
		{"demoteLead", said.demoteLead, []string{"%s"}},
		{"projectPhase", said.projectPhase, []string{"%s", "%s"}},
		{"dealMove", said.dealMove, []string{"%s", "%s"}},
		{"dealMoveOpen", said.dealMoveOpen, []string{"%s"}},
		{"dealReopen", said.dealReopen, []string{"%s", "%s"}},
		{"dealChange", said.dealChange, []string{"%s", "%s", "%s"}},
		{"dealClose", said.dealClose, []string{"%s", "%s"}},
		{"sendEmail", said.sendEmail, []string{"%s"}},
		{"cc", said.cc, []string{"%s"}},
		{"sendSubject", said.sendSubject, []string{"%q"}},
		{"sendMessage", said.sendMessage, []string{"%q"}},
		{"draftReply", said.draftReply, []string{"%s"}},
		{"accountSend", said.accountSend, []string{"%s"}},
		{"accountSendFiled", said.accountSendFiled, []string{"%q", "%d"}},
		{"booking", said.booking, []string{"%q", "%s", "%s"}},
		{"bookingHost", said.bookingHost, []string{"%s"}},
		{"bookingLinks", said.bookingLinks, []string{"%d"}},
		{"noSubject", said.noSubject, nil},
		{"relinkActivity", said.relinkActivity, []string{"%s", "%s", "%s"}},
		{"relinkThread", said.relinkThread, []string{"%q", "%s", "%s"}},
		{"relinkActivities", said.relinkActivities, []string{"%d", "%s", "%s"}},
		{"importPreview", said.importPreview, []string{"%s"}},
		{"importCommit", said.importCommit, []string{"%d", "%s", "%s"}},
		{"importCreate", said.importCreate, []string{"%d"}},
		{"importUpdate", said.importUpdate, []string{"%d"}},
		{"importUnchanged", said.importUnchanged, []string{"%d"}},
		{"importSkip", said.importSkip, []string{"%d"}},
		{"importIssues", said.importIssues, []string{"%d"}},
		{"approveWord", said.approveWord, nil},
		{"rejectWord", said.rejectWord, nil},
		{"decideApproval", said.decideApproval, []string{"%s", "%s"}},
		{"decideBundle", said.decideBundle, []string{"%s", "%s"}},
		{"runReport", said.runReport, []string{"%s"}},
		{"composeReport", said.composeReport, []string{"%d"}},
		{"analyticsQuery", said.analyticsQuery, []string{"%s"}},
		{"annotateNothing", said.annotateNothing, nil},
		{"annotateNarrative", said.annotateNarrative, nil},
		{"annotateBoth", said.annotateBoth, []string{"%d"}},
		{"annotateFindings", said.annotateFindings, []string{"%d"}},
		{"stepUpRecords", said.stepUpRecords, []string{"%d", "%d", "%s", "%d"}},
		{"stepUpChanges", said.stepUpChanges, []string{"%d", "%d", "%s", "%d"}},
	}
}

// verbsIn answers a sentence's formatting verbs in order. Go's verbs are
// positional, so a translation that reorders %q and %d prints the subject as
// the count.
func verbsIn(sentence string) []string {
	var verbs []string
	for i := 0; i+1 < len(sentence); i++ {
		if sentence[i] == '%' {
			verbs = append(verbs, sentence[i:i+2])
			i++
		}
	}
	return verbs
}

// Every shipped language writes the staging summaries in its own words, with
// the English sentence's formatting verbs in the English order.
func TestEveryShippedLanguageWritesItsOwnAgentSummaries(t *testing.T) {
	english := summaryFields(summaryByLang[textlang.English])
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := summaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for i, field := range summaryFields(said) {
				if field.got == "" {
					t.Errorf("%s is empty, and a blank summary reaches the inbox as nothing at all", field.name)
					continue
				}
				if got := strings.Join(verbsIn(field.got), " "); got != strings.Join(field.verbs, " ") {
					t.Errorf("%s carries verbs [%s], want [%s] in that order: %q",
						field.name, got, strings.Join(field.verbs, " "), field.got)
				}
				if lang != textlang.English && field.got == english[i].got {
					t.Errorf("%s is the English sentence verbatim; the entry exists and the reader still gets English",
						field.name)
				}
			}
		})
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageDescribesInEnglish(t *testing.T) {
	unshipped := func(context.Context) textlang.Lang { return textlang.Lang("kl") }
	if got := summaryIn(context.Background(), unshipped); got != summaryByLang[textlang.English] {
		t.Errorf("an unshipped language answered %+v, want the English set", got)
	}
}

func germanInstallation(context.Context) textlang.Lang { return textlang.German }

// The tool door writes in the language the registry was composed with: the
// option reaches the tool, and the tool hands it to the resolver.
func TestTheToolDoorDescribesInTheComposedLanguage(t *testing.T) {
	r := NewRegistry(nil, nil, WithBaseLanguage(germanInstallation))
	RegisterCoreTools(r, stubRecordProvider{}, nil, nil, nil, nil, nil)
	tool, ok := r.tools["create_record"].(createRecord)
	if !ok {
		t.Fatalf("create_record is registered as %T, want createRecord", r.tools["create_record"])
	}

	info, err := tool.StageInfo(context.Background(),
		json.RawMessage(`{"record_type":"contact","fields":{"full_name":"Ada","title":"CTO"}}`))
	if err != nil {
		t.Fatalf("staging a served create answered %v", err)
	}
	if want := "Datensatz vom Typ contact anlegen, Felder: full_name, title"; info.Summary != want {
		t.Errorf("summary = %q, want %q", info.Summary, want)
	}
}

// A patch, an account-started send and a booking all read the injected
// language, and pass their values through untranslated.
func TestStagedSummariesFollowTheInjectedLanguage(t *testing.T) {
	ctx := context.Background()
	patch, err := NewPatchCall(nil, germanInstallation, PatchCommand{
		RecordType: "deal", ID: ids.NewV7(), Fields: json.RawMessage(`{"amount":1}`),
	}).Subject(ctx)
	if err != nil {
		t.Fatalf("describing a patch: %v", err)
	}
	if want := "Datensatz vom Typ deal ändern, Felder: amount"; patch.Summary != want {
		t.Errorf("patch summary = %q, want %q", patch.Summary, want)
	}

	said := summaryIn(ctx, germanInstallation)
	send := describeAccountSend(said, SendCompanyEmailCommand{
		To: []string{"ada@example.com"}, Cc: []string{"bob@example.com"}, Subject: "Angebot",
	}, []RecordLink{{EntityType: "company", EntityID: ids.NewV7()}})
	if want := `E-Mail-Unterhaltung beginnen mit ada@example.com, Cc: bob@example.com, ` +
		`Betreff "Angebot", zugeordnete Datensätze: 1`; send != want {
		t.Errorf("send summary = %q, want %q", send, want)
	}

	start := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	booking := describeBooking(said, BookMeetingCommand{Start: start, End: start.Add(time.Hour)},
		[]RecordLink{{EntityType: "deal", EntityID: ids.NewV7()}})
	if want := `Termin buchen: "(kein Betreff)" von 2026-08-10T09:00:00Z bis 2026-08-10T10:00:00Z, ` +
		`verknüpfte Datensätze: 1`; booking != want {
		t.Errorf("booking summary = %q, want %q", booking, want)
	}
}

// A decision, a volume step-up and a brief annotation read the injected
// language too, including the verdict word the sentence opens with.
func TestAgentWorkSummariesFollowTheInjectedLanguage(t *testing.T) {
	ctx := context.Background()
	id := ids.NewV7()
	decide, err := NewDecideApprovalCall(germanInstallation, DecideApprovalCommand{ApprovalID: id}).Subject(ctx)
	if err != nil {
		t.Fatalf("describing a decision: %v", err)
	}
	if want := "Ablehnen: vorgemerkte Aktion " + id.String(); decide.Summary != want {
		t.Errorf("decision summary = %q, want %q", decide.Summary, want)
	}

	brief, err := NewAnnotateBriefCall(germanInstallation, AnnotateBriefCommand{Items: 2}).Subject(ctx)
	if err != nil {
		t.Fatalf("describing an annotation: %v", err)
	}
	if want := "2 Erkenntnis(se) in deinen Morgenbericht schreiben"; brief.Summary != want {
		t.Errorf("annotation summary = %q, want %q", brief.Summary, want)
	}
}
