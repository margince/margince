// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADraftSaveReadsEveryChoiceIntoThePatch(t *testing.T) {
	tag := ids.NewV7()
	req := crmcontracts.UpdateImportRunRequest{
		Object:       new(crmcontracts.ImportObjectContact),
		OnDuplicate:  new(crmcontracts.ImportOnDuplicateSkip),
		OnBrokenLink: new(crmcontracts.ImportOnBrokenLinkDropLink),
		DraftStep:    new(crmcontracts.ImportDraftStepMap),
		SourceName:   new("  hubspot export "),
		Parts: &[]crmcontracts.ImportPartRequest{{
			File: "contacts.csv", Object: crmcontracts.ImportObjectContact,
			Mapping: map[string]string{"Email": "email"}, SourceKey: new("Record ID"),
		}},
		ContextTagId: new(openapi_types.UUID(tag)),
	}
	patch, err := draftPatchFrom(req, true)
	if err != nil {
		t.Fatalf("a save of valid choices was refused: %v", err)
	}
	for field, got := range map[string]*string{
		"object": patch.Object, "on_duplicate": patch.OnDuplicate, "on_broken_link": patch.OnBrokenLink,
		"draft_step": patch.DraftStep, "source_name": patch.SourceName, "context_tag": patch.ContextTag,
	} {
		if got == nil {
			t.Errorf("the patch dropped %s", field)
		}
	}
	if *patch.SourceName != "hubspot export" {
		t.Errorf("source name = %q, want it trimmed to %q", *patch.SourceName, "hubspot export")
	}
	if *patch.ContextTag != tag.String() || *patch.OnDuplicate != "skip" || *patch.DraftStep != "map" {
		t.Errorf("patch = tag %q, on_duplicate %q, draft_step %q; want %q, skip, map",
			*patch.ContextTag, *patch.OnDuplicate, *patch.DraftStep, tag)
	}
	want := []migration.PartMapping{{
		File: "contacts.csv", Object: "contact", Fields: map[string]string{"Email": "email"}, SourceKey: "Record ID",
	}}
	if patch.Parts == nil || len(*patch.Parts) != 1 || !samePart((*patch.Parts)[0], want[0]) {
		t.Errorf("parts = %+v, want %+v", patch.Parts, want)
	}
}

func samePart(a, b migration.PartMapping) bool {
	return a.File == b.File && a.Object == b.Object && a.SourceKey == b.SourceKey && maps.Equal(a.Fields, b.Fields)
}

func TestADraftSaveTellsAnAbsentTagFromAnExplicitNull(t *testing.T) {
	absent, err := draftPatchFrom(crmcontracts.UpdateImportRunRequest{}, false)
	if err != nil || absent.ContextTag != nil {
		t.Errorf("a save without context_tag_id patched the tag to %v (err %v), want it left alone", absent.ContextTag, err)
	}
	cleared, err := draftPatchFrom(crmcontracts.UpdateImportRunRequest{}, true)
	if err != nil || cleared.ContextTag == nil || *cleared.ContextTag != "" {
		t.Errorf("an explicit null context_tag_id patched %v (err %v), want the tag cleared", cleared.ContextTag, err)
	}
}

func TestADraftSaveRefusesAChoiceOutsideItsValues(t *testing.T) {
	cases := []struct {
		name      string
		req       crmcontracts.UpdateImportRunRequest
		wantField string
	}{
		{"object", crmcontracts.UpdateImportRunRequest{Object: new(crmcontracts.ImportObject("invoice"))}, "object"},
		{"on duplicate", crmcontracts.UpdateImportRunRequest{OnDuplicate: new(crmcontracts.ImportOnDuplicate("merge"))}, "on_duplicate"},
		{"on broken link", crmcontracts.UpdateImportRunRequest{OnBrokenLink: new(crmcontracts.ImportOnBrokenLink("keep"))}, "on_broken_link"},
		{"draft step", crmcontracts.UpdateImportRunRequest{DraftStep: new(crmcontracts.ImportDraftStep("review"))}, "draft_step"},
		{"source name", crmcontracts.UpdateImportRunRequest{SourceName: new("crm/export")}, "source_name"},
		{"part object", crmcontracts.UpdateImportRunRequest{Parts: &[]crmcontracts.ImportPartRequest{{
			File: "deals.csv", Object: crmcontracts.ImportObject("deal"),
		}}}, "object"},
		{"zero tag", crmcontracts.UpdateImportRunRequest{ContextTagId: new(openapi_types.UUID{})}, "context_tag_id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := draftPatchFrom(c.req, true)
			var detailed *httperr.DetailedError
			if !errors.As(err, &detailed) || detailed.Status != http.StatusUnprocessableEntity {
				t.Fatalf("the save answered %v, want a 422", err)
			}
			if len(detailed.Fields) != 1 || detailed.Fields[0].Field != c.wantField {
				t.Errorf("the 422 named %+v, want field %q", detailed.Fields, c.wantField)
			}
		})
	}
}

func TestASavedBundleDraftIsTestedAsTheRequestItWasSavedFrom(t *testing.T) {
	tag := ids.NewV7()
	run := migration.Run{SourceRef: "imports/abc.zip", Mapping: &migration.RunMapping{
		Parts: []migration.PartMapping{
			{File: "companies.csv", Object: "company", Fields: map[string]string{"Name": "name"}, SourceKey: "Company ID"},
			{File: "contacts.csv", Object: "contact", Fields: map[string]string{"Email": "email"}},
		},
		SourceName: "hubspot", FileName: "export.zip", OnDuplicate: "skip", OnBrokenLink: "drop_link",
		ContextTag: tag.String(),
	}}
	req := requestOfRun(run)
	if req.SourceRef != run.SourceRef || req.Connector != crmcontracts.CreateImportRunRequestConnectorCsv {
		t.Errorf("request = source %q connector %q, want %q csv", req.SourceRef, req.Connector, run.SourceRef)
	}
	if req.Parts == nil || len(*req.Parts) != 2 || req.Object != nil {
		t.Fatalf("a bundle draft spelled parts %v and object %v, want two parts and no object", req.Parts, req.Object)
	}
	first, second := (*req.Parts)[0], (*req.Parts)[1]
	if first.Object != crmcontracts.ImportObjectCompany || first.SourceKey == nil || *first.SourceKey != "Company ID" {
		t.Errorf("first part = %+v, want the company file keyed on Company ID", first)
	}
	if second.SourceKey != nil {
		t.Errorf("a part saved without a source key sent %q, want none", *second.SourceKey)
	}
	if req.SourceName == nil || *req.SourceName != "hubspot" || req.FileName == nil || *req.FileName != "export.zip" {
		t.Errorf("source name %v and file name %v, want hubspot and export.zip", req.SourceName, req.FileName)
	}
	if req.OnDuplicate == nil || *req.OnDuplicate != crmcontracts.ImportOnDuplicateSkip ||
		req.OnBrokenLink == nil || *req.OnBrokenLink != crmcontracts.ImportOnBrokenLinkDropLink {
		t.Errorf("policies = %v / %v, want skip / drop_link", req.OnDuplicate, req.OnBrokenLink)
	}
	if req.ContextTagId == nil || req.ContextTagId.String() != tag.String() {
		t.Errorf("context tag = %v, want %s", req.ContextTagId, tag)
	}
}

func TestASingleFileDraftIsTestedAsOneObjectWithItsMapping(t *testing.T) {
	run := migration.Run{SourceRef: "imports/c.csv", Mapping: &migration.RunMapping{
		Object: "contact", Fields: map[string]string{"Email": "email"}, SourceKey: "Id", ContextTag: "not-a-uuid",
	}}
	req := requestOfRun(run)
	if req.Parts != nil || req.Object == nil || *req.Object != crmcontracts.ImportObjectContact {
		t.Fatalf("a single-file draft spelled parts %v object %v, want the contact object alone", req.Parts, req.Object)
	}
	if req.Mapping == nil || (*req.Mapping)["Email"] != "email" || req.SourceKey == nil || *req.SourceKey != "Id" {
		t.Errorf("mapping %v key %v, want Email->email keyed on Id", req.Mapping, req.SourceKey)
	}
	if req.OnDuplicate != nil || req.OnBrokenLink != nil || req.ContextTagId != nil || req.SourceName != nil {
		t.Errorf("unsaved choices were sent: %v %v %v %v", req.OnDuplicate, req.OnBrokenLink, req.ContextTagId, req.SourceName)
	}
	if bare := requestOfRun(migration.Run{SourceRef: "imports/new.csv"}); bare.Object != nil || bare.Parts != nil {
		t.Errorf("a draft with no saved mapping spelled object %v parts %v, want neither", bare.Object, bare.Parts)
	}
}

func TestADraftShowsItsProfileOnlyWhileAChoiceCanBeMade(t *testing.T) {
	stored, err := json.Marshal(crmcontracts.ImportSourceProfile{FileName: new("contacts.csv")})
	if err != nil {
		t.Fatal(err)
	}
	successor := ids.NewV7()
	draft := migration.Run{
		ID: ids.NewV7(), Status: migration.StatusPending,
		Version: 3, DraftStep: migration.DraftStepMap, Profile: stored, SupersededBy: &successor,
	}
	out := toContractImportRun(draft)
	if out.Profile == nil || out.Profile.RunId != openapi_types.UUID(draft.ID) {
		t.Fatalf("a pending draft answered profile %+v, want the stored one naming the run", out.Profile)
	}
	if out.Version != 3 || out.DraftStep == nil || *out.DraftStep != crmcontracts.ImportDraftStepMap {
		t.Errorf("version %d step %v, want 3 and map", out.Version, out.DraftStep)
	}
	if out.SupersededBy == nil || out.SupersededBy.String() != successor.String() {
		t.Errorf("superseded_by = %v, want %s", out.SupersededBy, successor)
	}
	for name, run := range map[string]migration.Run{
		"released": {Status: migration.StatusPending, Profile: stored, SourceReleased: true},
		"complete": {Status: migration.StatusComplete, Profile: stored},
		"corrupt":  {Status: migration.StatusPending, Profile: json.RawMessage(`{"columns":`)},
	} {
		if got := toContractImportRun(run); got.Profile != nil {
			t.Errorf("a %s run answered a profile, want none", name)
		}
	}
}
