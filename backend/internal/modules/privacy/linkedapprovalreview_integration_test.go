// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDecidedApproval writes a decided approval with one older audit image
// that still names someone. A nil target leaves both target columns empty.
func seedDecidedApproval(ctx context.Context, t *testing.T, tx pgx.Tx, summary, targetType string, target *ids.UUID) (approval, audit ids.UUID) {
	t.Helper()
	approval = ids.NewV7()
	var kind *string
	if target != nil {
		kind = &targetType
	}
	mustExec(ctx, t, tx, `INSERT INTO approval(id,kind,status,proposed_change,diff_hash,expires_at,proposed_by,summary,target_entity_type,target_entity_id,decided_at,consumed_at)
 VALUES(@id,'contact_update','approved','{}','legacy',timestamptz '2030-01-01','system:legacy',@summary,@kind,@target,now(),now())`,
		pgx.StrictNamedArgs{"id": approval, "summary": summary, "kind": kind, "target": target})
	return approval, seedPersonalAudit(ctx, t, tx, approvalEntity, approval)
}

func TestTheReviewPageNamesUnprovenApprovalsAndCorrectsProvedOnes(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, _, _ := seedDedupePairs(ctx, t, tx)
	open, held := ids.NewV7(), ids.NewV7()
	mustExec(ctx, t, tx, `INSERT INTO company(id,display_name,source,captured_by,legal_hold) VALUES($1,'Open Ltd','manual','human:x',false),($2,'Held Ltd','manual','human:x',true)`, open, held)

	unproven, _ := seedDecidedApproval(ctx, t, tx, "", "company", &open)
	untargeted, _ := seedDecidedApproval(ctx, t, tx, "", "", nil)
	underHold, _ := seedDecidedApproval(ctx, t, tx, "", "company", &held)
	stillWorded, _ := seedDecidedApproval(ctx, t, tx, "Call Private Contact", "company", &open)
	linked, linkedAudit := seedDecidedApproval(ctx, t, tx, "", "contact", &subject.contact.UUID)
	legacyContactProof(ctx, t, tx, subject.contact.UUID)

	page, err := UnlinkedApprovalHistoryPageTx(ctx, tx, ids.UUID{}, ids.NewV7(), 10_000, nil)
	if err != nil {
		t.Fatal(err)
	}
	named := map[ids.UUID]int{}
	for _, source := range page.Review {
		if source.Kind != ApprovalHistoryReview {
			t.Fatalf("named a %q source", source.Kind)
		}
		named[source.ID]++
	}
	for _, want := range []ids.UUID{unproven, untargeted} {
		if named[want] != 1 {
			t.Errorf("approval %s named %d times, want once", want, named[want])
		}
	}
	for _, skipped := range []ids.UUID{underHold, stillWorded, linked} {
		if named[skipped] != 0 {
			t.Errorf("approval %s was named for review", skipped)
		}
	}
	if page.Changed < 1 {
		t.Fatal("a link proved after the correction step must be corrected here")
	}
	assertLegacyAuditImage(ctx, t, tx, linkedAudit, false)
}
