// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestCatalogAndOfferArchiveHonorTheApprovedVersion(t *testing.T) {
	e := setupSeamEnv(t)
	_, stage, _ := twoStagePipeline(t, e.configEnv)
	_, pipeline, err := e.provider.StageSemantic(e.passport(), stage.UUID)
	if err != nil {
		t.Fatal(err)
	}
	deal := e.create(t, datasource.EntityDeal, `{"name":"Archive scope","pipeline_id":"`+pipeline.String()+`","stage_id":"`+stage.String()+`"}`)
	cases := []struct {
		ref   datasource.EntityRef
		patch string
	}{
		{e.create(t, datasource.EntityProduct, `{"name":"Support","currency":"EUR","unit_price_minor":1000}`), `{"name":"Updated support"}`},
		{e.create(t, datasource.EntityOfferTemplate, `{"name":"Standard","layout":{}}`), `{"name":"Updated standard","layout":{}}`},
		{e.create(t, datasource.EntityOffer, `{"deal_id":"`+deal.ID.String()+`","currency":"EUR"}`), `{"intro_text":"Updated terms"}`},
	}
	for _, tc := range cases {
		t.Run(string(tc.ref.Type), func(t *testing.T) {
			before, err := e.provider.Read(e.passport(), tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.provider.Update(e.passport(), datasource.UpdateInput{Ref: tc.ref, Patch: json.RawMessage(tc.patch), Source: "manual"}); err != nil {
				t.Fatal(err)
			}
			if _, err := e.provider.ArchiveAt(e.passport(), datasource.ArchiveInput{Ref: tc.ref, IfVersion: &before.Version}); !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("stale approval archived changed record: %v", err)
			}
			request := httptest.NewRequest(http.MethodDelete, "/archive", nil).WithContext(e.passport())
			request.Header.Set("If-Match", strconv.FormatInt(before.Version, 10))
			response := httptest.NewRecorder()
			handlers := Handlers{store: e.provider.store}
			switch tc.ref.Type {
			case datasource.EntityProduct:
				handlers.ArchiveProduct(response, request, crmcontracts.Id(tc.ref.ID), crmcontracts.ArchiveProductParams{})
			case datasource.EntityOfferTemplate:
				handlers.ArchiveOfferTemplate(response, request, crmcontracts.Id(tc.ref.ID), crmcontracts.ArchiveOfferTemplateParams{})
			case datasource.EntityOffer:
				handlers.ArchiveOffer(response, request, crmcontracts.Id(tc.ref.ID), crmcontracts.ArchiveOfferParams{})
			}
			if response.Code != http.StatusConflict {
				t.Fatalf("HTTP accepted a stale archive: %d %s", response.Code, response.Body.String())
			}
			after, err := e.provider.Read(e.passport(), tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(after.Fields, &fields); err != nil {
				t.Fatal(err)
			}
			if raw := fields["archived_at"]; len(raw) > 0 && string(raw) != "null" {
				t.Fatalf("stale attempt changed archive state: %s", raw)
			}
			if after.Version <= before.Version {
				t.Fatal("fixture did not change the version")
			}
			if _, err := e.provider.ArchiveAt(e.passport(), datasource.ArchiveInput{Ref: tc.ref, IfVersion: &after.Version}); err != nil {
				t.Fatalf("current approval refused: %v", err)
			}
			archived, err := e.provider.Read(e.passport(), tc.ref)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(archived.Fields, &fields); err != nil {
				t.Fatal(err)
			}
			if raw := fields["archived_at"]; len(raw) == 0 || string(raw) == "null" {
				t.Fatal("current approval did not archive")
			}
		})
	}
}
