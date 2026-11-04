// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A sender who works for a further legal entity signs with that entity's
// template, logo and imprint. The test goes through the real wiring: the
// signature seam, the send's PrepareSend and the delivery row the dispatcher
// later reads. The composer and the approval preview show the same sign-off.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAnEntitySenderSignsWithTheEntityThroughTheRealSend(t *testing.T) {
	e := integration.Setup(t)
	anchorID, recipient := seedTransactionalReply(t, e)
	admin := e.Admin()
	store := contacts.NewStore(InstallationDB(e.Pool)).WithSettings(StoredConfig(e.Pool))
	if _, err := store.SaveSignatureTemplate(admin, "<p>Company template</p>"); err != nil {
		t.Fatalf("save the workspace template: %v", err)
	}
	companyLogo := "logos/" + ids.NewV7().String() + ".png"
	e.WsExec(t, `INSERT INTO company (id, display_name, is_anchor, logo_object_key, logo_origin, source, captured_by)
		VALUES ($1, 'Acme', true, $2, 'upload', 'manual', 'human:x')`, ids.NewV7(), companyLogo)
	template, imprint := "<p>{logo}<b>{name}</b></p><p>{imprint}</p>", "Acme Wien GmbH\nFN 123456 a"
	vienna, err := store.CreateLegalEntity(admin, contacts.LegalEntityInput{
		DisplayName: new("Acme Wien"), LegalName: new("Acme Wien GmbH"), Country: new("at"),
		Imprint: &imprint, SignatureTemplate: &template,
	})
	if err != nil {
		t.Fatalf("create Vienna: %v", err)
	}
	entityLogo := "logos/" + ids.NewV7().String() + ".png"
	e.WsExec(t, `UPDATE legal_entity SET logo_object_key = $2 WHERE id = $1`, vienna.ID, entityLogo)
	for _, sender := range []ids.UUID{e.Rep1, e.AdminUser} {
		if err := store.AssignLegalEntityMember(admin, vienna.ID, ids.From[ids.UserKind](sender)); err != nil {
			t.Fatalf("assign a sender to Vienna: %v", err)
		}
	}
	const wantText = "Rep\nAcme Wien GmbH\nFN 123456 a"
	wantMarkup := []string{"cid:" + connector.SignatureLogoContentID, "<b>Rep</b>", "Acme Wien GmbH<br>FN 123456 a"}

	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
	if preview := previewSignOff(ctx, t, e); preview.Kind != "template" || preview.Text != wantText {
		t.Fatalf("the composer previews %+v, want Vienna's template", preview)
	}
	adapter := newCommsAdapter(e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: realDeliveryStager(t, e)})
	if _, err := adapter.SendEmail(ctx, anchorID, agents.SendEmailArgs{
		To: []string{recipient}, Subject: "Angebot", Body: signOffBody, ConsentPurpose: "transactional",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	body := e.WsScalar(t, `SELECT body FROM comms_outbound`)
	markup := e.WsScalar(t, `SELECT coalesce(html_body, '') FROM comms_outbound`)
	logo := e.WsScalar(t, `SELECT coalesce(inline_logo_key, '') FROM comms_outbound`)
	if body != signOffBody+"\n\n"+wantText {
		t.Errorf("the delivery row's text = %q, want the message over Vienna's sign-off", body)
	}
	for _, want := range wantMarkup {
		if !strings.Contains(markup, want) {
			t.Errorf("the delivery row's markup %q lacks %q", markup, want)
		}
	}
	if strings.Contains(markup, "Company template") || strings.Contains(body, "Company template") {
		t.Errorf("the workspace template signed a Vienna sender's mail")
	}
	if logo != entityLogo {
		t.Errorf("the delivery row stages logo %q, want Vienna's %q and never the company's %q", logo, entityLogo, companyLogo)
	}

	assertApprovalPreviewSignsAsVienna(t, e)
}

// The approval of a mail an admin proposes previews the sign-off the send
// would add, which for a Vienna sender is Vienna's.
func assertApprovalPreviewSignsAsVienna(t *testing.T, e *integration.Env) {
	t.Helper()
	ctx := e.Admin()
	company := e.SeedCompany(t, "Harbor account", &e.AdminUser)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(
		`{"to":["buyer@example.test"],"body":"Delivery Friday.","links":[{"entity_type":"company","entity_id":%q}]}`, company)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: "send_company_email", TargetType: "activity", ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	proposal, err := svc.GetWire(ctx, id)
	if err != nil {
		t.Fatalf("read the proposal: %v", err)
	}
	if proposal.Preview == nil || proposal.Preview.Details == nil {
		t.Fatalf("the approval has no preview: %+v", proposal.Preview)
	}
	signed := map[string]string{}
	for _, detail := range *proposal.Preview.Details {
		signed[detail.Field] = detail.Value.Text
	}
	if !strings.Contains(signed["signature_text"], "Acme Wien GmbH\nFN 123456 a") {
		t.Errorf("the approval previews the sign-off %q, want Vienna's imprint", signed["signature_text"])
	}
	if !strings.Contains(signed["signature_html"], "cid:"+connector.SignatureLogoContentID) {
		t.Errorf("the approval previews the markup %q, want Vienna's logo", signed["signature_html"])
	}
}
