// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The writes that take, change or destroy an activity's custom-field values
// beyond a plain log: accepting an email request, a patch, and the noise sweep.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// visitNoteStore is a store wired with a workspace catalog that holds one
// text field on activities. It answers the store and the field's column.
func visitNoteStore(t *testing.T, e *Env) (*activities.Store, string) {
	t.Helper()
	catalog := customfieldsmod.NewService(e.Pool, SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, CustomFieldAdminPerms), customfieldsmod.FieldSpec{
		Object: "activity", Label: "Visit note", Type: customfieldsmod.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the activity field: %v", err)
	}
	return activities.NewStore(e.DB()).WithFieldCatalog(catalog), *field.ColumnName
}

func storedValue(t *testing.T, e *Env, id ids.UUID, column string) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(`+pgx.Identifier{column}.Sanitize()+`, '') FROM activity WHERE id = $1`, id)
}

func auditedValue(t *testing.T, e *Env, id ids.UUID, action, image, column string) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(`+image+`->>$3, '') FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1 AND action = $2
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id, action, column)
}

func TestAcceptingAnEmailRequestKeepsItsCustomFieldValues(t *testing.T) {
	e := Setup(t)
	store, column := visitNoteStore(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	subject, direction := "Can you send slots?", "inbound"
	asked := clocktest.Now(t).Add(-2 * time.Hour)
	source, _, err := store.LogActivity(rep, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, ThreadKey: "slots-request",
		Source: "test", OccurredAt: &asked,
	})
	if err != nil {
		t.Fatalf("log the request: %v", err)
	}
	requestID := ids.UUID(source.Id)
	title := "Send three slots"
	task, _, err := store.LogActivity(rep, activities.LogActivityInput{
		Kind: "task", RequestActivityID: &requestID, Subject: &title, Source: "manual",
		CustomFields: map[string]any{column: "two mornings, one afternoon"},
	})
	if err != nil {
		t.Fatalf("accept the request: %v", err)
	}
	taskID := ids.UUID(task.Id)
	if task.AdditionalProperties[column] != "two mornings, one afternoon" {
		t.Errorf("the accepted task answers %v under %s, want the value it was given", task.AdditionalProperties[column], column)
	}
	if got := storedValue(t, e, taskID, column); got != "two mornings, one afternoon" {
		t.Errorf("the accepted task stored %q in %s", got, column)
	}
	if got := auditedValue(t, e, taskID, "create", "after", column); got != "two mornings, one afternoon" {
		t.Errorf("the task's create audit row carries %q for %s", got, column)
	}
}

func TestAPatchSetsAndClearsAnActivitysCustomFieldValue(t *testing.T) {
	e := Setup(t)
	store, column := visitNoteStore(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	subject := "Plant walk-through"
	logged, _, err := store.LogActivity(rep, activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &subject, Source: "manual",
		CustomFields: map[string]any{column: "first visit"},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))

	patched, err := store.UpdateActivity(rep, id, activities.UpdateActivityInput{
		CustomFields: map[string]any{column: "second visit"},
	})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.AdditionalProperties[column] != "second visit" || storedValue(t, e, id.UUID, column) != "second visit" {
		t.Fatalf("the patch answered %v and stored %q, want the new value", patched.AdditionalProperties[column], storedValue(t, e, id.UUID, column))
	}
	if before, after := auditedValue(t, e, id.UUID, "update", "before", column), auditedValue(t, e, id.UUID, "update", "after", column); before != "first visit" || after != "second visit" {
		t.Errorf("the update audit row shows %q → %q for %s, want first visit → second visit", before, after, column)
	}

	if _, err := store.UpdateActivity(rep, id, activities.UpdateActivityInput{
		CustomFields: map[string]any{column: nil},
	}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := storedValue(t, e, id.UUID, column); got != "" {
		t.Errorf("a null patch left %q in %s", got, column)
	}
	if _, err := store.UpdateActivity(rep, id, activities.UpdateActivityInput{
		CustomFields: map[string]any{"cf_nobody_defined": "x"},
	}); err == nil {
		t.Error("a patch naming no active field was accepted")
	}
}

func TestNoiseRedactionEmptiesAnActivitysCustomFieldValues(t *testing.T) {
	e := Setup(t)
	store, column := visitNoteStore(t, e)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	subject, direction := "Newsletter", "inbound"
	noise := make([]ids.UUID, 0, 2)
	for range 2 {
		logged, _, err := store.LogActivity(rep, activities.LogActivityInput{
			Kind: "email", Subject: &subject, Direction: &direction, Source: "test",
			CustomFields: map[string]any{column: "filed by hand"},
		})
		if err != nil {
			t.Fatalf("log: %v", err)
		}
		id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
		if _, err := store.ArchiveActivity(rep, id, nil); err != nil {
			t.Fatalf("archive: %v", err)
		}
		noise = append(noise, id.UUID)
	}
	// The second already lost its text: what is left of its content is the value.
	e.WsExec(t, `UPDATE activity SET subject = NULL, body = NULL WHERE id = $1`, noise[1])

	erasable, err := store.ErasableActivityColumns(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := store.RedactCapturedNoiseTx(ctx, tx, erasable, noise)
		return err
	}); err != nil {
		t.Fatalf("redact: %v", err)
	}
	for _, id := range noise {
		if got := storedValue(t, e, id, column); got != "" {
			t.Errorf("the redacted message %s still holds %q in %s", id, got, column)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'erase'`, noise[1]); n != 1 {
		t.Errorf("the message holding only a custom value was audited as erased %d times, want once", n)
	}
}
