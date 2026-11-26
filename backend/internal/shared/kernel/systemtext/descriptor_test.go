// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestCatalogRendersEveryDefinition(t *testing.T) {
	phrases := make(map[string]langcopy.Phrase, len(catalog))
	for code, definition := range catalog {
		phrases[code] = definition.Words
		placeholders := 0
		for _, verb := range langcopytest.Placeholders(definition.Words.En) {
			if verb != "%%" {
				placeholders++
			}
		}
		if got := placeholders; got != len(definition.Arguments) {
			t.Errorf("%s declares %d arguments for %d placeholders", code, len(definition.Arguments), got)
		}
		component := Sentence(code)
		for _, kind := range definition.Arguments {
			switch kind {
			case "count":
				component.Args = append(component.Args, Count(2))
			case "date":
				component.Args = append(component.Args, Date("2026-11-09"))
			case "text":
				component.Args = append(component.Args, Text("Example"))
			default:
				t.Fatalf("%s declares unsupported argument %s", code, kind)
			}
		}
		for _, locale := range textlang.Shipped {
			rendered, err := component.Render(locale)
			if err != nil || strings.Contains(rendered, "%!") || rendered == "" {
				t.Errorf("%s/%s rendered %q: %v", code, locale, rendered, err)
			}
		}
	}
	langcopytest.Census(t, phrases)
}

func TestBindingRetiresEditedComponents(t *testing.T) {
	subject := Literal("automation.stage_change")
	message, err := Bind(&Message{Subject: Sentence("automation.stage_change"), Body: Prose(Quote, textlang.English)}, subject, "Customer wording")
	if err != nil {
		t.Fatal(err)
	}
	title, body := message.Displays(subject, "Customer wording", textlang.Vietnamese)
	if title == nil || *title == subject || body != nil {
		t.Fatalf("localized title and original quote: %v %v", title, body)
	}
	edited, err := Bind(message, "My own instruction", "Customer wording")
	if err != nil || edited.Subject != nil || edited.Body == nil {
		t.Fatalf("edit must retire only its component: %#v, %v", edited, err)
	}
	if title, _ := message.Displays("My own instruction", "Customer wording", textlang.German); title != nil {
		t.Fatal("stale metadata replaced edited text")
	}
}

func TestInvalidDescriptorsAreRefused(t *testing.T) {
	for name, message := range map[string]*Message{
		"unknown version":  {Version: 2},
		"unknown code":     {Subject: Sentence("missing")},
		"missing argument": {Subject: Sentence("lead.reply")},
		"extra argument":   {Subject: Sentence("automation.stage_change", Text("extra"))},
		"wrong type":       {Subject: Sentence("lead.reply", Count(2))},
		"large argument":   {Subject: Sentence("lead.reply", Text(strings.Repeat("x", 513)))},
		"prose template":   {Subject: &Component{Origin: Generated, Code: "lead.reply"}},
		"quote sequence":   {Body: &Component{Origin: Quote, Following: []Component{*Sentence("core.erased")}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Bind(message, "Original", "Original"); err == nil {
				t.Fatal("invalid descriptor accepted")
			}
		})
	}
}

func TestDestinationMetadataIsIndependent(t *testing.T) {
	original := &Message{Version: 1, Subject: Sequence(Sentence("lead.reply", Text("Example")), Sentence("core.erased"))}
	cloned := ForDestination(original)
	cloned.Subject.Args[0] = Text("Other")
	cloned.Subject.Following[0].Code = "core.proposal_removed"
	if original.Subject.Args[0].Text != "Example" || original.Subject.Following[0].Code != "core.erased" {
		t.Fatal("destination metadata changed source metadata")
	}
}

func TestDuplicateCodesAreRefused(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("duplicate message code accepted")
		}
	}()
	mergeCatalogs(definitions, definitions)
}

func TestTemplateCannotIntroduceFactsAbsentFromCanonicalText(t *testing.T) {
	message, err := Bind(&Message{Subject: Sentence("lead.reply", Text("Hidden contact"))}, "Review qualification for Example", "")
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != nil {
		t.Fatal("descriptor introduced a name absent from the canonical component")
	}
}

func TestQuotedRangesKeepOriginalSourceWithoutStoringItAgain(t *testing.T) {
	canonical := "Review qualification for Example"
	message, err := Bind(&Message{Subject: Sentence("lead.reply", Excerpt(25, 7))}, canonical, "")
	if err != nil || message.Subject == nil {
		t.Fatalf("bind source range: %#v %v", message, err)
	}
	subject, _ := message.Displays(canonical, "", textlang.Vietnamese)
	if subject == nil || !strings.Contains(*subject, "Example") {
		t.Fatalf("original label was lost: %v", subject)
	}
	for _, argument := range []Argument{Excerpt(-1, 7), Excerpt(25, 100), Excerpt(40, 1)} {
		if _, err := Bind(&Message{Subject: Sentence("lead.reply", argument)}, canonical, ""); err == nil {
			t.Errorf("invalid source span accepted: %+v", argument)
		}
	}
}

func TestDestinationLimitsShortenOnlyOpaqueLabels(t *testing.T) {
	name := strings.Repeat("長", 180)
	original := "Review qualification for " + name
	message, subject, _, err := Fit(&Message{Subject: Sentence("lead.reply", Label(name))}, original, "", 100, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subject, "…") {
		t.Fatal("shortened label has no indication")
	}
	bound, err := Bind(message, subject, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range textlang.Shipped {
		title, _ := bound.Displays(subject, "", language)
		if title == nil || len([]rune(*title)) > 100 {
			t.Fatalf("%s subject violates destination cap: %v", language, title)
		}
	}
	if _, _, _, err := Fit(&Message{Subject: Sentence("lead.reply", Text(name))}, original, "", 100, 2000); err == nil {
		t.Fatal("immutable source text was shortened")
	}
}

func TestMetadataBoundIncludesDatabaseJSONSpacing(t *testing.T) {
	encoded := []byte(`{"code":"quoted, colon: and \\\"", "count":2}`)
	if metadataSize(encoded) != len(encoded)+3 {
		t.Fatalf("quoted punctuation changed the structural size: %d", metadataSize(encoded))
	}
	if _, err := Bind(&Message{Body: Sentence("receipt.overdue", Date("not a date"), Text("pending"))}, "", "Original"); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestOversizeOpaqueLabelsCanFitWithoutAcceptingOversizeQuotes(t *testing.T) {
	label := strings.Repeat("x", 600)
	component := Sentence("lead.reply", Label(label))
	original := Canonical(component, textlang.English)
	message, subject, _, err := Fit(&Message{Subject: component}, original, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(subject)) > 100 || !strings.Contains(subject, "…") {
		t.Fatalf("label not visibly shortened: %q", subject)
	}
	if _, err := Bind(message, subject, ""); err != nil {
		t.Fatal(err)
	}
	quoted := Sentence("lead.reply", Text(label))
	if _, _, _, err := Fit(&Message{Subject: quoted}, original, "", 100, 0); err == nil {
		t.Fatal("immutable quote silently shortened")
	}
}

func TestGermanSystemCopyUsesTheReadersInformalRegister(t *testing.T) {
	formal := regexp.MustCompile(`(^|[^A-Za-zÄÖÜäöüß])(Sie|Ihnen|Ihres|Ihrem|Ihren|Ihrer|Ihre|Ihr)($|[^A-Za-zÄÖÜäöüß])`)
	for code, definition := range catalog {
		if formal.MatchString(definition.Words.De) {
			t.Errorf("%s uses formal address: %s", code, definition.Words.De)
		}
	}
}
