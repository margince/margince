// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactMergePreviewMatchesProfileEvidenceSurvivorship(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	var contactIDs []ids.UUID
	for _, name := range []string{"Original contact", "Surviving contact"} {
		contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactIDs = append(contactIDs, ids.UUID(contact.Id))
	}
	for _, claim := range []struct {
		contact                  int
		field, value, quote, url string
	}{
		{0, "title", "Director", "Director of engineering", "https://source.example.test/team"},
		{0, "phone", "+12025550101", "First phone", "https://source.example.test/contact"},
		{0, "phone", "+12025550102", "Second phone", "https://source.example.test/contact"},
		{1, "title", "Founder", "Founder of the company", "https://survivor.example.test/team"},
		{1, "phone", "+12025550101", "Current phone evidence", "https://survivor.example.test/contact"},
	} {
		count, err := e.Contacts.SaveResearchClaims(ctx, ids.From[ids.ContactKind](contactIDs[claim.contact]), []contacts.ResearchClaimInput{{Field: claim.field, Value: claim.value, Quote: claim.quote, SourceURL: claim.url}})
		if err != nil || count != 1 {
			t.Fatalf("save claim: %d, %v", count, err)
		}
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", contactIDs[0], contactIDs[1], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.ProfileFields) != 3 {
		t.Fatalf("profile effects: %+v", preview.ProfileFields)
	}
	kept := map[string]contacts.MergeProfileValue{}
	for _, effect := range preview.ProfileFields {
		if effect.Source.ID.IsZero() || effect.Source.ObservedAt.IsZero() || effect.Source.EvidenceSnippet == "" || effect.Source.SourceRef == "" {
			t.Fatalf("missing provenance: %+v", effect)
		}
		value := effect.Source
		if value.Value == "+12025550102" {
			if effect.Target != nil {
				t.Fatalf("distinct phone collapses: %+v", effect)
			}
		} else {
			if effect.Target == nil {
				t.Fatalf("collision missing: %+v", effect)
			}
			value = *effect.Target
		}
		kept[value.Field+":"+value.ValueKey] = value
	}
	checkMergePreviewRefusals(t, e, "contact", contactIDs[0], contactIDs[1], preview, "phone")
	if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](contactIDs[0]), ids.From[ids.ContactKind](contactIDs[1]), &preview.SourceVersion); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT field, value_key, value, evidence_snippet, source_ref, observed_at FROM contact_profile_field WHERE contact_id = @contact`, pgx.StrictNamedArgs{"contact": contactIDs[1]})
		if err != nil {
			return err
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var field, key, value, quote, url string
			var observed time.Time
			if err := rows.Scan(&field, &key, &value, &quote, &url, &observed); err != nil {
				return err
			}
			expected, found := kept[field+":"+key]
			if !found || value != expected.Value || quote != expected.EvidenceSnippet || url != expected.SourceRef || !observed.Equal(expected.ObservedAt) {
				t.Errorf("unexpected copied evidence for %s:%s", field, key)
			}
			count++
		}
		if count != len(kept) {
			t.Errorf("profile rows: %d, want %d", count, len(kept))
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	var retired int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT count(*) FROM contact_profile_field WHERE contact_id = @contact`, pgx.StrictNamedArgs{"contact": contactIDs[0]}).Scan(&retired); err != nil {
		t.Fatal(err)
	}
	if retired != 0 {
		t.Fatalf("retired profile rows: %d", retired)
	}
}

func TestContactMergePreviewProtectsAndPreservesSignatureEvidence(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	var contactIDs []ids.UUID
	for _, name := range []string{"Signature contact", "Surviving contact"} {
		contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		contactIDs = append(contactIDs, ids.UUID(contact.Id))
	}
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	second := first.Add(24 * time.Hour)
	var latest ids.UUID
	for _, statement := range []struct {
		title      string
		at         time.Time
		confidence float64
	}{
		{"VP Finance", first, 0.95}, {"CFO", second, 0.9},
	} {
		body := "Regards, Dana, " + statement.title
		message, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
			Kind: "email", Subject: new("Contact details"), Body: &body, Direction: new("inbound"), OccurredAt: &statement.at, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contactIDs[0]}},
		})
		if err != nil {
			t.Fatal(err)
		}
		latest = ids.UUID(message.Id)
		result, err := e.Contacts.ApplySignatureFields(ctx, ids.From[ids.ContactKind](contactIDs[0]), latest,
			[]contacts.SignatureField{{Name: "title", Value: statement.title, Evidence: body, Confidence: statement.confidence}}, true)
		if err != nil || result.Applied != 1 {
			t.Fatalf("signature: %+v, %v", result, err)
		}
	}
	preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", contactIDs[0], contactIDs[1], nil, nil)
	if err != nil || len(preview.ProfileFields) != 1 {
		t.Fatalf("signature preview: %+v, %v", preview.ProfileFields, err)
	}
	field := preview.ProfileFields[0].Source
	if field.Value != "CFO" || field.Confidence == nil || *field.Confidence != 0.9 || !field.ObservedAt.Equal(second) || field.SupersededValue == nil || *field.SupersededValue != "VP Finance" || field.SupersededObservedAt == nil || !field.SupersededObservedAt.Equal(first) {
		t.Fatalf("signature history: %+v", field)
	}
	if _, err := e.Activities.SetAudience(ctx, ids.From[ids.ActivityKind](latest), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.AdminUser}}}); err != nil {
		t.Fatal(err)
	}
	hidden, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.Rep1, nil, integration.AdminPerms), "contact", contactIDs[0], contactIDs[1], nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) || len(hidden.ProfileFields) != 0 {
		t.Fatalf("hidden signature: %+v, %v", hidden.ProfileFields, err)
	}
	if _, err := e.Contacts.MergeContact(ctx, ids.From[ids.ContactKind](contactIDs[0]), ids.From[ids.ContactKind](contactIDs[1]), &preview.SourceVersion); err != nil {
		t.Fatal(err)
	}
	var value, superseded, ref string
	var confidence float64
	var observed, previous time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT value, confidence, observed_at, superseded_value, superseded_observed_at, source_ref FROM contact_profile_field WHERE contact_id = @contact AND field = 'title'`,
		pgx.StrictNamedArgs{"contact": contactIDs[1]}).Scan(&value, &confidence, &observed, &superseded, &previous, &ref); err != nil {
		t.Fatal(err)
	}
	if value != "CFO" || confidence != 0.9 || !observed.Equal(second) || superseded != "VP Finance" || !previous.Equal(first) || ref != "activity:"+latest.String() {
		t.Fatalf("merged signature: %q, %v, %v, %q, %v, %q", value, confidence, observed, superseded, previous, ref)
	}
}

func TestContactMergePreviewDoesNotReadUnchangedTargetProfileEvidence(t *testing.T) {
	for _, withSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty source", true: "different source field"}[withSource], func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
			var contactIDs []ids.UUID
			for _, name := range []string{"Original contact", "Surviving contact"} {
				contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: name, Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				contactIDs = append(contactIDs, ids.UUID(contact.Id))
			}
			if _, err := e.Contacts.SaveResearchClaims(ctx, ids.From[ids.ContactKind](contactIDs[1]), []contacts.ResearchClaimInput{{Field: "title", Value: "Director", Quote: "Director of engineering", SourceURL: "https://survivor.example.test/team"}}); err != nil {
				t.Fatal(err)
			}
			expected := 0
			if withSource {
				if _, err := e.Contacts.SaveResearchClaims(ctx, ids.From[ids.ContactKind](contactIDs[0]), []contacts.ResearchClaimInput{{Field: "phone", Value: "+12025550103", Quote: "Contact number", SourceURL: "https://source.example.test/contact"}}); err != nil {
					t.Fatal(err)
				}
				expected = 1
			}
			perms := integration.AdminPerms
			perms.RoleKeys = []string{"rep"}
			perms.RowScope = principal.RowScopeOwn
			perms.FieldMasks = []principal.FieldMask{{Object: "contact", Field: "title", Condition: principal.MaskAlways}}
			preview, err := e.Contacts.PreviewMergeSurvivorship(e.As(e.AdminUser, nil, perms), "contact", contactIDs[0], contactIDs[1], nil, nil)
			if err != nil || len(preview.ProfileFields) != expected {
				t.Fatalf("unaffected target evidence: %+v, %v", preview.ProfileFields, err)
			}
			for _, effect := range preview.ProfileFields {
				if effect.Source.Field != "phone" || effect.Target != nil {
					t.Fatalf("unrelated target evidence returned: %+v", effect)
				}
			}
		})
	}
}

func TestContactMergePreviewHonorsProfileVerdicts(t *testing.T) {
	for _, verdict := range []string{ai.VerdictCorrected, ai.VerdictSuppressed, ai.VerdictConfirmed} {
		t.Run(verdict, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.Admin()
			source, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Original contact", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			target, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Surviving contact", Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sourceID, targetID := ids.UUID(source.Id), ids.UUID(target.Id)
			save := func(value string) {
				t.Helper()
				_, err := e.Contacts.SaveResearchClaims(ctx, ids.From[ids.ContactKind](sourceID), []contacts.ResearchClaimInput{{Field: "title", Value: value, Quote: "Published title", SourceURL: "https://example.test/team"}})
				if err != nil {
					t.Fatal(err)
				}
			}
			save("Director")
			shown, corrected, note := "Director", "Founder", "Human decision"
			input := ai.RecordInput{SubjectType: "contact", SubjectID: sourceID, ClaimKind: ai.ClaimProfileField, ClaimPath: ai.ProfileFieldClaimPath("title"), Verdict: verdict, ValueShown: &shown, Note: &note}
			if verdict == ai.VerdictCorrected {
				input.CorrectedValue = &corrected
			}
			if err := ai.NewFeedbackStore(e.DB()).Record(ctx, input); err != nil {
				t.Fatal(err)
			}
			preview, err := e.Contacts.PreviewMergeSurvivorship(ctx, "contact", sourceID, targetID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.ProfileFields) != 1 {
				t.Fatalf("profile effects: %+v", preview.ProfileFields)
			}
			value := preview.ProfileFields[0].Source
			if value.Value != shown || value.Verdict == nil || value.Verdict.Verdict != verdict || value.Verdict.Note == nil || *value.Verdict.Note != note {
				t.Fatalf("lost assertion or human decision: %+v", value)
			}
			if verdict == ai.VerdictCorrected && (value.Verdict.CorrectedValue == nil || *value.Verdict.CorrectedValue != corrected) {
				t.Fatalf("lost correction: %+v", value.Verdict)
			}
			save("CEO")
			preview, err = e.Contacts.PreviewMergeSurvivorship(ctx, "contact", sourceID, targetID, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.ProfileFields) != 1 || preview.ProfileFields[0].Source.Value != "CEO" || preview.ProfileFields[0].Source.Verdict != nil {
				t.Fatalf("stale decision applied: %+v", preview.ProfileFields)
			}
		})
	}
}
