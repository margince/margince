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
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// summaryVerbs is the formatting verbs each sentence's call site passes, in
// order. The census holds its key set equal to summaryCopy's sentence fields,
// so a sentence added to the struct with no row here fails.
var summaryVerbs = map[string][]string{
	"archive":             {"%s", "%s"},
	"dismissDuplicate":    {"%s"},
	"reopenDuplicate":     {"%s"},
	"createHead":          {"%s"},
	"updateHead":          {"%s"},
	"logHead":             {"%s"},
	"settingFields":       {"%s"},
	"moreFields":          {"%d"},
	"merge":               {"%s", "%s", "%s"},
	"enrich":              {"%s", "%s", "%s"},
	"ownDomain":           nil,
	"onDeal":              {"%s", "%s"},
	"applyTag":            {"%s"},
	"changeList":          {"%s"},
	"addLineItem":         {"%s"},
	"updateLineItem":      {"%s", "%s"},
	"removeLineItem":      {"%s", "%s"},
	"setStakeholder":      {"%s"},
	"removeStakeholder":   {"%s", "%s"},
	"setCompany":          {"%s"},
	"removeCompany":       {"%s", "%s"},
	"confirmFact":         {"%s", "%s"},
	"updateFact":          {"%s", "%s"},
	"createFact":          {"%s"},
	"deleteFact":          {"%s", "%s"},
	"confirmProfileField": {"%s", "%s"},
	"updateProfileField":  {"%s", "%s"},
	"mergeMembers":        {"%q", "%s", "%q", "%s"},
	"mergeTags":           {"%q", "%q", "%q"},
	"setUpPartner":        {"%s", "%s"},
	"promoteLead":         {"%s", "%s"},
	"disqualifyLead":      {"%s"},
	"demoteLead":          {"%s"},
	"projectPhase":        {"%s", "%s"},
	"dealMove":            {"%s", "%s"},
	"dealMoveOpen":        {"%s"},
	"dealReopen":          {"%s", "%s"},
	"dealChange":          {"%s", "%s", "%s"},
	"dealClose":           {"%s", "%s"},
	"sendEmail":           {"%s"},
	"cc":                  {"%s"},
	"sendSubject":         {"%q"},
	"sendMessage":         {"%q"},
	"draftReply":          {"%s"},
	"accountSend":         {"%s"},
	"accountSendFiled":    {"%q", "%d"},
	"booking":             {"%q", "%s", "%s"},
	"bookingHost":         {"%s"},
	"bookingLinks":        {"%d"},
	"noSubject":           nil,
	"relinkActivity":      {"%s", "%s", "%s"},
	"relinkActivities":    {"%d", "%s", "%s"},
	"importPreview":       {"%s"},
	"importCommit":        {"%d", "%s", "%s"},
	"importCreate":        {"%d"},
	"importUpdate":        {"%d"},
	"importUnchanged":     {"%d"},
	"importSkip":          {"%d"},
	"importIssues":        {"%d"},
	"approveWord":         nil,
	"rejectWord":          nil,
	"decideApproval":      {"%s", "%s"},
	"decideBundle":        {"%s", "%s"},
	"runReport":           {"%s"},
	"composeReport":       {"%d"},
	"bulkChange":          {"%d"},
	"analyticsQuery":      {"%s"},
	"annotateNothing":     nil,
	"annotateNarrative":   nil,
	"annotateBoth":        {"%d"},
	"annotateFindings":    {"%d"},
	"stepUpChanges":       {"%d", "%d", "%s", "%d"},
}

func TestEveryShippedLanguageWritesItsOwnAgentSummaries(t *testing.T) {
	langcopytest.CensusByLanguage(t, summaryByLang, summaryVerbs, "lang")
}

// Every translated language names the same record types, each with a word. A
// type one language learned and another did not would print a German noun on
// one installation and a wire identifier on the next.
func TestEveryTranslatedLanguageNamesTheSameRecordTypes(t *testing.T) {
	german := recordNounsByLang[textlang.German]
	if len(german) == 0 {
		t.Fatal("German names no record type, and it is the reference set")
	}
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			if _, has := recordNounsByLang[lang]; has {
				t.Error("English carries a noun table; it reads the wire type aloud, and a table would be a second spelling")
			}
			continue
		}
		nouns, ok := recordNounsByLang[lang]
		if !ok {
			t.Errorf("the product ships %s and the record nouns have not learned it", lang)
			continue
		}
		for recordType := range german {
			if nouns[recordType] == "" {
				t.Errorf("%s has no word for %s", lang, recordType)
			}
		}
		for recordType := range nouns {
			if _, known := german[recordType]; !known {
				t.Errorf("%s names %s, which German does not", lang, recordType)
			}
		}
	}
}

// English reads the wire type aloud, and a type a language has no word for
// reads as the wire type rather than as nothing.
func TestARecordNounFallsBackToTheWireType(t *testing.T) {
	for _, tc := range []struct {
		lang       textlang.Lang
		recordType string
		want       string
	}{
		{textlang.English, "deal_room", "deal room"},
		{textlang.German, "deal_room", "Deal Room"},
		{textlang.German, "app_user", "app_user"},
		{textlang.Lang("kl"), "saved_view", "saved view"},
	} {
		if got := RecordNoun(tc.lang, tc.recordType); got != tc.want {
			t.Errorf("RecordNoun(%s, %s) = %q, want %q", tc.lang, tc.recordType, got, tc.want)
		}
	}
	if got := MoreFields(textlang.German); got != "+%d weitere" {
		t.Errorf("MoreFields(de) = %q, want the German set's overflow marker", got)
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageDescribesInEnglish(t *testing.T) {
	unshipped := func(context.Context) textlang.Lang { return textlang.Lang("kl") }
	if got := summaryIn(context.Background(), unshipped); got != summaryFor(textlang.English) {
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
	if want := "Kontakt erstellen, Felder: full_name, title"; info.Summary != want {
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
	if want := "Deal ändern, Felder: amount"; patch.Summary != want {
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

// Each set names the language it is written in, and the English wording of a
// record type is the same whichever way a set was reached: a wire type with an
// underscore prints as the wire type through the English set and through a set
// that carries no language at all.
func TestEverySetNamesItsLanguageAndEnglishKeepsTheWireType(t *testing.T) {
	for lang, said := range summaryByLang {
		if said.lang != lang {
			t.Errorf("the %s set says it is written in %q", lang, said.lang)
		}
	}
	english, unnamed := summaryFor(textlang.English), summaryCopy{}
	if got, want := unnamed.noun("deal_room"), english.noun("deal_room"); got != want || got != "deal_room" {
		t.Errorf("the unnamed set prints %q and the English set %q, want both \"deal_room\"", got, want)
	}
}

// German uses only the plain hyphen: an em or en dash in a record noun or a
// sentence breaks the house rule for German copy.
func TestGermanCopyCarriesNoLongDash(t *testing.T) {
	german := langcopytest.Sentences(t, summaryByLang[textlang.German], "lang")
	for recordType, noun := range recordNounsByLang[textlang.German] {
		german["noun "+recordType] = noun
	}
	for name, text := range german {
		if strings.ContainsAny(text, "\u2013\u2014") {
			t.Errorf("German %s carries an em or en dash: %q", name, text)
		}
	}
}
