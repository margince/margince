// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestALeadFromAWebsiteFormNamesTheForm(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["integrations"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	forms := webforms.NewStore(e.DB(), slog.Default())
	form, _, err := forms.CreateForm(ctx, webforms.CreateInput{
		Name: "Contact us", Sender: webforms.SenderBrowser, AllowedOrigins: []string{"https://visitor.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	intake := webforms.NewIntake(forms, newWebFormLander(e.DB(), e.Pool, slog.Default()))
	if _, err := intake.Submit(ctx, webforms.Arrival{
		FormKey: form.Key, ContentType: "application/x-www-form-urlencoded",
		Header: http.Header{"Origin": {"https://visitor.example"}},
		Body:   []byte("email=ada%40visitor.example&name=Ada&_mrg_ts=" + strconv.FormatInt(clocktest.Now(t).Add(-20*time.Second).UnixMilli(), 10)),
		Now:    clocktest.Now(t),
	}); err != nil {
		t.Fatalf("submitting the form: %v", err)
	}
	var leadID ids.UUID
	if err := e.Pool.QueryRow(t.Context(), `SELECT id FROM lead WHERE email = 'ada@visitor.example'`).Scan(&leadID); err != nil {
		t.Fatalf("finding the form's lead: %v", err)
	}
	lead, err := e.Contacts.GetLead(ctx, ids.From[ids.LeadKind](leadID), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if lead.WebFormName == nil || *lead.WebFormName != "Contact us" {
		t.Fatalf("web_form_name = %v, want the form's name", lead.WebFormName)
	}
}
