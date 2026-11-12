// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func declareKinds(t *testing.T) {
	t.Helper()
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	field := []extension.RecordKindField{{Name: "code", Label: "Code"}}
	recordkinds.Register("beta", []extension.RecordKindDeclaration{{Kind: "visit", DisplayName: "Beta visit", Table: "ext_beta_visit", Fields: field, NaturalKey: []string{"code"}}})
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "site", DisplayName: "Site", Table: "ext_alpha_site", Fields: field, NaturalKey: []string{"code"},
		DeletePolicy: extension.DeleteErasable,
	}})
}

func listKinds(ctx context.Context, t *testing.T) crmcontracts.RecordKindDirectory {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/record-kinds", nil).WithContext(ctx)
	channelProvidersHandlers{}.ListRecordKinds(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got crmcontracts.RecordKindDirectory
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func seat(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", Permissions: principal.Permissions{Objects: objects},
	})
}

func TestTheRecordKindDirectoryListsOnlyKindsTheSeatMayRead(t *testing.T) {
	declareKinds(t)
	got := listKinds(seat(map[string]principal.ObjectGrant{
		"ext_alpha_site": {Read: true, Update: true},
		"ext_beta_visit": {Create: true},
		"contact":        {Read: true},
	}), t)
	if len(got.Data) != 1 {
		t.Fatalf("data = %+v, want only the kind the seat may read", got.Data)
	}
	site := got.Data[0]
	if site.Name != "ext:alpha:site" || site.Unit != "alpha" || site.DisplayName != "Site" {
		t.Errorf("site = %+v", site)
	}
	if site.DeletePolicy != crmcontracts.RecordKindOptionDeletePolicyErasable {
		t.Errorf("delete_policy = %q", site.DeletePolicy)
	}
	if len(site.Actions) != 2 || site.Actions[0] != "read" || site.Actions[1] != "update" {
		t.Errorf("actions = %v, want the two the seat holds", site.Actions)
	}
	if len(site.NaturalKey) != 1 || site.NaturalKey[0] != "code" || len(site.Fields) != 1 {
		t.Errorf("fields/natural_key = %+v / %v", site.Fields, site.NaturalKey)
	}
}

func TestTheRecordKindDirectoryIsEmptyForASeatWithNoGrantAndNotNull(t *testing.T) {
	declareKinds(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/record-kinds", nil).WithContext(seat(nil))
	channelProvidersHandlers{}.ListRecordKinds(rec, req)
	if body := rec.Body.String(); body != "{\"data\":[]}\n" && body != "{\"data\":[]}" {
		t.Errorf("body = %q, want an empty data array and not null", body)
	}
}
