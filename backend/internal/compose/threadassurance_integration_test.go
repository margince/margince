// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestUnavailableDealEvidenceDefersAssessmentWithoutClosingFindings(t *testing.T) {
	for _, kind := range []string{"refusal", "meeting", "task"} {
		t.Run(kind, func(t *testing.T) {
			v := setupViability(t)
			if _, err := v.Contacts.UpdateContact(v.Admin(), ids.From[ids.ContactKind](v.buyer), contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: "buyer@assurance.example", IsPrimary: true, EmailType: "work"}}}); err != nil {
				t.Fatal(err)
			}
			v.check(t)
			const key = "assurance-unavailable-evidence"
			subject := "Customer evidence"
			at := v.now.Add(time.Hour)
			input := activities.LogActivityInput{Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual", ThreadKey: key, Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: v.silent}, {EntityType: "contact", EntityID: v.buyer}}}
			if kind == "refusal" {
				input.Kind = "email"
				input.Direction = new("inbound")
				input.EmailFrom = "buyer@assurance.example"
				at = v.now.Add(-time.Hour)
			}
			activity, _, err := v.Activities.LogActivity(v.Admin(), input)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "refusal" {
				if applied, err := v.Activities.SetReplyVerdict(v.Admin(), ids.UUID(activity.Id), activities.ReplyVerdictNegative, "test"); err != nil || !applied {
					t.Fatalf("refusal verdict applied=%v: %v", applied, err)
				}
			}
			v.check(t)
			snapshot := v.WsScalar(t, `SELECT coalesce(string_agg(id::text||':'||status,',' ORDER BY id),'') FROM assurance_exception WHERE subject_id=$1`, v.silent)
			run := beginUnavailableThread(t, v.Env, key)
			ctx := principal.SystemActing(v.Admin(), assuranceActor)
			err = database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
				_, err := assuranceSubjects(ctx, tx, v.now, nil, []ids.UUID{v.silent})
				return err
			})
			if !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("assessment with hidden %s: %v", kind, err)
			}
			if err := v.worker.check(ctx, v.WS, nil); err != nil {
				t.Fatal(err)
			}
			if status := v.WsScalar(t, `SELECT status FROM assurance_run ORDER BY as_of DESC,id DESC LIMIT 1`); status != "incomplete" {
				t.Fatalf("hidden evidence run status=%s", status)
			}
			if after := v.WsScalar(t, `SELECT coalesce(string_agg(id::text||':'||status,',' ORDER BY id),'') FROM assurance_exception WHERE subject_id=$1`, v.silent); after != snapshot {
				t.Fatalf("partial absence changed findings: %s -> %s", snapshot, after)
			}
			if err := database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
				subjects, err := assuranceSubjects(ctx, tx, v.now, nil, []ids.UUID{v.supported})
				if err == nil && len(subjects) != 1 {
					t.Fatalf("unaffected deal subjects=%d", len(subjects))
				}
				return err
			}); err != nil {
				t.Fatalf("unrelated scoped deal: %v", err)
			}
			finishUnavailableThread(t, v.Env, run)
			v.check(t)
		})
	}
}
