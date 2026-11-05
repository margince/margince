// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnActiveWebsiteFormMustBeResolvedBeforeItsOwnerIsMerged(t *testing.T) {
	e := integration.Setup(t)
	manager := memberManagerFor(t, e)
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["integrations"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	forms := webforms.NewStore(e.DB(), slog.Default())
	form, _, err := forms.CreateForm(ctx, webforms.CreateInput{Name: "Website enquiries", Sender: webforms.SenderBrowser, AllowedOrigins: []string{"https://visitor.example"}, OwnerID: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(preview.Blockers, "web_forms") {
		t.Fatalf("active form was not a blocker: %+v", preview.Blockers)
	}
	state := webforms.StatePaused
	if _, err := forms.UpdateForm(ctx, form.ID, webforms.UpdateInput{State: &state}); err != nil {
		t.Fatal(err)
	}
	preview, err = manager.Preview(ctx, e.Rep1, &e.Rep2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 0 {
		t.Fatalf("paused form still blocks retirement: %+v", preview.Blockers)
	}
	if err := manager.Merge(ctx, e.Rep1, e.Rep2, preview.Token); err != nil {
		t.Fatal(err)
	}
	state = webforms.StateActive
	if _, err := forms.UpdateForm(ctx, form.ID, webforms.UpdateInput{State: &state}); err == nil {
		t.Fatal("reactivated a form owned by a retired member")
	}
	if _, err := forms.UpdateForm(ctx, form.ID, webforms.UpdateInput{State: &state, OwnerID: &e.Rep2}); err != nil {
		t.Fatal(err)
	}
}
