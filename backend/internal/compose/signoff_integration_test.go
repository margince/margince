// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The sign-off a send appends, through the real wiring: the signature row
// contacts owns, the display name identity owns, and the composer's preview
// answering from the same code the send runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const signOffBody = "Hallo Anna, wie besprochen schicke ich dir das Angebot und die Unterlagen für das Projekt. Ich freue mich auf deine Rückmeldung."

// A sender with no signature signs off with a plain closing in the message's
// language and their name; once they write one, the send carries that
// signature and nothing else. Each time, the preview names the block the send
// then appended.
func TestASendSignsOffTheWayTheComposerPreviewSays(t *testing.T) {
	e := integration.Setup(t)
	anchorID, recipient := seedTransactionalReply(t, e)
	stager := &recordingStager{}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)

	send := func() string {
		t.Helper()
		if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{
			To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional",
		}); err != nil {
			t.Fatalf("send: %v", err)
		}
		return stager.staged[len(stager.staged)-1].Body
	}

	preview := previewSignOff(ctx, t, e)
	if preview.Kind != "closing" || preview.Text != "Viele Grüße,\nRep" {
		t.Fatalf("preview with no signature = %+v, want the German closing over the sender's name", preview)
	}
	if got, want := send(), signOffBody+"\n\n"+preview.Text; got != want {
		t.Fatalf("sent body = %q, want %q", got, want)
	}

	if _, err := contacts.NewStore(InstallationDB(e.Pool)).SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Marek Janetzke\nGradion"}); err != nil {
		t.Fatalf("save signature: %v", err)
	}
	preview = previewSignOff(ctx, t, e)
	if preview.Kind != "signature" || preview.Text != "Marek Janetzke\nGradion" {
		t.Fatalf("preview with a signature = %+v, want the signature alone", preview)
	}
	got := send()
	if want := signOffBody + "\n\n" + preview.Text; got != want {
		t.Fatalf("sent body = %q, want %q", got, want)
	}
	if strings.Contains(got, "Viele Grüße") {
		t.Fatalf("a signed send also carried the closing: %q", got)
	}
}

// A workspace template signs every member's mail in their own values, over
// their own plain-text signature. It lands as markup in the HTML part and as
// lines in the text part. A plain send gains the HTML part that carries it.
func TestATemplateSignsAPlainSendInMarkupAndText(t *testing.T) {
	e := integration.Setup(t)
	anchorID, recipient := seedTransactionalReply(t, e)
	stager := &recordingStager{}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	// The logo is the workspace's own company, which a seat reads like any other.
	perms := integration.SchedulerPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	if _, err := contacts.NewStore(InstallationDB(e.Pool)).WithSettings(StoredConfig(e.Pool)).
		SaveSignatureTemplate(e.Admin(), `<p>{logo}<b>{name}</b><br>{title}<br><span style="color:#2a7">{phone}</span></p>`); err != nil {
		t.Fatalf("save template: %v", err)
	}
	logoKey := "logos/" + ids.NewV7().String() + ".png"
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, logo_origin, source, captured_by)
		VALUES ($1, 'Company A', true, $2, 'upload', 'manual', 'human:x')`, ids.NewV7(), logoKey)
	signatures := contacts.NewStore(InstallationDB(e.Pool))
	title, phone := "Head of Sales", "+49 30 1234"
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{
		Body: "my own text", Title: &title, Phone: &phone,
	}); err != nil {
		t.Fatalf("save signature: %v", err)
	}
	// A save that sends only the text, as a client unaware of the fields does,
	// keeps the title and phone.
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "my own text"}); err != nil {
		t.Fatalf("save text only: %v", err)
	}
	if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{
		To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	sent := stager.staged[len(stager.staged)-1]
	if want := signOffBody + "\n\nRep\nHead of Sales\n+49 30 1234"; sent.Body != want {
		t.Fatalf("text part = %q, want %q", sent.Body, want)
	}
	for _, want := range []string{"<b>Rep</b>", "Head of Sales", `style="color:#2a7"`} {
		if !strings.Contains(sent.HTMLBody, want) {
			t.Errorf("HTML part %q lacks %q", sent.HTMLBody, want)
		}
	}
	if strings.Contains(sent.Body, "my own text") || strings.Contains(sent.HTMLBody, "my own text") {
		t.Errorf("the member's own text signed alongside the template")
	}
	if sent.InlineLogoKey != logoKey || !strings.Contains(sent.HTMLBody, "cid:"+connector.SignatureLogoContentID) {
		t.Errorf("the staged message embeds logo %q in %q, want %q", sent.InlineLogoKey, sent.HTMLBody, logoKey)
	}
	personal := "<p>Personal {name}</p>" + connector.SignatureLogoTag
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "my own text", HTML: &personal}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional"}); err != nil {
		t.Fatal(err)
	}
	sent = stager.staged[len(stager.staged)-1]
	if sent.InlineLogoKey != logoKey || !strings.Contains(sent.HTMLBody, connector.SignatureLogoTag) || sent.Body != signOffBody+"\n\nPersonal {name}" {
		t.Fatalf("personal logo send = %+v", sent)
	}
}

// previewSignOff asks POST /emails:sign-off through the handlers the server
// mounts.
func previewSignOff(ctx context.Context, t *testing.T, e *integration.Env) struct{ Text, Kind string } {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"body": signOffBody, "subject": "Angebot"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/emails:sign-off", bytes.NewReader(payload)).WithContext(ctx)
	rec := httptest.NewRecorder()
	newActivitiesHandlers(e.Pool).PreviewEmailSignOff(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview answered %d: %s", rec.Code, rec.Body.String())
	}
	var out struct{ Text, Kind string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	return out
}

// seedTransactionalReply seeds a contact who granted transactional mail and a
// thread to answer, so the send passes its gates and carries no footer.
func seedTransactionalReply(t *testing.T, e *integration.Env) (anchorID ids.UUID, recipient string) {
	t.Helper()
	admin := e.Admin()
	recipient = "anna@buyer.test"
	contact := e.SeedContact(t, "Anna Buyer", &e.Rep1)
	addContactEmail(t, e, contact, recipient)
	consentStore := consent.NewStore(InstallationDB(e.Pool))
	purpose, err := consentStore.CreatePurpose(admin, "transactional", "transactional", false)
	if err != nil {
		t.Fatalf("create purpose: %v", err)
	}
	if _, err := consentStore.Record(admin, consent.RecordInput{
		ContactID: ids.From[ids.ContactKind](contact), PurposeID: purpose.ID, NewState: "granted",
		PolicyText: &grantedWording,
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	anchorID = ids.NewV7()
	if err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'Angebot', now(), 'manual', 'human:x')`, anchorID)
		return err
	}); err != nil {
		t.Fatalf("seed anchor: %v", err)
	}
	return anchorID, recipient
}

func TestPersonalMarkupAndItsClearingReachBothAlternativesThroughTheRealSend(t *testing.T) {
	e := integration.Setup(t)
	anchorID, recipient := seedTransactionalReply(t, e)
	stager := &recordingStager{}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	signatures := contacts.NewStore(InstallationDB(e.Pool)).WithSettings(StoredConfig(e.Pool))
	if _, err := signatures.SaveSignatureTemplate(e.Admin(), "<p>Company template</p>"); err != nil {
		t.Fatal(err)
	}
	own := `<p onclick="bad()"><b>Personal HTML</b><br>{name}</p><script>bad()</script><img src="https://tracker.test/pixel">`
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Saved plain", HTML: &own}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ body, markup, text string }{
		{signOffBody, "", "Personal HTML\n{name}"},
		{signOffBody, "<p>Body with markup</p>", "Personal HTML\n{name}"},
	} {
		if _, err := adapter.store.SendEmail(ctx, activities.FromActivity(ids.From[ids.ActivityKind](anchorID)), activities.SendEmailInput{Recipients: []string{recipient}, Subject: "Angebot", Body: test.body, HTMLBody: test.markup, ConsentPurpose: "transactional"}, adapter.gate, adapter.stager); err != nil {
			t.Fatal(err)
		}
		sent := stager.staged[len(stager.staged)-1]
		if sent.Body != test.body+"\n\n"+test.text {
			t.Errorf("text = %q", sent.Body)
		}
		if strings.Count(sent.HTMLBody, "<b>Personal HTML</b>") != 1 {
			t.Errorf("markup = %q", sent.HTMLBody)
		}
		for _, denied := range []string{"Company template", "Saved plain", "onclick", "bad()", "tracker.test", "Viele Grüße"} {
			if strings.Contains(sent.HTMLBody, denied) || strings.Contains(sent.Body, denied) {
				t.Errorf("send kept %q: %+v", denied, sent)
			}
		}
	}
	empty := ""
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Saved plain", HTML: &empty}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional"}); err != nil {
		t.Fatal(err)
	}
	sent := stager.staged[len(stager.staged)-1]
	if sent.Body != signOffBody+"\n\nCompany template" || !strings.Contains(sent.HTMLBody, "<p>Company template</p>") || strings.Contains(sent.HTMLBody, "Personal HTML") {
		t.Fatalf("cleared send = %+v", sent)
	}
}

func TestPersonalMarkupOmissionClearingAndUnicodeLimitsUseTheSignatureStore(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	signatures := contacts.NewStore(InstallationDB(e.Pool))
	own := "<p>" + strings.Repeat("界", contacts.SignatureMaxRunes-7) + "</p>"
	saved, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Saved plain", HTML: &own})
	if err != nil || saved.HTML != own {
		t.Fatalf("boundary save = %+v, %v", saved, err)
	}
	saved, err = signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "New plain"})
	if err != nil || saved.HTML != own || saved.Body != "New plain" {
		t.Fatalf("omitted save = %+v, %v", saved, err)
	}
	tooLong := own + "界"
	_, err = signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "New plain", HTML: &tooLong})
	var limit *contacts.SignatureTooLongError
	if !errors.As(err, &limit) || limit.Field != "html" || limit.Runes != contacts.SignatureMaxRunes+1 {
		t.Fatalf("limit error = %v", err)
	}
	payload, err := json.Marshal(map[string]string{"body": "New plain", "html": tooLong})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/v1/me/email-signature", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	contacts.NewHandlers(InstallationDB(e.Pool)).SaveMyEmailSignature(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"field":"html"`) {
		t.Fatalf("HTTP limit = %d %s", response.Code, response.Body.String())
	}
	empty := ""
	saved, err = signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "New plain", HTML: &empty})
	if err != nil || saved.HTML != "" || saved.Body != "New plain" {
		t.Fatalf("clearing save = %+v, %v", saved, err)
	}
	if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "", HTML: &own}); err != nil {
		t.Fatal(err)
	}
	var proof string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_object('before', a.before, 'after', a.after, 'event', o.envelope))::text
   FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
   WHERE o.envelope->>'type' = 'email_signature.changed'`).Scan(&proof)
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id=(o.envelope->'trace'->>'audit_log_id')::uuid
 WHERE o.envelope->>'type'='email_signature.changed' AND a.before ? 'has_html_signature' AND a.after ? 'has_html_signature'`); got != 4 {
		t.Fatalf("HTML audit presence recorded %d changes, want 4", got)
	}
	for _, private := range []string{"界", "Saved plain", "New plain", "<p>"} {
		if strings.Contains(proof, private) {
			t.Errorf("audit or event contains signature content %q", private)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'email_signature.changed' AND envelope->'payload'->>'has_signature' = 'true'`); got != 4 {
		t.Errorf("signed change events = %d, want 4 including HTML without plain text", got)
	}
}

func TestSignatureDraftAndSaveRefuseInvalidOptionalMarkup(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	for _, test := range []struct {
		preview bool
		body    string
	}{
		{true, `{"body":"","draft":{"html":"` + strings.Repeat("界", contacts.SignatureMaxRunes+1) + `"}}`},
		{false, `{"body":"","html":null}`},
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/emails:sign-off", strings.NewReader(test.body)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		if test.preview {
			newActivitiesHandlers(e.Pool).PreviewEmailSignOff(response, request)
		} else {
			contacts.NewHandlers(InstallationDB(e.Pool)).SaveMyEmailSignature(response, request)
		}
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid markup accepted: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestDepartedSendersCannotChangeTheirSignature(t *testing.T) {
	for _, departure := range []string{"deactivated", "archived"} {
		t.Run(departure, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
			signatures := contacts.NewStore(InstallationDB(e.Pool))
			if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Original"}); err != nil {
				t.Fatal(err)
			}
			if departure == "deactivated" {
				e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1)
			} else {
				e.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep1)
			}
			if _, err := signatures.SaveMyEmailSignature(ctx, contacts.SaveSignatureInput{Body: "Changed"}); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("departed save: %v", err)
			}
			if got := e.WsScalar(t, `SELECT body FROM email_signature WHERE owner_id = $1`, e.Rep1); got != "Original" {
				t.Fatalf("stored signature = %q", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'email_signature.changed'`); got != 1 {
				t.Fatalf("signature events = %d", got)
			}
		})
	}
}
