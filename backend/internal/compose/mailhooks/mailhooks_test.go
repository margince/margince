// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailhooks

import (
	"context"
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
	return extension.Extension{
		Name:      extension.Name(name),
		MailHooks: extension.MailHooks{Parties: hook, Reasons: []string{"declared"}},
	}
}

// addCompany is a parties hook that adds one company and names a reason.
func addCompany(company ids.UUID, reason string, seen *[]string, tag string) extension.MailPartiesHook {
	return func(_ context.Context, _ extension.MailView, p extension.MailParties) (extension.MailParties, error) {
		*seen = append(*seen, tag)
		p.Reason = reason
		p.Companies = append(append([]string{}, p.Companies...), company.String())
		return p, nil
	}
}

func TestHooksRunInUnitNameOrderAndEachSeesThePreviousResult(t *testing.T) {
	var seen []string
	first, second := ids.NewV7(), ids.NewV7()
	h := New(served(
		partiesUnit("b-unit", addCompany(second, "declared", &seen, "b")),
		partiesUnit("a-unit", addCompany(first, "declared", &seen, "a")),
	), nil)
	out, changes := h.Parties(context.Background(), capture.MailView{}, capture.MailParties{})
	if len(seen) != 2 || seen[0] != "a" || seen[1] != "b" {
		t.Fatalf("order = %v, want a then b", seen)
	}
	if len(out.Companies) != 2 || out.Companies[0] != first || out.Companies[1] != second || len(changes) != 2 {
		t.Fatalf("each hook must see the previous result: %v %v", out, changes)
	}
}

func TestAChangeThatNamesNoDeclaredReasonIsIgnored(t *testing.T) {
	var seen []string
	company := ids.NewV7()
	for _, reason := range []string{"", "made_up"} {
		out, changes := New(served(partiesUnit("u", addCompany(company, reason, &seen, "u"))), nil).
			Parties(context.Background(), capture.MailView{}, capture.MailParties{})
		if len(out.Companies) != 0 {
			t.Errorf("reason %q: an undeclared reason must not change the result, got %v", reason, out)
		}
		if len(changes) != 1 || changes[0].Result != capture.HookIgnored || changes[0].Reason != reason {
			t.Errorf("reason %q: want one ignored change, got %v", reason, changes)
		}
	}
}

func TestAFailingPanickingOrSlowHookDecidesNothingAndStopsTheChain(t *testing.T) {
	id := ids.NewV7()
	release := make(chan struct{})
	defer close(release)
	in := capture.MailParties{Contacts: []ids.UUID{id}}
	for name, hook := range map[string]extension.MailPartiesHook{
		"error": func(context.Context, extension.MailView, extension.MailParties) (extension.MailParties, error) {
			return extension.MailParties{}, errors.New("no")
		},
		"panic": func(context.Context, extension.MailView, extension.MailParties) (extension.MailParties, error) {
			panic("boom")
		},
		"slow": func(ctx context.Context, _ extension.MailView, _ extension.MailParties) (extension.MailParties, error) {
			<-ctx.Done()
			return extension.MailParties{}, ctx.Err()
		},
		"deaf": func(context.Context, extension.MailView, extension.MailParties) (extension.MailParties, error) {
			<-release
			return extension.MailParties{}, nil
		},
	} {
		var later []string
		units := served(partiesUnit("a", hook), partiesUnit("b", addCompany(ids.NewV7(), "declared", &later, "b")))
		out, changes := New(units, nil).Parties(context.Background(), capture.MailView{}, in)
		if len(out.Contacts) != 1 || out.Contacts[0] != id {
			t.Errorf("%s: result changed: %v", name, out)
		}
		if len(changes) != 1 || changes[0].Result != capture.HookFailed || changes[0].Unit != "a" {
			t.Errorf("%s: want one failed change by a, got %v", name, changes)
		}
		if len(later) != 0 {
			t.Errorf("%s: a later hook ran after the failure: %v", name, later)
		}
	}
}

func TestNoUnitMeansNoHooksActive(t *testing.T) {
	if New(served(), nil).Active() {
		t.Fatal("Active with no unit")
	}
	deal := func(context.Context, extension.MailView, extension.MailDeal) (extension.MailDeal, error) {
		return extension.MailDeal{}, nil
	}
	unit := extension.Extension{Name: "d", MailHooks: extension.MailHooks{Deal: deal, Reasons: []string{"declared"}}}
	if !New(served(partiesUnit("u", nil), unit), nil).Active() {
		t.Fatal("not Active with a deal hook")
	}
}
