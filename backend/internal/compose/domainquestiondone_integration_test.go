// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDomainQuestionDeferralPreservesTheReviewDestination(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	const domain = "deferred-question.test"
	openTriageQuestion(t, e, domain, "hello@"+domain, "Sender")
	if _, err := e.Contacts.ResolveUnreadableDomainTriage(ctx, contacts.ResolveDomainTriageInput{Domain: domain}); err != nil {
		t.Fatal(err)
	}
	at := clocktest.Now(t).Add(time.Hour)
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	ref := attention.RowRef{Source: "domain_question", RowID: domain}
	if err := service.MarkRowsDone(e.As(e.Rep2, nil, integration.AdminPerms), []attention.RowRef{ref}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("another reader deferred private question: %v", err)
	}
	if err := service.MarkRowsDone(ctx, []attention.RowRef{ref}); err != nil {
		t.Fatal(err)
	}
	day, err := service.Worklist(ctx, "mine", "", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range day.Queue {
		if row.Source == "domain_question" && row.Id == domain {
			t.Fatal("deferred question remains in Worklist")
		}
	}
	review, _, err := e.Contacts.DomainQuestionsForOwner(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(review) != 1 || review[0].Domain != domain {
		t.Fatalf("deferred review = %+v", review)
	}
	if err := service.ReopenRows(ctx, []attention.RowRef{ref}); err != nil {
		t.Fatal(err)
	}
	day, err = service.Worklist(ctx, "mine", "", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range day.Queue {
		if row.Source == "domain_question" && row.Id == domain {
			return
		}
	}
	t.Fatal("restored question is absent from Worklist")
}
