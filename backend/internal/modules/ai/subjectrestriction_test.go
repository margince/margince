// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// restrictedOnly answers a subject restricted when it names the one contact.
type restrictedOnly struct {
	contact ids.UUID
	asked   int
}

func (r *restrictedOnly) AnyRestricted(_ context.Context, s model.Subjects) (bool, error) {
	r.asked++
	return slices.Contains(s.Contacts, r.contact), nil
}

func restrictionRouter(check model.SubjectRestrictions) (*Router, *FakeClient) {
	cheap := NewFakeClient().Script("answer", "answer", "answer")
	r := testRouter(map[Tier]model.Client{TierCheapCloud: cheap, TierPremium: cheap}, &memMeter{}, DefaultMonthlyTokens, ProfileEUHosted)
	return r.WithSubjectRestrictions(check), cheap
}

func TestAnHonouringTaskRefusesARestrictedContactBeforeAnyProvider(t *testing.T) {
	anna := ids.NewV7()
	r, provider := restrictionRouter(&restrictedOnly{contact: anna})
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}, Subjects: model.AboutContacts(anna)}

	_, _, completeErr := r.Complete(wsContext(t), TaskSummarize, req)
	_, _, structuredErr := r.CompleteStructured(wsContext(t), TaskSummarize, req, func(string) error { return nil })
	for name, err := range map[string]error{"Complete": completeErr, "CompleteStructured": structuredErr} {
		if !errors.Is(err, ErrSubjectRestricted) {
			t.Errorf("%s answered %v, want ErrSubjectRestricted", name, err)
		}
	}
	if calls := len(provider.Calls()); calls != 0 {
		t.Fatalf("the provider was asked %d time(s) about a restricted contact", calls)
	}
}

func TestAnHonouringTaskRefusesARequestThatDeclaresNoSubjects(t *testing.T) {
	r, provider := restrictionRouter(&restrictedOnly{})
	_, _, err := r.Complete(wsContext(t), TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}})
	if !errors.Is(err, ErrSubjectsUndeclared) {
		t.Fatalf("an undeclared request answered %v, want ErrSubjectsUndeclared", err)
	}
	if len(provider.Calls()) != 0 {
		t.Fatal("the provider was asked for an undeclared request")
	}
}

func TestAnUnrestrictedSubjectIsServed(t *testing.T) {
	r, _ := restrictionRouter(&restrictedOnly{contact: ids.NewV7()})
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}, Subjects: model.AboutContacts(ids.NewV7())}
	if _, _, err := r.Complete(wsContext(t), TaskSummarize, req); err != nil {
		t.Fatalf("an unrestricted contact was refused: %v", err)
	}
}

func TestATaskThatDoesNotHonourRestrictionsIsNotAsked(t *testing.T) {
	check := &restrictedOnly{}
	r, _ := restrictionRouter(check)
	if HonoursRestriction(TaskNlSearch) {
		t.Fatal("the fixture needs a task that does not honour restrictions")
	}
	if _, _, err := r.Complete(wsContext(t), TaskNlSearch, model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}}); err != nil {
		t.Fatal(err)
	}
	if check.asked != 0 {
		t.Fatalf("the restriction check was asked %d time(s) for a task that reads no contact", check.asked)
	}
}

func TestAProductionRouterWithNoCheckWiredRefuses(t *testing.T) {
	r, provider := restrictionRouter(nil)
	r.restrictionsRequired = true
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}, Subjects: model.AboutContacts(ids.NewV7())}
	if _, _, err := r.Complete(wsContext(t), TaskSummarize, req); !errors.Is(err, ErrSubjectRestricted) {
		t.Fatalf("a router with no check answered %v, want a refusal", err)
	}
	if len(provider.Calls()) != 0 {
		t.Fatal("the provider was asked with no restriction check wired")
	}
}

func TestADecisionSiteRefusesARestrictedContactBeforeAnyProvider(t *testing.T) {
	anna := ids.NewV7()
	r, provider := restrictionRouter(&restrictedOnly{contact: anna})
	req := model.Request{Messages: []model.Message{{Role: "user", Content: "who wrote"}}, Subjects: model.AboutContacts(anna)}
	_, _, err := r.Decide(wsContext(t), TaskCaptureCounterpartyVerdict, "verdict", decision.Request{}, req,
		func(string) error { return nil }, nil)
	if !errors.Is(err, ErrSubjectRestricted) {
		t.Fatalf("Decide answered %v, want ErrSubjectRestricted", err)
	}
	if len(provider.Calls()) != 0 {
		t.Fatal("the provider was asked about a restricted contact")
	}
}

func TestAnEmptySubjectSetIsRefusedUnlessItSaysWhyNoContactIsInvolved(t *testing.T) {
	r, provider := restrictionRouter(&restrictedOnly{})
	empty := model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}, Subjects: &model.Subjects{}}
	if _, _, err := r.Complete(wsContext(t), TaskSummarize, empty); !errors.Is(err, ErrSubjectsUndeclared) {
		t.Fatalf("an empty subject set answered %v, want ErrSubjectsUndeclared", err)
	}
	if len(provider.Calls()) != 0 {
		t.Fatal("the provider was asked for an empty subject set")
	}
	marked := empty
	marked.Subjects = model.AboutNoContact("the company's own recorded facts")
	if _, _, err := r.Complete(wsContext(t), TaskSummarize, marked); err != nil {
		t.Fatalf("a request that says why no contact is involved was refused: %v", err)
	}
}

func TestTheRecordASiteNamedStandsInForUndeclaredSubjects(t *testing.T) {
	anna := ids.NewV7()
	r, provider := restrictionRouter(&restrictedOnly{contact: anna})
	ctx := WithSubject(wsContext(t), ids.From[ids.ContactKind](anna).Ref(), "Anna")
	_, _, err := r.Complete(ctx, TaskSummarize, model.Request{Messages: []model.Message{{Role: "user", Content: "brief"}}})
	if !errors.Is(err, ErrSubjectRestricted) {
		t.Fatalf("a call about a restricted contact answered %v, want ErrSubjectRestricted", err)
	}
	if len(provider.Calls()) != 0 {
		t.Fatal("the provider was asked about a restricted contact")
	}
}
