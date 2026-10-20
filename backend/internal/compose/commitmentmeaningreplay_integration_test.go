// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNonActionRereadPreservesExistingWorkAndRenewal(t *testing.T) {
	for _, confidence := range []float64{0.75, 0.95} {
		f := setupRenewalPrivacy(t, confidence)
		assertNonActionRereadPreservesAdmission(t, f)
	}
}

func assertNonActionRereadPreservesAdmission(t *testing.T, f renewalPrivacyFixture) {
	t.Helper()
	source := ids.From[ids.ActivityKind](f.messages[0])
	before, err := f.e.Activities.GetActivity(f.e.Admin(), source, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if before.CommitmentAdmissions == nil {
		t.Fatal("fixture has no current admission")
	}
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), f.e.WS), "human:"+f.e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
	for _, state := range []string{"completed", "unclear", "request"} {
		c := f.commitment
		c.Meaning = &commitmentMeaning{State: state, DeadlinePrecision: "none"}
		c.AdmissionSourceID, c.RenewedAt, c.RenewalEvidence = nil, nil, nil
		c.AsOf, c.Summary, c.DueDate = c.AsOf.Add(time.Hour), "Later interpretation", ""
		if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
			if err == nil && outcome != CommitmentRemembered {
				t.Errorf("%s reread = %s", state, outcome)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		after, err := f.e.Activities.GetActivity(f.e.Admin(), source, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before.CommitmentAdmissions, after.CommitmentAdmissions) {
			t.Fatalf("%s reread changed the existing admission", state)
		}
	}
}
