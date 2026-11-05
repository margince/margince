// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An activity's custom-field values are content, so the subject-rights engines
// treat them like its subject and body: the Art. 15 export hands them back and
// the Art. 17 cascade empties them.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// visitNote seeds a contact and one meeting with them alone, logged through the
// store's own writer with a value in an activity custom field. It answers the
// contact, the activity and the field's column.
func visitNote(t *testing.T, e *integration.Env) (ids.ContactID, ids.UUID, string) {
	t.Helper()
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
		Object: "activity", Label: "Visit note", Type: customfields.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the activity field: %v", err)
	}
	column := *field.ColumnName
	contact := e.SeedContact(t, "Ines Albrecht", nil)
	subject := "Site visit"
	logged, _, err := activities.NewStore(e.DB()).WithFieldCatalog(catalog).LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &subject, Source: "manual",
		Links:        []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		CustomFields: map[string]any{column: "asked about the night shift"},
	})
	if err != nil {
		t.Fatalf("log the meeting: %v", err)
	}
	return ids.From[ids.ContactKind](contact), ids.UUID(logged.Id), column
}

func storedVisitNote(t *testing.T, e *integration.Env, activityID ids.UUID, column string) string {
	t.Helper()
	return e.WsScalar(t, `SELECT coalesce(`+pgx.Identifier{column}.Sanitize()+`, '') FROM activity WHERE id = $1`, activityID)
}

func TestSubjectAccessHandsBackAnActivitysCustomFieldValues(t *testing.T) {
	e := integration.Setup(t)
	contact, activityID, column := visitNote(t, e)

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}
	for _, row := range pkg.Activities {
		if row["id"] == activityID.String() {
			if row[column] != "asked about the night shift" {
				t.Fatalf("the export's meeting row carries %v under %s, want the value logged with it: %v", row[column], column, row)
			}
			return
		}
	}
	t.Fatalf("the export lists no row for the meeting logged with the subject: %v", pkg.Activities)
}

func TestErasureEmptiesAnActivitysCustomFieldValues(t *testing.T) {
	e := integration.Setup(t)
	contact, activityID, column := visitNote(t, e)
	if got := storedVisitNote(t, e, activityID, column); got != "asked about the night shift" {
		t.Fatalf("the logged value was not stored: %q", got)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contact.UUID, "subject request"); err != nil {
		t.Fatalf("EraseContact: %v", err)
	}
	if got := storedVisitNote(t, e, activityID, column); got != "" {
		t.Fatalf("the erased subject's meeting still holds %q in %s", got, column)
	}
}

// A controller's release ends a hold and erases what it kept, the custom-field
// values with the text. The held row is seeded directly, as no writer produces
// a restriction the compiled-in floor would not.
func TestReleasingAHeldActivityEmptiesItsCustomFieldValues(t *testing.T) {
	e := integration.Setup(t)
	_, _, column := visitNote(t, e)
	held := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		                      retention_class, retention_class_at,
		                      restricted_at, restricted_until, restricted_reason, archived_at, `+
		pgx.Identifier{column}.Sanitize()+`)
		VALUES ($1, 'note', 'Held visit', 'kept under a hold', now() - interval '30 days', 'manual', 'human:seed',
		        'commercial_correspondence', now(), now(), now() + interval '6 years', 'commercial_correspondence', now(),
		        'asked about the night shift')`, held)

	reason, err := privacy.ParseStatedReason("the obligation ended: contract closed out")
	if err != nil {
		t.Fatal(err)
	}
	eraser := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB()))
	if err := eraser.ReleaseRestriction(releaseControllerCtx(e), held, reason); err != nil {
		t.Fatalf("ReleaseRestriction: %v", err)
	}
	if got := storedVisitNote(t, e, held, column); got != "" {
		t.Fatalf("the released record still holds %q in %s", got, column)
	}
}
