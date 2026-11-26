// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestRenewalPreviewAndDeliveryShareDatesAndTheChosenRecipient(t *testing.T) {
	for _, recipient := range []string{"record_owner", "authorizer"} {
		for _, recurring := range []bool{false, true} {
			name := recipient + "/one date"
			if recurring {
				name = recipient + "/annual date"
			}
			t.Run(name, func(t *testing.T) { checkRenewalPreviewAndDelivery(t, recipient, recurring) })
		}
	}
}

func checkRenewalPreviewAndDelivery(t *testing.T, recipient string, recurring bool) {
	t.Helper()
	e := Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	authority := identity.NewService(e.Pool)
	rbac, err := authority.EffectiveRBAC(e.Admin(), e.WS, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.AdminUser, rbac.TeamIDs, rbac.Permissions)
	now := time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	fields, column, subject := seedReviewedRenewalDates(t, e, recurring)
	store := automation.NewAutomationStore(e.DB()).WithFieldCatalog(fields).SetClock(func() time.Time { return now })
	rule, err := store.Create(ctx, automation.CreateAutomationInput{
		Key: "renewal_reminder", Name: "Prepare for renewal",
		Params: map[string]any{"object": "contact", "date_field": column, "days_before": float64(30), "recurs_yearly": recurring, "recipient": recipient},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.Preview(ctx, rule.ID, automation.AutomationPreviewInput{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.MatchesNow != 1 || preview.Renewal == nil || preview.Renewal.MissingDates != 1 || len(preview.Renewal.Samples) != 1 {
		t.Fatalf("renewal population = %+v", preview)
	}
	sample := preview.Renewal.Samples[0]
	if sample.OccurrenceDate.Format(time.DateOnly) != "2027-01-05" || sample.ReminderDate.Format(time.DateOnly) != "2026-12-06" {
		t.Fatalf("calculated renewal dates = %+v", sample)
	}
	enabled := true
	if _, err := store.Update(ctx, rule.ID, automation.UpdateAutomationInput{Enabled: &enabled, InitialRunConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	scanner := compose.NewTimeScannerWithClock(e.DB(), func() time.Time { return now }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := scanner.ScanWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), e.WS); err != nil {
		t.Fatal(err)
	}
	want := e.Rep1
	if recipient == "authorizer" {
		want = e.AdminUser
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id=a.id WHERE a.kind='task' AND l.contact_id=$1 AND a.assignee_id=$2`, subject, want); got != 1 {
		t.Fatalf("tasks for the chosen recipient = %d", got)
	}
}

func seedReviewedRenewalDates(t *testing.T, e *Env, recurring bool) (*customfields.Service, string, ids.UUID) {
	t.Helper()
	fields := customfields.NewService(e.Pool, SchemaPool(t))
	ctx := e.As(e.Rep1, nil, CustomFieldAdminPerms)
	field, err := fields.Create(ctx, customfields.FieldSpec{Object: "contact", Label: "Reviewed renewal", Type: customfields.TypeDate, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	column := *field.ColumnName
	store := contacts.NewStore(e.DB()).WithFieldCatalog(fields)
	owner := ids.From[ids.UserKind](e.Rep1)
	var subject ids.UUID
	for index, date := range []string{"2027-01-05", "2027-03-01", ""} {
		values := map[string]any{}
		if date != "" {
			if recurring {
				date = "1990" + date[4:]
			}
			values[column] = date
		}
		contact, err := store.CreateContact(ctx, contacts.CreateContactInput{FullName: "Renewal customer " + date, Source: "manual", OwnerID: &owner, CustomFields: values})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			subject = ids.UUID(contact.Id)
		}
	}
	return fields, column, subject
}
