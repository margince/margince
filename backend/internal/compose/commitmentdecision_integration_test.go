// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNewerUnchangedAdmissionPreventsAnOlderResultFromReplacingIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	source := logReplyMessage(ctx, t, e, "pricing", "Pricing", "I will send pricing.", 0)
	id := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	at := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	decision := crmcontracts.CommitmentAdmission{
		Locator: "pricing", Summary: "Send pricing", SourceQuote: "I will send pricing.",
		Outcome: string(activities.CommitmentIncomplete), RuleVersion: activities.CommitmentAdmissionVersion,
		EvidenceIds: []openapi_types.UUID{source.Id}, EvaluatedAt: at,
	}
	for _, evaluated := range []time.Time{at, at.Add(2 * time.Hour), at.Add(time.Hour)} {
		decision.EvaluatedAt = evaluated
		if evaluated.Equal(at.Add(time.Hour)) {
			decision.Outcome, decision.CurrentWork = string(CommitmentTaskWritten), true
		}
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return activities.RecordCommitmentAdmissionTx(ctx, tx, id, decision)
		}); err != nil {
			t.Fatal(err)
		}
	}
	read, err := e.Activities.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if read.CommitmentAdmissions == nil || len(*read.CommitmentAdmissions) != 1 {
		t.Fatalf("admissions = %#v", read.CommitmentAdmissions)
	}
	got := (*read.CommitmentAdmissions)[0]
	if got.Outcome != string(activities.CommitmentIncomplete) || got.CurrentWork || !got.EvaluatedAt.Equal(at.Add(2*time.Hour)) {
		t.Fatalf("an older result replaced the latest evaluation: %#v", got)
	}
}

func TestAdmissionHistoryObeysSourcePrivacyAndSubjectErasure(t *testing.T) {
	e := integration.Setup(t)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Pat Buyer"})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	source := sourceWithAdmission(owner, t, e, []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contactID.UUID}})
	control := sourceWithAdmission(e.Admin(), t, e, nil)
	list, _, err := e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range list {
		if ids.UUID(row.Id) == source.UUID {
			found = row.CommitmentAdmissions != nil && len(*row.CommitmentAdmissions) == 1
		}
	}
	if !found {
		t.Fatal("timeline omitted the source's commitment review")
	}
	exported, err := privacy.AssembleSAR(e.Admin(), e.DB(), contactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.CommitmentAdmissions) != 1 || exported.CommitmentAdmissions[0]["source_quote"] != "I will send pricing." {
		t.Fatalf("export omitted held admission: %#v", exported.CommitmentAdmissions)
	}
	if _, err := e.Activities.SetAudience(owner, source, activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	}); err != nil {
		t.Fatal(err)
	}
	hidden, err := e.Activities.GetActivity(e.Admin(), source, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	list, _, err = e.Activities.ListActivities(e.Admin(), activities.ListActivitiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if ids.UUID(row.Id) == source.UUID && row.CommitmentAdmissions != nil {
			t.Fatal("timeline disclosed private commitment review")
		}
	}
	if hidden.CommitmentAdmissions != nil {
		t.Fatalf("private source disclosed admissions: %#v", hidden.CommitmentAdmissions)
	}
	own, err := e.Activities.GetActivity(owner, source, storekit.LiveOnly)
	if err != nil || own.CommitmentAdmissions == nil {
		t.Fatalf("owner lost admission: %v", err)
	}
	exported, err = privacy.AssembleSAR(e.Admin(), e.DB(), contactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.CommitmentAdmissions) != 1 || exported.CommitmentAdmissions[0]["source_quote"] != nil || exported.CommitmentAdmissions[0]["summary"] != nil {
		t.Fatalf("export republished private source content: %#v", exported.CommitmentAdmissions)
	}
	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contactID.UUID, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id = $1`, source); got != 0 {
		t.Fatalf("erasure left %d admission records", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id = $1`, control); got != 1 {
		t.Fatalf("unrelated admission count = %d", got)
	}
}

func sourceWithAdmission(ctx context.Context, t *testing.T, e *integration.Env, links []activities.ActivityLinkInput) ids.ActivityID {
	t.Helper()
	at := time.Date(2024, 10, 18, 12, 0, 0, 0, time.UTC)
	quote := "I will send pricing."
	row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "meeting", Body: &quote, OccurredAt: &at, Source: "manual", Links: links})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return activities.RecordCommitmentAdmissionTx(ctx, tx, source, crmcontracts.CommitmentAdmission{
			Locator: "pricing", Summary: "Send pricing", SourceQuote: quote, Outcome: string(activities.CommitmentHistorical),
			RuleVersion: activities.CommitmentAdmissionVersion, EvidenceIds: []openapi_types.UUID{row.Id}, EvaluatedAt: at.AddDate(2, 0, 0),
		})
	}); err != nil {
		t.Fatal(err)
	}
	return source
}

func TestStoredCommitmentWithoutAReadSetReportsUnassessedContext(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	first, later := now.Add(-2*time.Hour), now.Add(-time.Hour)
	body := "I will send pricing."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Body: &body, OccurredAt: &first, Source: "manual", ThreadKey: "unassessed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Body: &body, OccurredAt: &later, Source: "manual", ThreadKey: "unassessed"}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		evidence, err := activities.CommitmentAdmissionEvidenceTx(e.Admin(), tx, activities.CommitmentAdmissionInput{SourceID: ids.From[ids.ActivityKind](ids.UUID(source.Id))})
		if err != nil {
			return err
		}
		if got := activities.AdmitAutomaticCommitment(evidence, now, time.UTC, nil); got != activities.CommitmentContextUnassessed {
			t.Fatalf("stored source verdict = %q, want unassessed", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
