// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestSystemTaskDisplayFollowsTheReaderAndRealEditsRetireOnlyTheirComponent(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	subject := systemtext.Literal("automation.stage_change")
	body := systemtext.Literal("automation.new_lead")
	activity, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: &subject, Body: &body, SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.stage_change"), Body: systemtext.Sentence("automation.new_lead")}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(activity.Id))
	for _, language := range textlang.Shipped {
		localized, err := store.GetActivity(systemtext.WithLocale(ctx, language), id, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if localized.SystemDisplay == nil || localized.SystemDisplay.Subject == nil || *localized.SystemDisplay.Subject != systemtext.Copy("automation.stage_change", language) || *localized.Subject != subject {
			t.Fatalf("%s display or canonical text changed: %+v", language, localized)
		}
	}
	done := true
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{IsDone: &done, Subject: &subject}); err != nil {
		t.Fatal(err)
	}
	var kept bool
	if err := e.owner.QueryRow(context.Background(), `SELECT system_text ? 'subject' AND system_text ? 'body' FROM activity WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&kept); err != nil || !kept {
		t.Fatalf("unrelated/no-op edit retired provenance: %v %v", kept, err)
	}
	edited := "My own instruction"
	updated, err := store.UpdateActivity(systemtext.WithLocale(ctx, textlang.German), id, UpdateActivityInput{Subject: &edited})
	if err != nil {
		t.Fatal(err)
	}
	if updated.SystemDisplay == nil || updated.SystemDisplay.Subject != nil || updated.SystemDisplay.Body == nil || *updated.Subject != edited {
		t.Fatalf("edit retained stale subject or lost body: %+v", updated)
	}
	before, after := auditImagesFor(t, e, id.UUID)
	if before["system_text"] == nil || after["system_text"] == nil {
		t.Fatalf("metadata lifecycle absent from audit: before=%v after=%v", before, after)
	}
}

func TestSchedulingDefaultOriginSurvivesOtherEditsButNeverReclassifiesLegacyTitles(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	first, err := store.SaveSchedulingProfile(ctx, defaultSchedulingProfile())
	if err != nil {
		t.Fatal(err)
	}
	german := systemtext.WithLocale(ctx, textlang.German)
	read, err := store.SchedulingProfile(german)
	if err != nil || read.Title != systemtext.Copy("booking.meeting", textlang.German) {
		t.Fatalf("default did not localize: %+v %v", read, err)
	}
	read.BufferMinutes++
	if _, err := store.SaveSchedulingProfile(german, read); err != nil {
		t.Fatal(err)
	}
	var title string
	var origin bool
	if err := e.owner.QueryRow(ctx, `SELECT scheduling_policy->>'title',coalesce((scheduling_policy->>'_default_title')::boolean,false) FROM booking_page WHERE slug=@slug`, pgx.StrictNamedArgs{"slug": *first.Slug}).Scan(&title, &origin); err != nil || title != systemtext.Literal("booking.meeting") || !origin {
		t.Fatalf("presentation was saved over the default: %s %v %v", title, origin, err)
	}
	read.Title = "My meeting"
	read.UseDefaultTitle = new(false)
	if _, err := store.SaveSchedulingProfile(german, read); err != nil {
		t.Fatal(err)
	}
	read, err = store.SchedulingProfile(systemtext.WithLocale(ctx, textlang.Vietnamese))
	if err != nil || read.Title != "My meeting" {
		t.Fatalf("human title changed language: %+v %v", read, err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE booking_page SET scheduling_policy=jsonb_set(scheduling_policy-'_default_title','{title}',to_jsonb(@title::text)) WHERE slug=@slug`, pgx.StrictNamedArgs{"title": systemtext.Literal("booking.meeting"), "slug": *first.Slug}); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.SchedulingProfile(german)
	if err != nil || legacy.Title != systemtext.Literal("booking.meeting") {
		t.Fatalf("English equality reclassified legacy title: %+v %v", legacy, err)
	}
}

func TestABookingPageStoredUnderAnEarlierDefaultWordingStaysOnTheDefault(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	first, err := store.SaveSchedulingProfile(ctx, defaultSchedulingProfile())
	if err != nil {
		t.Fatal(err)
	}
	stored := func(title string, flagged bool) {
		t.Helper()
		if _, err := e.owner.Exec(ctx, `UPDATE booking_page SET scheduling_policy=(scheduling_policy-'_default_title') || jsonb_build_object('title',@title::text)
 || CASE WHEN @flagged::boolean THEN jsonb_build_object('_default_title',true) ELSE '{}'::jsonb END WHERE slug=@slug`, pgx.StrictNamedArgs{"title": title, "flagged": flagged, "slug": *first.Slug}); err != nil {
			t.Fatal(err)
		}
	}
	onDefault := func(stage string) {
		t.Helper()
		for _, language := range textlang.Shipped {
			read, err := store.SchedulingProfile(systemtext.WithLocale(ctx, language))
			if err != nil || read.Title != systemtext.Copy("booking.meeting", language) || read.UseDefaultTitle == nil || !*read.UseDefaultTitle {
				t.Fatalf("%s: a %s reader lost the default title: %+v %v", stage, language, read, err)
			}
		}
	}
	storedTitle := func() string {
		t.Helper()
		var title string
		if err := e.owner.QueryRow(ctx, `SELECT scheduling_policy->>'title' FROM booking_page WHERE slug=@slug AND (scheduling_policy->>'_default_title')::boolean`, pgx.StrictNamedArgs{"slug": *first.Slug}).Scan(&title); err != nil {
			t.Fatalf("the page left the default title: %v", err)
		}
		return title
	}
	auditedTitles := func() (string, string) {
		t.Helper()
		var before, after string
		if err := e.owner.QueryRow(ctx, `SELECT a.before->>'title', a.after->>'title' FROM audit_log a JOIN booking_page p ON p.id = a.entity_id
 WHERE a.entity_type = 'booking_page' AND a.action = 'update' AND p.slug=@slug ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1`, pgx.StrictNamedArgs{"slug": *first.Slug}).Scan(&before, &after); err != nil {
			t.Fatalf("reading the page's audit: %v", err)
		}
		return before, after
	}

	shippedDefaults := map[textlang.Lang]string{textlang.English: "Let's meet", textlang.German: "Treffen wir uns", textlang.Vietnamese: "Hãy gặp nhau"}
	for _, language := range textlang.Shipped {
		earlier := shippedDefaults[language]
		stored(earlier, true)
		onDefault(earlier)
		reader := systemtext.WithLocale(ctx, language)
		read, err := store.SchedulingProfile(reader)
		if err != nil {
			t.Fatal(err)
		}
		read.BufferMinutes++
		if _, err := store.SaveSchedulingProfile(reader, read); err != nil {
			t.Fatal(err)
		}
		if title := storedTitle(); title != earlier {
			t.Fatalf("an edit of another field rewrote the stored default %q as %q", earlier, title)
		}
		if before, after := auditedTitles(); before != earlier || after != earlier {
			t.Fatalf("the audit of that edit shows the title %q -> %q, not the stored %q", before, after, earlier)
		}
	}

	stale, err := store.SchedulingProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale.Title, stale.UseDefaultTitle = "Let's meet", nil
	if _, err := store.SaveSchedulingProfile(systemtext.WithLocale(ctx, textlang.English), stale); err != nil {
		t.Fatal(err)
	}
	onDefault("an English reader sent the earlier wording back")

	stored("Let's meet", true)
	custom, err := store.SchedulingProfile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	custom.Title, custom.UseDefaultTitle = "Intro call", new(false)
	if _, err := store.SaveSchedulingProfile(ctx, custom); err != nil {
		t.Fatal(err)
	}
	if before, after := auditedTitles(); before != "Let's meet" || after != "Intro call" {
		t.Fatalf("the audit of a title edit shows %q -> %q, not the stored wording to the new title", before, after)
	}

	stored("Let's meet", false)
	legacy, err := store.SchedulingProfile(ctx)
	if err != nil || legacy.Title != "Let's meet" || legacy.UseDefaultTitle == nil || *legacy.UseDefaultTitle {
		t.Fatalf("a title saved without the default flag was reclassified: %+v %v", legacy, err)
	}
}

func TestAHumanNoteMatchingAnErasureCaptionRetainsItsOwnWords(t *testing.T) {
	e := setupSend(t)
	ctx := systemtext.WithLocale(e.as(principal.RowScopeAll), textlang.German)
	subject := "Erased"
	activity, _, err := e.store(nil).LogActivity(ctx, LogActivityInput{Kind: "note", Subject: &subject})
	if err != nil {
		t.Fatal(err)
	}
	if activity.SystemDisplay != nil || activity.Subject == nil || *activity.Subject != subject {
		t.Fatalf("human caption was classified by English equality: %+v", activity)
	}
}

func TestANewCustomTitleCannotBecomeDefaultByMatchingEnglish(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	custom := defaultSchedulingProfile()
	custom.UseDefaultTitle = nil
	if _, err := store.SaveSchedulingProfile(ctx, custom); err != nil {
		t.Fatal(err)
	}
	german := systemtext.WithLocale(ctx, textlang.German)
	got, err := store.SchedulingProfile(german)
	if err != nil || got.Title != custom.Title || got.UseDefaultTitle == nil || *got.UseDefaultTitle {
		t.Fatalf("custom wording gained a built-in origin: %+v %v", got, err)
	}
}
