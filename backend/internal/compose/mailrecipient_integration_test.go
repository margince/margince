// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var emailPreviewDirectory = flag.String("email-previews", "", "write fictional production-assembled mail examples to this directory")

// previewWeek is the Monday, at UTC midnight, after the current week. Rows a
// store dates from the wall clock then always land before the week under review.
func previewWeek(t *testing.T) time.Time {
	t.Helper()
	today := clocktest.Now(t).UTC().Truncate(24 * time.Hour)
	return today.AddDate(0, 0, 7-(int(today.Weekday())+6)%7)
}

// reviewHour is the Monday morning after week, when its review is assembled.
func reviewHour(week time.Time) time.Time {
	return week.AddDate(0, 0, 7).Add(9*time.Hour + 30*time.Minute)
}

func exportRecipientMail(t *testing.T, name, subject, text, markup string) {
	t.Helper()
	if subject == "" || text == "" || !strings.Contains(markup, "<html lang=") {
		t.Fatalf("incomplete production message %s", name)
	}
	if *emailPreviewDirectory == "" {
		return
	}
	data, err := json.Marshal(struct{ Subject, Text, HTML string }{subject, text, markup})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(*emailPreviewDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*emailPreviewDirectory, name+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTeamMorningRanksAcrossEveryRosterMember(t *testing.T) {
	b := setupBriefJob(t).withMailer(&countingMailer{})
	labelRecipientTeam(t, b.Env)
	ctx := b.Admin()
	actor := identity.Identity{UserID: ids.From[ids.UserKind](b.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](b.WS), Roles: []string{"admin"}, Permissions: integration.AdminPerms}
	for _, member := range []ids.UUID{b.Rep3, b.AdminUser} {
		if err := b.worker.users.SetTeamMember(ctx, actor, b.Team1, member, true); err != nil {
			t.Fatal(err)
		}
	}
	scope := identity.RoutineTeam
	settings := identity.Delivery{Scope: &scope, TeamID: &b.Team1}
	if _, err := b.worker.users.SaveMyDelivery(ctx, settings); err != nil {
		t.Fatal(err)
	}
	roster, _, err := b.worker.users.DeliveryTeam(ctx, settings)
	if err != nil || len(roster) != 4 {
		t.Fatalf("roster: %d %v", len(roster), err)
	}
	now := reviewHour(previewWeek(t))
	for i, member := range roster {
		title, due := fmt.Sprintf("Routine customer task %d", i), now.Add(14*24*time.Hour)
		if i == len(roster)-1 {
			title, due = "Urgent buyer deadline", now.Add(-2*time.Hour)
		}
		assignee := ids.From[ids.UserKind](member.UserID)
		if _, _, err := b.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "task", Subject: &title, DueAt: &due, AssigneeID: &assignee, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	message, useful, err := b.worker.morningMessage(ctx, now, mailcopy.For("de"), false)
	if err != nil || !useful {
		t.Fatalf("team message: %v %v", useful, err)
	}
	if urgent := strings.Index(message.Text, "Urgent buyer deadline"); urgent < 0 {
		t.Fatal("the last roster member's urgent customer work was omitted")
	} else if routine := strings.Index(message.Text, "Routine customer task"); routine >= 0 && routine < urgent {
		t.Fatal("roster order outranked the real urgency")
	}
	if !strings.Contains(message.Text, mailcopy.TeamFragment(b.Team1.String(), "")) {
		t.Fatal("team mail opens the personal overview")
	}
	exportRecipientMail(t, "team-priority-de", message.Title, message.Text, emailview.Render(message))
}

// seedRecipientOutcomes records a won deal and two due tasks on the Thursday of week.
func seedRecipientOutcomes(t *testing.T, e *mailEnv, week time.Time) {
	t.Helper()
	thursday := week.AddDate(0, 0, 3).Add(12 * time.Hour)
	e.Deals = e.Deals.WithClock(func() time.Time { return thursday })
	e.Activities = e.Activities.WithClock(func() time.Time { return thursday })
	pipeline, open, won := integration.DealFixture(t, e.Env)
	deal := e.SeedDeal(t, "Northstar renewal", pipeline, open, &e.Rep1)
	reason, detail := "verbal", "Customer accepted the renewal"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason, WonWithoutContractDetail: &detail}); err != nil {
		t.Fatal(err)
	}
	due := thursday.Add(3*time.Hour + 30*time.Minute)
	for _, member := range []ids.UUID{e.Rep1, e.Rep2} {
		title := "Send the revised Northstar quote"
		assignee := ids.From[ids.UserKind](member)
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "task", Subject: &title, DueAt: &due, AssigneeID: &assignee, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProductionRoutineRecipientPreviews(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		for _, edition := range []string{"personal", "team", "combined", "weekly-morning"} {
			t.Run(language+"/"+edition, func(t *testing.T) {
				e := setupWeeklyMail(t)
				labelRecipientTeam(t, e.Env)
				e.worker.engine = newWeeklyEngine(e.Pool)
				week := previewWeek(t)
				now := reviewHour(week)
				e.worker.now = func() time.Time { return now }
				seedRecipientOutcomes(t, e, week)
				if _, err := e.worker.users.SaveMyLocale(e.repCtx, language); err != nil {
					t.Fatal(err)
				}
				scope, combine := edition, false
				if edition == "weekly-morning" {
					scope, combine = identity.RoutineCombined, true
				}
				settings, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Scope: &scope, TeamID: &e.Team1, CombineRoutine: &combine})
				if err != nil {
					t.Fatal(err)
				}
				review, _, err := e.worker.engine.AssembleFor(e.repCtx, now)
				if err != nil {
					t.Fatal(err)
				}
				if !weekly.MailUseful(review) {
					t.Fatal("real recorded customer work did not qualify for a report")
				}
				if scope != identity.RoutinePersonal {
					roster, _, err := e.worker.users.DeliveryTeam(e.repCtx, settings)
					if err != nil {
						t.Fatal(err)
					}
					name, err := e.worker.users.DeliveryTeamName(e.repCtx, settings)
					if err != nil {
						t.Fatal(err)
					}
					members := make([]weekly.TeamMember, 0, len(roster))
					for _, member := range roster {
						members = append(members, weekly.TeamMember{UserID: member.UserID, DisplayName: member.DisplayName})
					}
					if _, _, err := e.worker.engine.AssembleTeamFor(e.repCtx, e.Team1, name, members, now); err != nil {
						t.Fatal(err)
					}
				}
				e.worker.mailWeekly(e.repCtx, review.ID, now)
				if e.relay.count() != 1 {
					t.Fatalf("production weekly submitted %d messages", e.relay.count())
				}
				if !strings.Contains(e.relay.markup[0], `lang="`+language+`"`) {
					t.Fatal("saved recipient language was not honored")
				}
				if strings.Count(e.relay.markup[0], "<h1 ") != 1 {
					t.Fatal("combined edition has multiple primary headings")
				}
				if strings.Count(e.relay.markup[0], mailcopy.For(language).ManagePreferencesLabel) != 1 {
					t.Fatal("preferences footer was repeated or misplaced")
				}
				exportRecipientMail(t, "weekly-"+edition+"-"+language, e.relay.subjects[0], e.relay.bodies[0], e.relay.markup[0])
			})
		}
	}
}

func TestProductionAccountRoomAndPrivacyPreviews(t *testing.T) {
	expiry := previewWeek(t).AddDate(0, 0, 25).Add(12*time.Hour + 37*time.Minute)
	for _, language := range []string{"en", "de", "vi"} {
		words := mailcopy.For(language)
		messages := map[string]emailview.Message{
			"reset":         words.AccountMessage(false, "", "", "https://example.test/#/set-password/fictional"),
			"member-invite": words.AccountMessage(true, "Alex Example", "Northstar GmbH", "https://example.test/#/set-password/fictional"),
			"buyer-invite":  words.RoomMessage("Zweijahresangebot", "Alex Example", "Northstar GmbH", "https://example.test/#/room?c=fictional", expiry),
			"meeting":       words.MeetingMessage("Angebot besprechen", "Zweijahresvertrag", "Videoanruf", "https://example.test/manage", expiry, expiry.Add(45*time.Minute)),
		}
		for key, message := range messages {
			exportRecipientMail(t, key+"-"+language, message.Title, message.Text, emailview.Render(message))
		}
		for _, key := range consent.ControllerTemplateKeys() {
			rendered, _, err := consent.RenderControllerTemplate(key, expiry, language)
			if err != nil {
				t.Fatal(err)
			}
			rendered = consent.CompleteControllerMessage(rendered, "Northstar GmbH", []string{"Northstar GmbH\nBeispielstraße 12\n10115 Berlin", "privacy@example.test"})
			markup := consent.RenderControllerPresentation(rendered, "Northstar GmbH")
			if language == "de" && (strings.Contains(markup, "deine Daten") || strings.Contains(markup, "Erfahre,")) {
				t.Fatal("privacy preview and body use different address registers")
			}
			exportRecipientMail(t, key+"-"+language, rendered.Subject, rendered.Body, markup)
		}
	}
}

func TestAnEmptyWeekDoesNotSubmitOrSpendAnEmail(t *testing.T) {
	e := setupWeeklyMail(t)
	now := reviewHour(previewWeek(t))
	e.worker.now = func() time.Time { return now }
	review, _, err := e.worker.engine.AssembleFor(e.repCtx, now)
	if err != nil {
		t.Fatal(err)
	}
	e.worker.mailWeekly(e.repCtx, review.ID, now)
	if e.relay.count() != 0 {
		t.Fatal("an empty week produced an email")
	}
	pending, err := e.worker.engine.MailPending(e.repCtx, review.ID)
	if err != nil || !pending {
		t.Fatalf("empty suppression consumed the send claim: %v %v", pending, err)
	}
}

func TestProductionNoticeRecipientPreviews(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		t.Run(language, func(t *testing.T) {
			e := setupNoticeMail(t)
			if _, err := identity.NewService(e.e.Pool).SaveMyLocale(e.e.Admin(), language); err != nil {
				t.Fatal(err)
			}
			notice := e.pendingNotice(t)
			e.work(t, notice)
			if e.relay.count() != 1 {
				t.Fatal("approval was not delivered")
			}
			exportRecipientMail(t, "approval-"+language, e.relay.subjects[0], e.relay.bodies[0], e.relay.markup[0])
			d := setupDigest(t)
			if _, err := d.worker.users.SaveMyLocale(d.seatCtx(d.Rep1), language); err != nil {
				t.Fatal(err)
			}
			contact := d.capturedContact(t, "Northstar procurement", d.Rep1)
			d.raise(t, "Northstar requested the revised quote", contactRef(contact))
			d.run(t)
			if d.relay.count() != 1 {
				t.Fatal("digest was not delivered")
			}
			exportRecipientMail(t, "digest-"+language, d.relay.subjects[0], d.relay.bodies[0], d.relay.markup[0])
		})
	}
}

func TestProductionHelpAndPartialWeekPreviews(t *testing.T) {
	e := setupWeeklyMail(t)
	// A week already closed: the meeting row's created_at is the database's now(),
	// after the week, so its occurred_at alone sets the partial boundary.
	start := previewWeek(t).AddDate(0, 0, -21)
	plan := weeklyPlanStore(e.Pool)
	commitment, err := plan.AddCommitment(e.repCtx, start, weeklyplan.NewCommitment{Label: "Northstar renewal", DueOn: &start})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.AskForHelp(e.repCtx, commitment.ID, "Please join the pricing call before Friday."); err != nil {
		t.Fatal(err)
	}
	e.worker.engine = newWeeklyEngine(e.Pool)
	since := start.Add(48 * time.Hour)
	held, subject := "held", "Northstar customer review"
	e.Activities = e.Activities.WithClock(func() time.Time { return since.Add(time.Hour) })
	contact := e.SeedContact(t, "Northstar buyer", &e.Rep1)
	if _, _, err := e.Activities.LogActivity(e.repCtx, activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &since, MeetingStatus: &held, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}}); err != nil {
		t.Fatal(err)
	}
	partial, _, err := e.worker.engine.AssembleFor(e.repCtx, reviewHour(start))
	if err != nil {
		t.Fatal(err)
	}
	if partial.NumericSummary == nil || partial.NumericSummary.FigureCoverage == nil || partial.NumericSummary.FigureCoverage.Meetings == nil || partial.NumericSummary.FigureCoverage.Meetings.Status != "partial" {
		t.Fatal("production review did not retain partial meeting coverage")
	}
	// The mail prints the boundary in the review's frozen zone, not the zone pgx scanned it in.
	zone, err := time.LoadLocation(partial.NumericSummary.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "de", "vi"} {
		words := mailcopy.For(language)
		help, err := e.worker.weeklyHelpMail(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms), weekly.TeamRep{UserID: e.Rep1, DisplayName: "Alex Example", HelpRequested: 1}, start, words)
		if err != nil || !strings.Contains(help.String(), "Please join the pricing call") {
			t.Fatalf("actual request missing: %s %v", help.String(), err)
		}
		message := help.Message(words.MailHelp, language, words.MailHelp)
		exportRecipientMail(t, "team-help-"+language, message.Title, message.Text, emailview.Render(message))
		message = weekly.MailMessage(partial, "https://example.test", words)
		boundary := partial.NumericSummary.FigureCoverage.Meetings.RecordedSince
		if boundary == nil || !strings.Contains(message.Text, words.Date(boundary.In(zone))) {
			t.Fatalf("partial coverage lost its boundary: counts=%+v body=%s", partial.Counts, message.Text)
		}
		exportRecipientMail(t, "weekly-partial-"+language, message.Title, message.Text, emailview.Render(message))
	}
}

func TestProductionMorningRecipientPreviews(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		for _, scope := range []string{identity.RoutinePersonal, identity.RoutineTeam, identity.RoutineCombined} {
			t.Run(language+"/"+scope, func(t *testing.T) {
				relay := &countingMailer{}
				b := setupBriefJob(t).withMailer(relay)
				labelRecipientTeam(t, b.Env)
				now := reviewHour(previewWeek(t))
				seedWorkFor(t, b.Env, now, b.Rep1, b.Rep2)
				b.now = now
				ctx := b.As(b.Rep1, []ids.UUID{b.Team1}, integration.AdminPerms)
				if _, err := b.worker.users.SaveMyLocale(ctx, language); err != nil {
					t.Fatal(err)
				}
				if _, err := b.worker.users.SaveMyDelivery(ctx, identity.Delivery{Scope: &scope, TeamID: &b.Team1}); err != nil {
					t.Fatal(err)
				}
				run, err := b.worker.engine.SnapshotRun(ctx, now)
				if err != nil {
					t.Fatal(err)
				}
				b.worker.mailMorning(ctx, run, now)
				if relay.count() != 1 {
					t.Fatalf("production morning sent %d messages", relay.count())
				}
				if !strings.Contains(relay.markup[0], `lang="`+language+`"`) {
					t.Fatal("morning ignored saved language")
				}
				exportRecipientMail(t, "morning-"+scope+"-"+language, relay.subjects[0], relay.bodies[0], relay.markup[0])
			})
		}
	}
}

func TestATeamWeeklyWithoutAnOriginKeepsEveryContentBlock(t *testing.T) {
	e := setupWeeklyMail(t)
	week := previewWeek(t)
	now := reviewHour(week)
	seedRecipientOutcomes(t, e, week)
	scope := identity.RoutineTeam
	settings, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{Scope: &scope, TeamID: &e.Team1})
	if err != nil {
		t.Fatal(err)
	}
	review, _, err := e.worker.engine.AssembleFor(e.repCtx, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.worker.engine.AssembleTeamFor(e.repCtx, e.Team1, "Northstar sales", []weekly.TeamMember{{UserID: e.Rep1, DisplayName: "Alex Example"}}, now); err != nil {
		t.Fatal(err)
	}
	full, err := e.worker.weeklyMessage(e.repCtx, review, settings, now, mailcopy.For("de"))
	if err != nil {
		t.Fatal(err)
	}
	e.worker.mail.PublicBaseURL = ""
	without, err := e.worker.weeklyMessage(e.repCtx, review, settings, now, mailcopy.For("de"))
	if err != nil {
		t.Fatal(err)
	}
	for _, block := range full.Blocks {
		if block.ActionURL != "" {
			continue
		}
		if !strings.Contains(without.Text, (emailview.Content{Blocks: []emailview.Block{block}}).String()) {
			t.Fatalf("content disappeared without an origin: %+v", block)
		}
	}
	if strings.Contains(without.Text, "https:") {
		t.Fatal("an origin-less team message invented a link")
	}
}

func TestProductionControllerStagingPreviews(t *testing.T) {
	for _, language := range []string{"en", "de", "vi"} {
		lane := setupConfirmLane(t)
		lane.store.WithMailLanguage(consent.MailLanguageFunc(identity.LanguageOf))
		integration.SetBaseLanguage(lane.e.Admin(), t, lane.e.Pool, language)
		if _, err := lane.store.IssueConfirmToken(lane.ctx, ids.From[ids.ContactKind](lane.contact), ""); err != nil {
			t.Fatal(err)
		}
		purpose, err := lane.store.CreatePurpose(lane.e.Admin(), "marketing_email", "Product news", true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lane.store.IssueConsentLink(lane.ctx, ids.From[ids.ContactKind](lane.contact), purpose.ID, ""); err != nil {
			t.Fatal(err)
		}
		rows, err := lane.e.Pool.Query(lane.ctx, "SELECT subject,body,html_body FROM comms_outbound WHERE sender_kind='controller' ORDER BY subject")
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for rows.Next() {
			var subject, text, markup string
			if err := rows.Scan(&subject, &text, &markup); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(markup, `lang="`+language+`"`) {
				t.Fatal("staging ignored installation language")
			}
			key := "record_confirmation"
			if strings.Contains(text, mailcopy.For(language).MarketingPurpose) {
				key = "consent_confirmation"
			}
			if !strings.Contains(text, "{{confirmation-link}}") || strings.Contains(text, lane.vault.sealed) {
				t.Fatal("staged example lost sealing")
			}
			exportRecipientMail(t, key+"-staged-"+language, subject, text, markup)
			count++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if count != 2 {
			t.Fatalf("staged %d messages", count)
		}
	}
}

func TestDigestDecisionsUseCurrentStateAndTheirOwnDestination(t *testing.T) {
	e := setupNoticeMail(t)
	notice := e.pendingNotice(t)
	ctx := e.e.Admin()
	store := notices.NewStore(InstallationDB(e.e.Pool))
	if _, err := store.SaveNotificationPreference(ctx, notices.ClassApprovalPending, notices.DeliveryDigest); err != nil {
		t.Fatal(err)
	}
	var created time.Time
	if err := e.e.Pool.QueryRow(ctx, "SELECT created_at FROM notice WHERE id=$1", notice).Scan(&created); err != nil {
		t.Fatal(err)
	}
	now := created.Add(time.Minute)
	worker := newNotificationDigestWorker(e.e.Pool, NotificationMailConfig{}, e.worker.log)
	worker.now = func() time.Time { return now }
	window := notices.Window{Day: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), Morning: now}
	seat := ids.From[ids.UserKind](e.e.AdminUser)
	lines, err := worker.readDigestLines(ctx, seat, window)
	if err != nil || lines.total != 1 {
		t.Fatalf("pending digest: %+v %v", lines, err)
	}
	approval := lines.quotable[0].target.ID
	if lines.quotable[0].target.Type != notices.TargetApproval || !strings.Contains(digestMail(lines, "https://example.test", mailcopy.For("de")).Text, "/#/home?approval="+approval.String()) {
		t.Fatal("digest decision did not open the decision")
	}
	if _, err := approvals.NewService(InstallationDB(e.e.Pool)).Decide(ctx, ids.From[ids.ApprovalKind](approval), false, nil); err != nil {
		t.Fatal(err)
	}
	lines, err = worker.readDigestLines(ctx, seat, window)
	if err != nil || lines.total != 0 {
		t.Fatalf("settled decision survived in digest: %+v %v", lines, err)
	}
}

func labelRecipientTeam(t *testing.T, e *integration.Env) {
	t.Helper()
	name := "Northstar sales"
	actor := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS), Roles: []string{"admin"}, Permissions: integration.AdminPerms}
	if _, err := identity.NewService(e.Pool).UpdateTeam(e.Admin(), actor, e.Team1, identity.UpdateTeamInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
}
