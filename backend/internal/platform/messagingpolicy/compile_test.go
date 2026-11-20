// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingpolicy

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"unicode"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

const pairKeys = `fixtures:
  - finding: objection_route
    triggers: fixtures/a.md
    clears: fixtures/b.md
`

const draft = "---\nadvertising: true\n---\nHello\n"

// policyFS is a one-country policy whose country layer front matter is front.
func policyFS(front string) fstest.MapFS {
	return fstest.MapFS{
		"index.md":      {Data: []byte("---\nlayers: [aa.md]\n---\nThe layers.\n")},
		"aa.md":         {Data: []byte("---\n" + front + pairKeys + "---\nWhy.\n")},
		"fixtures/a.md": {Data: []byte(draft)},
		"fixtures/b.md": {Data: []byte(draft)},
	}
}

const country = "jurisdiction: aa\nversion: 3\ncreated: 2026-01-02\n"

func TestACountryLayerCompilesToItsRules(t *testing.T) {
	policy, err := Compile(policyFS(country + "reply-window: 720h\nconsent-only: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := policy.Rules
	if r.Jurisdiction != "aa" || r.Version != 3 || !r.ConsentOnly || r.ReplyWindow.Hours() != 720 {
		t.Errorf("compiled %+v", r)
	}
	if len(r.Digest) != 64 || len(policy.Fixtures) != 1 || policy.Fixtures[0].Triggers.Body != "Hello\n" {
		t.Errorf("digest %q, fixtures %+v", r.Digest, policy.Fixtures)
	}
}

func TestTheSourceIsRefused(t *testing.T) {
	for name, front := range map[string]string{
		"an unknown key":            country + "reply-windw: 720h\n",
		"no version":                "jurisdiction: aa\ncreated: 2026-01-02\n",
		"a misnamed country":        "jurisdiction: bb\nversion: 1\ncreated: 2026-01-02\n",
		"an invalid obligation":     country + "frequency-cap: {messages: 0, window: 24h}\n",
		"a prose territorial test":  country + reviewed + citation("the sender is in the EU"),
		"no territorial test":       country + reviewed + "citations: [{code: frequency_cap_reached, instrument: Act A}]\n",
		"an undeclared instrument":  country + reviewed + "citations: [{code: c, territorial-test: any_sender, instrument: Act Z}]\n",
		"a citation never reviewed": country + citation("any_sender"),
	} {
		if _, err := Compile(policyFS(front)); err == nil {
			t.Errorf("%s compiled", name)
		}
	}
}

const reviewed = "source-checked: 2026-01-03\nreviewed: 2026-01-04\nreviewer: maintainers\n" +
	"instruments: [{name: Act A, provision: §1, source-url: https://example.org/a}]\n"

func citation(territory string) string {
	return "citations: [{code: frequency_cap_reached, instrument: Act A, territorial-test: " + territory + "}]\n"
}

func TestADecidableTerritorialTestSelectsTheCitation(t *testing.T) {
	policy, err := Compile(policyFS(country + reviewed + citation("sender_elsewhere")))
	if err != nil {
		t.Fatal(err)
	}
	if got := policy.Rules.Citations; len(got) != 1 || got[0].Territory != messaging.SenderElsewhere {
		t.Errorf("citations %+v", got)
	}
}

func TestALaterLayerMayOnlyTighten(t *testing.T) {
	layered := func(fallback, country string) fstest.MapFS {
		fsys := policyFS(country)
		fsys["index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [fallback.md, aa.md]\n---\nThe layers.\n")}
		fsys["fallback.md"] = &fstest.MapFile{Data: []byte("---\n" + fallback + pairKeys + "---\nWhy.\n")}
		return fsys
	}
	policy, err := Compile(layered("reply-window: 720h\n", country+"consent-only: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if policy.Rules.ReplyWindow.Hours() != 720 || !policy.Rules.ConsentOnly {
		t.Errorf("the country layer did not keep the fallback's window and add its own key: %+v", policy.Rules)
	}
	_, err = Compile(layered("reply-window: 720h\n", country+"reply-window: 8760h\n"))
	if err == nil || !strings.Contains(err.Error(), "ReplyWindow") {
		t.Errorf("a longer window over the fallback compiled: %v", err)
	}
}

func TestTheIndexListsOneCountryInOrder(t *testing.T) {
	for name, layers := range map[string]string{
		"no country":           "[fallback.md]",
		"two countries":        "[aa.md, bb.md]",
		"a fallback last":      "[aa.md, fallback.md]",
		"a research note":      "[research/xx.md, aa.md]",
		"a layer that escapes": "[../aa.md]",
	} {
		fsys := policyFS(country)
		fsys["index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: " + layers + "\n---\nThe layers.\n")}
		if _, err := Compile(fsys); err == nil {
			t.Errorf("an index with %s compiled", name)
		}
	}
}

func TestTheIndexAccountsForEveryPolicyFile(t *testing.T) {
	for name, change := range map[string]func(fstest.MapFS){
		"a layer listed twice": func(fsys fstest.MapFS) {
			fsys["regional/eu.md"] = fsys["aa.md"]
			fsys["index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [regional/eu.md, regional/eu.md, aa.md]\n---\nThe layers.\n")}
		},
		"an unlisted layer": func(fsys fstest.MapFS) { fsys["regional/eu.md"] = fsys["aa.md"] },
		"an empty index body": func(fsys fstest.MapFS) {
			fsys["index.md"] = &fstest.MapFile{Data: []byte("---\nlayers: [aa.md]\n---\n")}
		},
	} {
		fsys := policyFS(country)
		change(fsys)
		if _, err := Compile(fsys); err == nil {
			t.Errorf("a policy folder with %s compiled", name)
		}
	}
	fsys := policyFS(country)
	fsys["research/bb.md"] = &fstest.MapFile{Data: []byte("Notes on another market.\n")}
	if _, err := Compile(fsys); err != nil {
		t.Errorf("a research note stopped the policy compiling: %v", err)
	}
}

func TestAPairNeedsBothDrafts(t *testing.T) {
	fsys := policyFS(country)
	delete(fsys, "fixtures/b.md")
	if _, err := Compile(fsys); err == nil {
		t.Error("a pair with one draft compiled")
	}
}

func TestTheDigestCoversTheExplanation(t *testing.T) {
	before, err := Compile(policyFS(country))
	if err != nil {
		t.Fatal(err)
	}
	fsys := policyFS(country)
	fsys["aa.md"].Data = []byte(strings.Replace(string(fsys["aa.md"].Data), "Why.", "Why not.", 1))
	after, err := Compile(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if before.Rules.Digest == after.Rules.Digest {
		t.Error("an edit to the explanation left the digest unchanged")
	}
}

// Every obligation a pack can declare has its key. Identity and provenance
// are stated by their own keys, and the generator stamps the digest.
func TestEveryRulesFieldHasAKey(t *testing.T) {
	keys := map[string]bool{}
	layer := reflect.TypeFor[layerFile]()
	for field := range layer.Fields() {
		keys[field.Tag.Get("yaml")] = true
	}
	rules := reflect.TypeFor[messaging.Rules]()
	for field := range rules.Fields() {
		name := field.Name
		if name == "Provenance" || name == "Digest" {
			continue
		}
		if key := hyphenated(name); !keys[key] {
			t.Errorf("messaging.Rules.%s has no front-matter key %q", name, key)
		}
	}
}

// hyphenated spells a Go field name as its key: OptOutAcknowledgementUTCOffset
// is opt-out-acknowledgement-utc-offset.
func hyphenated(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		startsWord := i > 0 && unicode.IsUpper(r) &&
			(unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])))
		if startsWord {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
