// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedClassificationBindsFullPriorContextAndWithholdsPrivateHistory(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	ctx := e.Admin()
	system := principal.SystemActing(ctx, "system:classification-context")
	store := activities.NewStore(e.DB())
	thread := ids.NewV7().String()
	body := strings.Repeat("a", owedPriorBodyLimit+100) + "original"
	prior, _, err := store.LogActivity(system, activities.LogActivityInput{Kind: "email", Direction: new("outbound"), Body: &body, Subject: new("Earlier request"), Source: "seed", ThreadKey: thread, CounterpartyEmail: "buyer@example.test", CounterpartyOutboundAttested: true, OccurredAt: new(at.Add(-time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	source, _, err := store.LogActivity(system, activities.LogActivityInput{Kind: "email", Direction: new("inbound"), Body: new("The report is attached."), Subject: new("Monthly report"), Source: "seed", ThreadKey: thread, CounterpartyEmail: "buyer@example.test", OccurredAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	if wrote, err := store.SetOwedVerdict(system, id, activities.OwedVerdictAsksUs, "old-rules", at); err != nil || !wrote {
		t.Fatalf("seed verdict: %v %v", wrote, err)
	}
	var expected activities.OwedReviewCandidate
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		expected, err = activities.OwedReviewTx(ctx, tx, id, owedBodyLimit, owedPriorBodyLimit)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if expected.PriorID == nil || *expected.PriorID != ids.UUID(prior.Id) || !expected.ContextReadable {
		t.Fatalf("missing prior binding: %+v", expected)
	}
	changedBody := strings.Repeat("a", owedPriorBodyLimit+100) + "changed beyond prompt window"
	if _, err := store.UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(prior.Id)), activities.UpdateActivityInput{Body: &changedBody}); err != nil {
		t.Fatal(err)
	}
	err = e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := activities.ApplyReviewedOwedVerdictTx(ctx, tx, expected, activities.OwedVerdictInformsUs, "current-rules", "classification_changed", ids.NewV7(), owedBodyLimit, owedPriorBodyLimit)
		return err
	})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("changed untruncated context accepted: %v", err)
	}
	if _, err := store.SetAudience(ctx, ids.From[ids.ActivityKind](ids.UUID(prior.Id)), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		candidate, err := activities.OwedReviewTx(ctx, tx, id, owedBodyLimit, owedPriorBodyLimit)
		if err == nil && (candidate.ContextReadable || candidate.Candidate.PriorOutbound != nil) {
			t.Fatal("private preceding message was treated as absent context or exposed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
