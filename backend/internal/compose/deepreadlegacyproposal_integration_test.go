// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The "deepread" approval the confirm-first path used to write, staged by hand
// so the accept effect still has a subject.
//
// A fixture here would have been wrong a day ago and is right now, and the
// difference is worth stating rather than assuming. The read no longer stages
// what it finds — it applies directly — so nothing in production writes this
// kind any more. What the accept tests hold is the narrower promise that rows
// staged BEFORE that change are still sitting in inboxes and must still decide
// correctly. Their subject is therefore a row production cannot produce today,
// and seeding it is the only way to have one. When the last of those rows ages
// out and the effect goes, this goes with it.
//
// It stages through the approvals writer that owns the table rather than an
// INSERT of this file's own: a decision walks the row the writer produces —
// canonical identity, diff hash, bundle — and a hand-rolled row that merely
// looked like one would prove nothing about the path under test.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func stageLegacyDeepReadProposal(
	t *testing.T,
	e *integration.Env,
	svc *approvals.Service,
	org ids.UUID,
	readID ids.UUID,
	fields []people.DeepReadField,
	facts []people.DeepReadFact,
) ids.UUID {
	t.Helper()
	proposedChange, err := json.Marshal(people.DeepReadProposal{
		OrganizationID: ids.From[ids.OrganizationKind](org),
		SourceURL:      seedURL,
		SiteReadID:     readID,
		Fields:         fields,
		Facts:          facts,
	})
	if err != nil {
		t.Fatalf("marshalling the legacy proposal: %v", err)
	}
	digest := sha256.Sum256(proposedChange)
	var approvalID ids.ApprovalID
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		approvalID, err = svc.StageOrJoinPendingInTx(ctx, tx, approvals.StageInput{
			Kind:           deepReadProposalKind,
			ProposedChange: proposedChange,
			DiffHash:       hex.EncodeToString(digest[:]),
			TargetType:     enrichTargetType,
			TargetID:       org,
			Summary:        "Deep site read of " + seedURL,
			JoinPending:    true,
			BundleID:       ids.NewV7(),
		})
		return err
	}); err != nil {
		t.Fatalf("staging the legacy proposal: %v", err)
	}
	return approvalID.UUID
}
