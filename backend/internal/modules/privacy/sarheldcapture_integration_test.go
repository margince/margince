// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSubjectAccessIncludesHeldCaptureFactsWithinTheContentAudience(t *testing.T) {
	ctx := principal.SystemActing(context.Background(), "privacy_test")
	tx := subjectColumnsTx(ctx, t)
	subject, pair, _ := seedDedupePairs(ctx, t, tx)
	var user ids.UUID
	if err := tx.QueryRow(ctx, `SELECT split_part(captured_by,':',2)::uuid FROM dedupe_candidate WHERE id=@id`,
		pgx.StrictNamedArgs{"id": pair}).Scan(&user); err != nil {
		t.Fatal(err)
	}
	for _, audience := range []string{"workspace", "selected"} {
		activity := ids.NewV7()
		mustExec(ctx, t, tx, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by,
 audience,restricted_at,archived_at,restricted_reason,restricted_until,retention_class,retention_class_at)
 VALUES (@id,'email','Held message',now(),'manual','system:privacy_test',@audience,
 now(),now(),'controller_hold',now()+interval '100 years','commercial_correspondence',now())`, pgx.StrictNamedArgs{"id": activity, "audience": audience})
		mustExec(ctx, t, tx, `INSERT INTO capture_thread_verdict(thread_key,user_id,first_activity_id,seen_addresses)
 VALUES (@key,@user,@activity,@addresses)`, pgx.StrictNamedArgs{"key": activity.String(), "user": user,
			"activity": activity, "addresses": append(append([]string{}, subject.emails...), "other-private@else.test")})
		mustExec(ctx, t, tx, `INSERT INTO capture_party_source(activity_id,user_id,provider,sent_by_owner,containers,
 containers_known,recipients,policy_addresses,admission_blocked)
 VALUES (@activity,@user,'gmail',false,'{}',true,@addresses,@addresses,false)`,
			pgx.StrictNamedArgs{"activity": activity, "user": user,
				"addresses": append(append([]string{}, subject.emails...), "other-private@else.test")})
	}
	var pkg SARPackage
	sections, err := sarRetainedCopySections(ctx, &pkg, []ids.UUID{subject.contact.UUID}, subject.emails, nil)
	if err != nil {
		t.Fatal(err)
	}
	sections = append(sections, sarPartySourceSection(&pkg, subject.emails))
	checked := 0
	for _, section := range sections {
		if section.dest != &pkg.ThreadVerdicts && section.dest != &pkg.CapturePartySources {
			continue
		}
		checked++
		rows, err := rowMaps(ctx, tx, section.query, section.args...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("capture export rows=%d, want the workspace-held record only", len(rows))
		}
		raw, err := json.Marshal(rows)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "other-private@else.test") || !strings.Contains(string(raw), subject.emails[0]) {
			t.Fatalf("capture export crossed the address boundary: %s", raw)
		}
	}
	if checked != 2 {
		t.Fatalf("checked %d capture sections, want 2", checked)
	}
}
