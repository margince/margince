// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A sender's mail signs with the legal entity they work for: its template,
// else the workspace's, and its logo and imprint. A sender of the default
// entity signs as every sender did before entities existed.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	workspaceTemplate = "<p>{name} · Acme</p>"
	viennaTemplate    = "<p>{logo}{name}</p><p>{imprint}</p>"
	anchorLogoKey     = "logos/acme.png"
	viennaLogoKey     = "logos/wien.png"
	viennaImprint     = "Acme Wien GmbH\nFN 123456 a"
)

// signingEnv is two further entities beside the default, one sender in each,
// and the workspace's own template and company logo.
type signingEnv struct {
	*promoteConsentEnv
	defaultSender, viennaSender, grazSender ids.UUID
}

func setupSigning(t *testing.T, withWorkspaceTemplate bool) signingEnv {
	t.Helper()
	e := setupPromoteConsent(t)
	ctx := context.Background()
	admin := actingAs(e, "admin", entityAdmin)
	company, err := e.store.WithInstallationRenamer(ignoreInstallationName).SaveCompany(admin, SaveCompanyInput{DisplayName: "Acme"})
	if err != nil {
		t.Fatalf("save the company: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE company SET logo_object_key = $2, logo_origin = 'upload' WHERE id = $1`,
		company.CompanyID, anchorLogoKey); err != nil {
		t.Fatalf("give the company a logo: %v", err)
	}
	if withWorkspaceTemplate {
		if _, err := e.owner.Exec(ctx, `INSERT INTO setting (key, value, updated_at) VALUES ($1, to_jsonb($2::text), now())`,
			SignatureTemplate.Key(), workspaceTemplate); err != nil {
			t.Fatalf("save the workspace template: %v", err)
		}
	}
	vienna, err := e.store.CreateLegalEntity(admin, LegalEntityInput{
		DisplayName: new("Acme Wien"), LegalName: new("Acme Wien GmbH"), Country: new("at"),
		Imprint: new(viennaImprint), SignatureTemplate: new(viennaTemplate),
	})
	if err != nil {
		t.Fatalf("create Vienna: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE legal_entity SET logo_object_key = $2 WHERE id = $1`,
		vienna.ID, viennaLogoKey); err != nil {
		t.Fatalf("give Vienna a logo: %v", err)
	}
	graz, err := e.store.CreateLegalEntity(admin, LegalEntityInput{
		DisplayName: new("Acme Graz"), LegalName: new("Acme Graz GmbH"), Country: new("at"),
	})
	if err != nil {
		t.Fatalf("create Graz: %v", err)
	}
	env := signingEnv{promoteConsentEnv: e, defaultSender: e.user, viennaSender: ids.NewV7(), grazSender: ids.NewV7()}
	for entity, user := range map[ids.LegalEntityID]ids.UUID{vienna.ID: env.viennaSender, graz.ID: env.grazSender} {
		if _, err := e.owner.Exec(ctx, `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`,
			user, "sender-"+user.String()+"@acme.test", "Sender"); err != nil {
			t.Fatalf("add a sender: %v", err)
		}
		if err := e.store.AssignLegalEntityMember(admin, entity, ids.From[ids.UserKind](user)); err != nil {
			t.Fatalf("assign a sender: %v", err)
		}
	}
	return env
}

// as is the sender's own session: a send only ever asks for its own signature.
func (e signingEnv) as(user ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, Objects: entityAdmin, RowScope: principal.RowScopeAll},
	})
}

func (e signingEnv) signatureOf(t *testing.T, user ids.UUID) SenderSignature {
	t.Helper()
	signature, err := e.store.SignatureFor(e.as(user), user)
	if err != nil {
		t.Fatalf("read the signature of %s: %v", user, err)
	}
	return signature
}

func TestEachSenderSignsWithTheirEntitysTemplateLogoAndImprint(t *testing.T) {
	e := setupSigning(t, true)
	for _, tc := range []struct {
		name   string
		sender ids.UUID
		want   SenderSignature
	}{
		// What a sender got before entities: the workspace's template and the
		// company logo.
		{"the default entity", e.defaultSender, SenderSignature{Template: workspaceTemplate, LogoKey: anchorLogoKey}},
		{
			"an entity with its own template and logo", e.viennaSender,
			SenderSignature{Template: viennaTemplate, Imprint: viennaImprint, LogoKey: viennaLogoKey},
		},
		// No template of its own falls back to the workspace's; no logo of its
		// own shows none rather than the company's.
		{"an entity with neither", e.grazSender, SenderSignature{Template: workspaceTemplate}},
	} {
		if got := e.signatureOf(t, tc.sender); got != tc.want {
			t.Errorf("%s: a sender signs with %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestTheSettingsTabSaysAnEntityTemplateSigns(t *testing.T) {
	e := setupSigning(t, false)
	for sender, want := range map[ids.UUID]bool{e.defaultSender: false, e.viennaSender: true, e.grazSender: false} {
		got, err := e.store.GetMyEmailSignature(e.as(sender))
		if err != nil {
			t.Fatalf("read the settings tab: %v", err)
		}
		if got.TemplateActive != want {
			t.Errorf("sender %s: template active = %v, want %v", sender, got.TemplateActive, want)
		}
	}
}

func TestTheDefaultEntitySignsWithTheWorkspaceTemplate(t *testing.T) {
	e := setupSigning(t, true)
	admin := actingAs(e.promoteConsentEnv, "admin", entityAdmin)
	_, err := e.store.UpdateLegalEntity(admin, defaultEntity(t, e.promoteConsentEnv),
		LegalEntityInput{SignatureTemplate: new("<p>{name}</p>")}, nil)
	if fault, ok := errors.AsType[*values.ParseError](err); !ok || fault.Code != codeFieldOnInstallation {
		t.Errorf("a template on the default entity answered %v, want %s", err, codeFieldOnInstallation)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE legal_entity SET signature_template = '<p>x</p>' WHERE is_default`); err == nil {
		t.Error("the table took a template on the default entity")
	}
}
