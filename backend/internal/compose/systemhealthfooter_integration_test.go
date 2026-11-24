// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// A background send nobody holds for review, such as a dropped meeting
// reminder, counts in the admin's footer warning as a background send. A
// refusal that carries no decision set records nothing.
//
// Mutation: return nil from RecordUnheldRefusalTx and this fails.
func TestADroppedBackgroundSendCountsInTheFooterWarning(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	held := commsauthz.Decision{ReasonCode: commsauthz.ReasonFooterIncomplete}
	held.Evidence.FooterFindings = []commsauthz.FooterFinding{{Kind: string(messagingrules.ObjectionRoute)}}
	refusal := fmt.Errorf("staging the reminder: %w", &pendingReviewError{
		set: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{held}}, cause: apperrors.ErrConsentNotGranted,
	})
	for _, err := range []error{refusal, errors.New("a provider fault, not a refusal")} {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return commsStager{}.RecordUnheldRefusalTx(ctx, tx, err)
		}); err != nil {
			t.Fatalf("recording the dropped send: %v", err)
		}
	}

	source, findings, err := Server{jobHealthHandlers: jobHealthHandlers{pool: e.Pool}}.footerSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(findings, func(f crmcontracts.HealthFinding) bool { return f.FooterGap != nil && f.FooterGap.IsDefault })
	if i < 0 || source.Condition != healthWaitingHuman {
		t.Fatalf("no footer warning for the default entity: %+v %+v", source, findings)
	}
	gap := findings[i].FooterGap
	if gap.HeldInBackground != 1 || gap.HeldByUser != 0 || !slices.Contains(gap.Missing, crmcontracts.HealthFooterGapMissing(messagingrules.ObjectionRoute)) {
		t.Errorf("footer gap %+v, want one background send missing the objection route", gap)
	}
}
