// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

// stagedControllerMail is the one controller delivery the lane staged.
func stagedControllerMail(t *testing.T, l *confirmLaneEnv) (activityID ids.UUID, subject, body string) {
	t.Helper()
	if err := l.e.Pool.QueryRow(context.Background(), `
		SELECT activity_id, subject, body FROM comms_outbound WHERE sender_kind = 'controller'`).
		Scan(&activityID, &subject, &body); err != nil {
		t.Fatalf("reading the staged controller delivery: %v", err)
	}
	return activityID, subject, body
}

func TestTheTimelineShowsAConfirmRequestWasSentByTheRelayAndFromWhichAddress(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	staged, _, _ := stagedControllerMail(t, l)

	contactType := "contact"
	page, _, err := activities.NewStore(InstallationDB(l.e.Pool)).ListActivities(l.ctx, activities.ListActivitiesInput{
		EntityType: &contactType, EntityID: &l.contact,
	})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	for _, row := range page {
		if ids.UUID(row.Id) != staged {
			continue
		}
		if row.ControllerSend == nil {
			t.Fatal("the timeline row of a relay send carries no controller_send, so it reads as the rep's own mail")
		}
		if row.ControllerSend.FromAddress == nil || *row.ControllerSend.FromAddress != laneFromAddress {
			t.Errorf("controller_send.from_address = %v, want %q, the address the relay was configured with",
				row.ControllerSend.FromAddress, laneFromAddress)
		}
		if row.EmailSummary == nil || row.EmailSummary.Delivery == nil {
			t.Error("the relay send lost its delivery state on the timeline")
		}
		return
	}
	t.Fatalf("the confirm request's activity %s is not on the contact's timeline", staged)
}

func TestAConfirmRequestInAPickedLanguageIsWrittenAndAskedInIt(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), "de"); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	_, subject, _ := stagedControllerMail(t, l)
	german, _, err := consent.RenderControllerTemplate(consent.TemplateRecordConfirmation, time.Time{}, "de")
	if err != nil {
		t.Fatal(err)
	}
	english, _, err := consent.RenderControllerTemplate(consent.TemplateRecordConfirmation, time.Time{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	if german.Subject == english.Subject {
		t.Fatal("the German and English subjects are the same, so this test cannot tell the languages apart")
	}
	if subject != german.Subject {
		t.Errorf("staged subject %q, want the German %q: the installation speaks English and the rep picked German",
			subject, german.Subject)
	}

	var locale string
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT question_locale FROM confirm_token`).Scan(&locale); err != nil {
		t.Fatal(err)
	}
	if locale != "de" {
		t.Errorf("question_locale %q, want de: the page would ask in a language the mail was not written in", locale)
	}
	_, token, _ := lastCut(l.vault.sealed, "#/confirm/")
	ref, err := l.store.ResolveConfirmToken(l.ctx, token, consent.FetchByAHuman)
	if err != nil {
		t.Fatalf("ResolveConfirmToken: %v", err)
	}
	if ref.QuestionLocale != "de" {
		t.Errorf("the page resolves language %q, want de", ref.QuestionLocale)
	}
}

func TestAPreviewShowsTheMailTheSendStagesAndSendsNothing(t *testing.T) {
	l := setupConfirmLane(t)
	contact := ids.From[ids.ContactKind](l.contact)

	preview, err := l.store.PreviewControllerMail(l.ctx, contact, consent.LinkRecordConfirmation, "vi")
	if err != nil {
		t.Fatalf("PreviewControllerMail: %v", err)
	}
	for _, table := range []string{"confirm_token", "comms_outbound", "activity"} {
		if n := l.e.WsCount(t, `SELECT count(*) FROM `+table); n != 0 {
			t.Fatalf("a preview wrote %d %s row(s); opening it must send nothing and mint nothing", n, table)
		}
	}
	if preview.From != laneFromAddress || preview.To != "subject@example.test" {
		t.Errorf("preview from %q to %q, want %q to the subject's own address", preview.From, preview.To, laneFromAddress)
	}
	if preview.Language != "vi" || preview.DefaultLanguage != string(mailcopy.Fallback) {
		t.Errorf("preview language %q default %q, want vi over the installation's %s",
			preview.Language, preview.DefaultLanguage, mailcopy.Fallback)
	}
	marker := mailcopy.For("vi").PreviewLinkMarker
	if !strings.Contains(preview.Text, marker) || strings.Contains(preview.Text, "{{confirmation-link}}") {
		t.Errorf("the preview text must carry the link marker and no placeholder:\n%s", preview.Text)
	}

	if _, err := l.store.IssueConfirmToken(l.ctx, contact, "vi"); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	_, subject, body := stagedControllerMail(t, l)
	if subject != preview.Subject {
		t.Errorf("preview subject %q, staged %q", preview.Subject, subject)
	}
	assertSameMailButTheExpiry(t, preview.Text, strings.ReplaceAll(body, "{{confirmation-link}}", marker), mailcopy.For("vi"))
}

// assertSameMailButTheExpiry compares two renderings line by line, except the
// link's expiry, which each rendered at its own instant.
func assertSameMailButTheExpiry(t *testing.T, previewed, staged string, words mailcopy.Copy) {
	t.Helper()
	want, got := strings.Split(staged, "\n"), strings.Split(previewed, "\n")
	if len(want) != len(got) {
		t.Fatalf("the preview has %d lines and the staged mail %d:\npreview:\n%s\nstaged:\n%s", len(got), len(want), previewed, staged)
	}
	for i := range want {
		if want[i] != got[i] && !strings.HasPrefix(want[i], words.ConfirmPersonal) {
			t.Errorf("line %d: preview %q, staged %q", i+1, got[i], want[i])
		}
	}
}

func TestAPreviewAndASendRefuseALanguageTheMailIsNotWrittenIn(t *testing.T) {
	l := setupConfirmLane(t)
	contact := ids.From[ids.ContactKind](l.contact)
	var refusal *consent.ValidationError

	if _, err := l.store.PreviewControllerMail(l.ctx, contact, consent.LinkPrivacyNotice, "fr"); !errors.As(err, &refusal) || refusal.Field != "language" {
		t.Errorf("preview in fr: %v, want a refusal of the language", err)
	}
	if _, err := l.store.IssuePrivacyNotice(l.ctx, contact, "fr"); !errors.As(err, &refusal) || refusal.Field != "language" {
		t.Errorf("privacy notice in fr: %v, want a refusal of the language", err)
	}
	if n := l.e.WsCount(t, `SELECT count(*) FROM confirm_token`); n != 0 {
		t.Errorf("a refused language still minted %d link(s)", n)
	}
}

func TestAPreviewRefusesAContactWithNoLiveAddressAsTheSendDoes(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.e.Pool.Exec(context.Background(), `UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, l.contact); err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](l.contact)
	var refusal *consent.ValidationError
	_, previewErr := l.store.PreviewControllerMail(l.ctx, contact, consent.LinkRecordConfirmation, "")
	_, sendErr := l.store.IssueConfirmToken(l.ctx, contact, "")
	if !errors.As(previewErr, &refusal) || !errors.As(sendErr, &refusal) {
		t.Errorf("preview %v, send %v: both must refuse a contact with no live address", previewErr, sendErr)
	}
}

func TestTheNetworkReceiptOfARelaySendCarriesTheSameMark(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	staged, _, _ := stagedControllerMail(t, l)
	// A relay send is not engagement and makes no edge of its own; it is a
	// receipt on the edge the rep's own correspondence makes.
	subject, outbound := "Q3 pricing", "outbound"
	own, _, err := activities.NewStore(InstallationDB(l.e.Pool)).LogActivity(l.ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &outbound, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: l.contact}},
	})
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	if err := database.WithWorkspaceTx(l.ctx, l.e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(l.ctx, tx, []ids.UUID{staged, ids.UUID(own.Id)})
	}); err != nil {
		t.Fatalf("folding the edge: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/contacts/"+l.contact.String()+"/graph", nil).WithContext(l.ctx)
	NewContactGraphReads(l.e.Pool, l.e.DB()).GetContactGraph(rec, req, crmcontracts.Id(l.contact))
	if rec.Code != http.StatusOK {
		t.Fatalf("graph → %d: %s", rec.Code, rec.Body.String())
	}
	var graph crmcontracts.ContactGraph
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	for _, edge := range graph.Edges {
		if edge.Receipts == nil {
			continue
		}
		for _, receipt := range *edge.Receipts {
			if ids.UUID(receipt.ActivityId) != staged {
				continue
			}
			if receipt.ControllerSend == nil || receipt.ControllerSend.FromAddress == nil ||
				*receipt.ControllerSend.FromAddress != laneFromAddress {
				t.Fatalf("the receipt of a relay send carries %+v, want controller_send from %q", receipt.ControllerSend, laneFromAddress)
			}
			return
		}
	}
	t.Fatalf("the relay send is no receipt on the contact's graph: %s", rec.Body.String())
}

func TestTheTimelineNamesTheFromTheRelayActuallySentWith(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	staged, _, _ := stagedControllerMail(t, l)
	var deliveryID ids.UUID
	if err := l.e.Pool.QueryRow(context.Background(), `SELECT id FROM comms_outbound WHERE activity_id = $1`, staged).Scan(&deliveryID); err != nil {
		t.Fatal(err)
	}

	const workerFrom = "privacy@worker.example.test"
	relay := testsmtp.Start(t, false)
	host, port := relay.Address(t)
	dispatcher := newSendDispatcher(l.e.Pool, nil, sendPace{limit: 10, window: time.Minute, maxAge: time.Hour}, nil, nil, nil).
		WithControllerRelay(ControllerRelayFor(mailer.SMTP{Host: host, Port: port, FromAddress: workerFrom}), l.vault)
	if outcome, _, err := dispatcher.DispatchWithWait(sendWorkerScope(l.ctx), deliveryID); err != nil || outcome != comms.OutcomeSent {
		t.Fatalf("controller dispatch: %s %v", outcome, err)
	}

	contactType := "contact"
	page, _, err := activities.NewStore(InstallationDB(l.e.Pool)).ListActivities(l.ctx, activities.ListActivitiesInput{
		EntityType: &contactType, EntityID: &l.contact,
	})
	if err != nil {
		t.Fatalf("ListActivities: %v", err)
	}
	for _, row := range page {
		if ids.UUID(row.Id) != staged {
			continue
		}
		if row.ControllerSend == nil || row.ControllerSend.FromAddress == nil || *row.ControllerSend.FromAddress != workerFrom {
			t.Errorf("the sent mail reads %+v, want the From it left with, %q, not the staged %q",
				row.ControllerSend, workerFrom, laneFromAddress)
		}
		return
	}
	t.Fatalf("the sent mail %s is not on the contact's timeline", staged)
}

func TestTheSingleMailViewMarksARelaySendAsTheTimelineDoes(t *testing.T) {
	l := setupConfirmLane(t)
	if _, err := l.store.IssueConfirmToken(l.ctx, ids.From[ids.ContactKind](l.contact), ""); err != nil {
		t.Fatalf("IssueConfirmToken: %v", err)
	}
	staged, _, _ := stagedControllerMail(t, l)

	shown, err := activities.NewStore(InstallationDB(l.e.Pool)).GetEmailPresentation(l.ctx, ids.From[ids.ActivityKind](staged), nil)
	if err != nil {
		t.Fatalf("GetEmailPresentation: %v", err)
	}
	if shown.ControllerSend == nil || shown.ControllerSend.FromAddress == nil || *shown.ControllerSend.FromAddress != laneFromAddress {
		t.Errorf("the opened relay send reads %+v, want controller_send from %q: its From names the rep",
			shown.ControllerSend, laneFromAddress)
	}
}
