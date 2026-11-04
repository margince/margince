// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent's offer is issued by the entity of the human it acts for.
func TestAnAgentsOfferTakesTheEntityOfTheHumanItActsFor(t *testing.T) {
	human := ids.NewV7()
	want := ids.New[ids.LegalEntityKind]()
	var asked ids.UserID
	store := NewStore(nil, Installation{
		AuthorEntity: func(_ context.Context, _ pgx.Tx, user ids.UserID) (ids.LegalEntityID, error) {
			asked = user
			return want, nil
		},
	})
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(), OnBehalfOf: human,
	})
	got, err := store.offerEntity(agent, nil, nil)
	if err != nil {
		t.Fatalf("resolve the agent's entity: %v", err)
	}
	if asked.UUID != human || got != want {
		t.Errorf("the agent's offer asked for user %s and got %s, want the human %s and their entity", asked, got, human)
	}
}

// A sent offer never reads the entity as it is now. One sent before
// snapshots prints the installation's name.
func TestASentOfferWithoutASnapshotNeverReadsTheLiveEntity(t *testing.T) {
	store := NewStore(nil, Installation{
		Name:            func(context.Context, pgx.Tx) (string, error) { return "Margince Test", nil },
		IssuerLegalName: func(context.Context, pgx.Tx) (string, error) { return "", nil },
		IssuerEntity: func(context.Context, pgx.Tx, *ids.LegalEntityID) (IssuerEntity, error) {
			return IssuerEntity{}, errors.New("a sent offer read the live entity")
		},
	})
	for _, snapshot := range []*map[string]any{nil, {}} {
		issuer, err := store.resolveRenderIssuer(context.Background(), nil, crmcontracts.Offer{
			Status: crmcontracts.OfferStatusSent, IssuerSnapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("render a sent offer's issuer: %v", err)
		}
		if issuer.Name != "Margince Test" || issuer.LogoKey != "" || len(issuer.Address) != 0 {
			t.Errorf("a sent offer without a snapshot renders %+v, want the installation's name alone", issuer)
		}
	}
}

// The render reads the logo the issuer block names, and a logo whose object
// is gone leaves the document without one.
func TestTheRenderReadsTheNamedLogoAndSkipsAMissingOne(t *testing.T) {
	blob := blobstore.NewMemory()
	ctx := context.Background()
	if err := blob.Put(ctx, "logos/frozen.png", bytes.NewReader([]byte("png")), 3, "image/png"); err != nil {
		t.Fatal(err)
	}
	h := Handlers{}.WithBlobstore(blob)
	if got := h.issuerLogo(ctx, "logos/frozen.png"); string(got) != "png" {
		t.Errorf("the render read %q for the named logo", got)
	}
	if got := h.issuerLogo(ctx, "logos/swept.png"); got != nil {
		t.Errorf("a missing logo object read as %q, want none", got)
	}
}

// A template locale names its language with `-` or `_`. A locale whose
// language is no ISO 639-1 code takes the default template's. So a stray
// template value never reaches the offer's language check in the table.
func TestATemplateLocaleGivesItsLanguageOrTheDefault(t *testing.T) {
	for locale, want := range map[string]string{
		"de-DE": "de", "de_AT": "de", "en-GB": "en", "EN_us": "en", "fr": "fr",
		"": "de", "x": "de", "deu-DE": "de", "12-34": "de",
	} {
		if got := languageOfLocale(locale); got != want {
			t.Errorf("locale %q gives language %q, want %q", locale, got, want)
		}
	}
}
