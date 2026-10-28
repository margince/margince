// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/techprofile"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Each change as the contacts lane builds it. A move names what it left, and
// a departure names its value as both the current and the previous one.
var technicalChangesTheLaneBuilds = map[string]contacts.TechnicalChange{
	"mail moved": {Field: contacts.FactMailProvider, Kind: contacts.TechnicalMoved,
		Value: "Microsoft 365", ValueKey: "microsoft365", Previous: "Own mail server", PreviousKey: techprofile.MailSelfHosted},
	"mail set": {Field: contacts.FactMailProvider, Kind: contacts.TechnicalAppeared,
		Value: "Another provider", ValueKey: techprofile.MailOther},
	"mail gone": {Field: contacts.FactMailProvider, Kind: contacts.TechnicalGone,
		Value: "Google Workspace", ValueKey: "google", Previous: "Google Workspace", PreviousKey: "google"},
	"service new": {Field: contacts.FactOperatedService, Kind: contacts.TechnicalAppeared,
		Value: "Careers page", ValueKey: techprofile.ServiceCareers},
	"service gone": {Field: contacts.FactOperatedService, Kind: contacts.TechnicalGone,
		Value: "Webshop", ValueKey: techprofile.ServiceWebshop, Previous: "Webshop", PreviousKey: techprofile.ServiceWebshop},
	"hosting moved": {Field: contacts.FactHostingProvider, Kind: contacts.TechnicalMoved,
		Value: "Hetzner", ValueKey: "hetzner", Previous: "AWS", PreviousKey: "aws"},
	"hosting set": {Field: contacts.FactHostingProvider, Kind: contacts.TechnicalAppeared,
		Value: "Hetzner", ValueKey: "hetzner"},
	"hosting gone": {Field: contacts.FactHostingProvider, Kind: contacts.TechnicalGone,
		Value: "AWS", ValueKey: "aws", Previous: "AWS", PreviousKey: "aws"},
	"technology new": {Field: contacts.FactTechnology, Kind: contacts.TechnicalAppeared,
		Value: "nginx", ValueKey: "nginx"},
	"technology gone": {Field: contacts.FactTechnology, Kind: contacts.TechnicalGone,
		Value: "WordPress", ValueKey: "wordpress", Previous: "WordPress", PreviousKey: "wordpress"},
}

// The reader words the stored parts through the writer's own function, so in
// the base language it reproduces the stored sentence to the letter.
func TestATechnicalChangeReadsBackInTheBaseLanguageAsItWasStored(t *testing.T) {
	for name, change := range technicalChangesTheLaneBuilds {
		parts := technicalPartsOf(change)
		if !parts.complete() {
			t.Errorf("%s: the reader would refuse parts the writer stores", name)
		}
		for _, base := range textlang.Shipped {
			stored, ok := parts.worded(base)
			if !ok {
				t.Fatalf("%s: the writer files no signal for it", name)
			}
			if got := rereadSummary(t, base, kindTechnicalChange, parts); got != stored {
				t.Errorf("%s in %s reads back %q, stored %q", name, base, got, stored)
			}
		}
	}
}

func TestAGhostedOrQuietSignalReadsBackInTheBaseLanguageAsItWasStored(t *testing.T) {
	ghosted := ghostedThreadParts{Days: ghostedThresholdDays}
	quiet := projectQuietParts{Project: "ERP rollout", Days: 30}
	if !ghosted.complete() || !quiet.complete() {
		t.Fatal("the reader would refuse parts the scan stores")
	}
	for _, base := range textlang.Shipped {
		if got, stored := rereadSummary(t, base, kindGhostedThread, ghosted), ghosted.worded(base); got != stored {
			t.Errorf("ghosted in %s reads back %q, stored %q", base, got, stored)
		}
		if got, stored := rereadSummary(t, base, kindProjectGoneQuiet, quiet), quiet.worded(base); got != stored {
			t.Errorf("quiet project in %s reads back %q, stored %q", base, got, stored)
		}
	}
}

// A departed provider says it left, never that it is where the company runs.
func TestADepartedProviderReadsAsGone(t *testing.T) {
	for name, want := range map[string]string{
		"mail gone":    "Mail no longer runs on Google Workspace",
		"hosting gone": "No longer hosted at AWS",
	} {
		if got, _ := technicalPartsOf(technicalChangesTheLaneBuilds[name]).worded(textlang.English); got != want {
			t.Errorf("%s reads %q, want %q", name, got, want)
		}
	}
}

func TestASignalReadsItsPartsInAnotherLanguage(t *testing.T) {
	for _, c := range []struct {
		kind, params, want string
	}{
		{kindTechnicalChange, `{"field":"mail_provider","kind":"moved","value":"Microsoft 365","value_key":"microsoft365","previous":"Own mail server","previous_key":"self_hosted"}`,
			"Mail hiện chạy trên Microsoft 365 (trước đây là máy chủ mail riêng)"},
		{kindGhostedThread, `{"days":14}`, "Chúng ta đã viết cách đây ít nhất 14 ngày và chưa ai trả lời."},
		{kindProjectGoneQuiet, `{"project":"ERP rollout","days":30}`, "Không có gì được lưu cho ERP rollout trong ít nhất 30 ngày."},
	} {
		if got, ok := signalSummaryWords(textlang.Vietnamese, c.kind, json.RawMessage(c.params)); !ok || got != c.want {
			t.Errorf("%s reads %q (%v), want %q", c.kind, got, ok, c.want)
		}
	}
}

// Parts the words do not know are refused, so the reader keeps the stored
// sentence rather than one with a hole in it.
func TestPartsTheWordsDoNotKnowAreRefused(t *testing.T) {
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
		if got, ok := signalSummaryWords(textlang.Vietnamese, c.kind, json.RawMessage(c.params)); ok {
			t.Errorf("%s was worded as %q, want it refused", name, got)
		}
	}
}

func rereadSummary[P technicalChangeParts | ghostedThreadParts | projectQuietParts](
	t *testing.T, lang textlang.Lang, kind string, parts P,
) string {
	t.Helper()
	params, err := json.Marshal(parts)
	if err != nil {
		t.Fatalf("encoding the parts: %v", err)
	}
	got, ok := signalSummaryWords(lang, kind, params)
	if !ok {
		t.Fatalf("the reader refused the %s parts the writer stores: %s", kind, params)
	}
	return got
}
