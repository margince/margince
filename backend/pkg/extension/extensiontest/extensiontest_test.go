// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extensiontest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// failures records what a check reports instead of failing the test that runs it.
type failures struct {
	testing.TB
	reported []string
}

func (f *failures) Helper() {}

func (f *failures) Errorf(format string, args ...any) {
	f.reported = append(f.reported, fmt.Sprintf(format, args...))
}

func (f *failures) said(t *testing.T, want string) {
	t.Helper()
	if want == "" && len(f.reported) != 0 || want != "" && (len(f.reported) != 1 || !strings.Contains(f.reported[0], want)) {
		t.Errorf("reported %q, want one naming %q", f.reported, want)
	}
}

func demo() extension.Extension {
	return extension.Extension{
		Name: "demo",
		RecordKinds: []extension.RecordKindDeclaration{{
			Kind: "visit", DisplayName: "Visit", Table: "ext_demo_visit",
			Fields: []extension.RecordKindField{{Name: "code", Label: "Code"}}, NaturalKey: []string{"code"},
		}},
		LinkTypes: []extension.LinkType{{Type: "follows", Label: "Follows", From: []string{"deal"}}},
		SignalTypes: []extension.SignalTypeDeclaration{{Kind: "loss", Label: "Loss", Evidence: []extension.SignalEvidenceField{
			{Name: "amount", Label: "Amount", Type: extension.SignalFieldNumber, Required: true},
		}}},
	}
}

func TestCheckReportsWhatTheBootRefuses(t *testing.T) {
	twice := demo()
	twice.LinkTypes = append(twice.LinkTypes, twice.LinkTypes[0])
	noKey := demo()
	noKey.RecordKinds[0].NaturalKey = nil
	for name, c := range map[string]struct {
		ext  extension.Extension
		want string
	}{
		"valid":              {demo(), ""},
		"a link type twice":  {twice, `declares link type "follows" twice`},
		"a kind with no key": {noKey, "natural key"},
	} {
		got := &failures{}
		Check(got, c.ext)
		t.Run(name, func(t *testing.T) { got.said(t, c.want) })
	}
}

func TestCheckSignalReadsTheDeclaredLayout(t *testing.T) {
	for name, c := range map[string]struct {
		kind   string
		fields map[string]any
		want   string
	}{
		"fits":           {"loss", map[string]any{"amount": 12.5}, ""},
		"missing member": {"loss", map[string]any{}, "required"},
		"wrong type":     {"loss", map[string]any{"amount": "a lot"}, "must be a number"},
		"undeclared":     {"gain", map[string]any{}, "no signal type"},
	} {
		got := &failures{}
		CheckSignal(got, demo(), c.kind, c.fields)
		t.Run(name, func(t *testing.T) { got.said(t, c.want) })
	}
}

func TestCheckLinkReadsTheDeclaredEnds(t *testing.T) {
	for name, c := range map[string]struct {
		linkType, from, to, want string
	}{
		"declared ends":        {"follows", "deal", "company", ""},
		"a start it narrowed":  {"follows", "contact", "deal", "does not join"},
		"not a record at all":  {"follows", "deal", "project", "does not join"},
		"undeclared link type": {"precedes", "deal", "deal", "no link type"},
	} {
		got := &failures{}
		CheckLink(got, demo(), c.linkType, c.from, c.to)
		t.Run(name, func(t *testing.T) { got.said(t, c.want) })
	}
}
