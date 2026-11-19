// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailhooks

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func served(units ...extension.Extension) func() []extension.Extension {
	return func() []extension.Extension { return units }
}

func partiesUnit(name string, hook extension.MailPartiesHook) extension.Extension {
	return extension.Extension{Name: extension.Name(name), MailHooks: extension.MailHooks{Parties: hook}}
}

func TestHooksRunInUnitNameOrderAndAnUnreadableAnswerChangesNothing(t *testing.T) {
	var seen []string
	step := func(tag string) extension.MailPartiesHook {
		return func(_ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
			seen = append(seen, tag)
			p.Reason = tag
			p.Companies = append(append([]string{}, p.Companies...), tag)
			return p, nil
		}
	}
	h := New(served(partiesUnit("b-unit", step("b")), partiesUnit("a-unit", step("a"))))
	_, changes := h.Parties(capture.MailView{}, capture.MailParties{})
	if len(seen) != 2 || seen[0] != "a" || seen[1] != "b" {
		t.Fatalf("order = %v, want a then b", seen)
	}
	if len(changes) != 0 {
		t.Fatalf("a hook answering a non-id must change nothing, got %v", changes)
	}
}

func TestAFailingPanickingOrSlowHookLeavesTheResultAlone(t *testing.T) {
	id := ids.NewV7()
	release := make(chan struct{})
	defer close(release)
	in := capture.MailParties{Contacts: []ids.UUID{id}}
	for name, hook := range map[string]extension.MailPartiesHook{
		"error": func(extension.MailView, extension.MailParties) (extension.MailParties, error) {
			return extension.MailParties{}, errors.New("no")
		},
		"panic": func(extension.MailView, extension.MailParties) (extension.MailParties, error) { panic("boom") },
		"slow": func(extension.MailView, extension.MailParties) (extension.MailParties, error) {
			<-release
			return extension.MailParties{}, nil
		},
	} {
		out, changes := New(served(partiesUnit("u", hook))).Parties(capture.MailView{}, in)
		if len(changes) != 0 || len(out.Contacts) != 1 || out.Contacts[0] != id {
			t.Errorf("%s: result changed: %v %v", name, out, changes)
		}
	}
}

func TestNoUnitMeansNoHooksActive(t *testing.T) {
	if New(served()).Active() {
		t.Fatal("Active with no unit")
	}
	if !New(served(partiesUnit("u", nil), extension.Extension{Name: "d", MailHooks: extension.MailHooks{Deal: func(extension.MailView, extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{}, nil
	}}})).Active() {
		t.Fatal("not Active with a deal hook")
	}
}
