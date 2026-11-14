// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordreferences_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/recordreferences"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Only the reference route is exercised here; production wiring is covered in compose/integration.
type referenceServer struct {
	crmcontracts.ServerInterface
	handlers recordreferences.Handlers
}

func (s referenceServer) ReadRecordReferences(w http.ResponseWriter, r *http.Request, params crmcontracts.ReadRecordReferencesParams) {
	s.handlers.ReadRecordReferences(w, r, params)
}

func referenceRouter(e *integration.Env) http.Handler {
	server := referenceServer{handlers: recordreferences.NewHandlers(e.Contacts, e.Deals, e.Projects)}
	return crmcontracts.HandlerFromMuxWithBaseURL(server, chi.NewRouter(), "/v1")
}

func referenceRequest(ctx context.Context, router http.Handler, kind string, want ...ids.UUID) *httptest.ResponseRecorder {
	query := url.Values{"kind": {kind}}
	for _, id := range want {
		query.Add("id", id.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/records/references?"+query.Encode(), nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func referenceResponse(t *testing.T, response *httptest.ResponseRecorder) []crmcontracts.RecordReference {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("reference response %d: %s", response.Code, response.Body.String())
	}
	var out crmcontracts.RecordReferenceListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data == nil {
		t.Fatal("data is null, want an array")
	}
	return out.Data
}

func TestRecordReferencesMatchDetailReadsAndKeepArchivedNames(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			fixture := makeReferenceFixture(t, e, kind, "Reference record")
			router := referenceRouter(e)
			for _, archived := range []bool{false, true} {
				if archived {
					if err := fixture.archive(); err != nil {
						t.Fatal(err)
					}
				}
				want, err := fixture.detail(e.Admin())
				if err != nil {
					t.Fatal(err)
				}
				if want.Archived != archived {
					t.Fatalf("detail archived=%v, want %v", want.Archived, archived)
				}
				got := referenceResponse(t, referenceRequest(e.Admin(), router, kind, fixture.id))
				if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
					t.Fatalf("references=%+v, detail=%+v", got, want)
				}
				direct, err := fixture.read(e.Admin(), []ids.UUID{fixture.id})
				if err != nil || !reflect.DeepEqual(direct, got) {
					t.Fatalf("module=%+v err=%v, route=%+v", direct, err, got)
				}
				labels, err := fixture.labels(e.Admin(), []ids.UUID{fixture.id})
				if err != nil {
					t.Fatal(err)
				}
				if archived && len(labels) != 0 {
					t.Fatalf("archived legacy labels=%v", labels)
				}
				if !archived && labels[fixture.id] != "Reference record" {
					t.Fatalf("legacy labels=%v", labels)
				}
			}
		})
	}
}

func TestRecordReferencesPreserveBoundsOrderAndObjectDenials(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			first := makeReferenceFixture(t, e, kind, "First reference")
			second := makeReferenceFixture(t, e, kind, "Second reference")
			router := referenceRouter(e)
			got := referenceResponse(t, referenceRequest(e.Admin(), router, kind, second.id, first.id, second.id, ids.NewV7()))
			if len(got) != 2 || ids.UUID(got[0].Id) != second.id || ids.UUID(got[1].Id) != first.id {
				t.Fatalf("wrong order or duplicates: %+v", got)
			}
			hundred := make([]ids.UUID, 100)
			for i := range hundred {
				hundred[i] = ids.NewV7()
			}
			hundred[0] = first.id
			if got := referenceResponse(t, referenceRequest(e.Admin(), router, kind, hundred...)); len(got) != 1 {
				t.Fatalf("100 ids: %+v", got)
			}
			tooMany := slices.Clone(hundred)
			tooMany = append(tooMany, first.id)
			if response := referenceRequest(e.Admin(), router, kind, tooMany...); response.Code != 422 {
				t.Fatalf("101 ids: %d %s", response.Code, response.Body.String())
			}
			for i := range tooMany {
				tooMany[i] = first.id
			}
			if response := referenceRequest(e.Admin(), router, kind, tooMany...); response.Code != 422 {
				t.Fatalf("101 repetitions: %d %s", response.Code, response.Body.String())
			}
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			delete(perms.Objects, kind)
			denied := e.As(e.AdminUser, nil, perms)
			if response := referenceRequest(denied, router, kind, first.id); response.Code != 403 {
				t.Fatalf("object denial=%d %s", response.Code, response.Body.String())
			}
			if _, err := first.read(denied, []ids.UUID{first.id}); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("module denial=%v", err)
			}
			if response := referenceRequest(denied, router, kind, tooMany...); response.Code != 403 {
				t.Fatalf("bounds preceded grant: %d", response.Code)
			}
			missing := referenceResponse(t, referenceRequest(e.Admin(), router, kind, ids.NewV7()))
			if len(missing) != 0 {
				t.Fatalf("missing rows=%+v", missing)
			}
		})
	}
}

func TestRecordReferencesOmitPrivateContactsLikeDetailReads(t *testing.T) {
	e := integration.Setup(t)
	fixture := makeReferenceFixture(t, e, "contact", "Private reference")
	id := ids.From[ids.ContactKind](fixture.id)
	if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	colleague := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := fixture.detail(colleague); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("private detail=%v", err)
	}
	got := referenceResponse(t, referenceRequest(colleague, referenceRouter(e), "contact", fixture.id, ids.NewV7()))
	if len(got) != 0 {
		t.Fatalf("private references=%+v", got)
	}
	if own := referenceResponse(t, referenceRequest(e.Admin(), referenceRouter(e), "contact", fixture.id)); len(own) != 1 {
		t.Fatalf("owner reference=%+v", own)
	}
}

func TestRecordReferenceStatementCountDoesNotGrowWithIDs(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			first := makeReferenceFixture(t, e, kind, "First cost reference")
			second := makeReferenceFixture(t, e, kind, "Second cost reference")
			counts := make([]int64, 0, 2)
			for _, want := range [][]ids.UUID{{first.id}, {first.id, second.id}} {
				ctx, meter := database.WithStatementMeter(e.Admin())
				got, err := first.read(ctx, want)
				if err != nil || len(got) != len(want) {
					t.Fatalf("read=%+v err=%v", got, err)
				}
				counts = append(counts, meter.Cost().Statements)
			}
			if counts[0] == 0 || counts[0] != counts[1] {
				t.Fatalf("statement counts=%v", counts)
			}
		})
	}
}

func TestRecordReferencesRejectMalformedRequests(t *testing.T) {
	e := integration.Setup(t)
	router := referenceRouter(e)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"kind=unknown&id=" + ids.NewV7().String(), 422},
		{"kind=contact&id=not-a-uuid", 400},
		{"kind=contact", 400},
	} {
		request := httptest.NewRequest(http.MethodGet, "/v1/records/references?"+tc.query, nil).WithContext(e.Admin())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s: status=%d body=%s", tc.query, response.Code, response.Body.String())
		}
	}
}

func TestRecordReferencesPreserveNamesWhenOtherFieldsAreMasked(t *testing.T) {
	e := integration.Setup(t)
	fixture := makeReferenceFixture(t, e, "deal", "Masked amount deal")
	perms := integration.AdminPerms
	perms.RowScope = principal.RowScopeTeam
	perms.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, perms)
	detail, err := e.Deals.GetDeal(reader, ids.From[ids.DealKind](fixture.id), storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	if detail.MaskedFields == nil || len(*detail.MaskedFields) == 0 {
		t.Fatal("fixture did not apply the mask")
	}
	got := referenceResponse(t, referenceRequest(reader, referenceRouter(e), "deal", fixture.id))
	if len(got) != 1 || got[0].Name == nil || *got[0].Name != detail.Name {
		t.Fatalf("reference=%+v, detail name=%q", got, detail.Name)
	}
}

func TestRecordReferencesFollowTheDetailReadersScope(t *testing.T) {
	for _, kind := range []string{"contact", "company", "lead", "deal", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			fixture := makeReferenceFixture(t, e, kind, "Scope reference")
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.RowScope = principal.RowScopeOwn
			perms.Objects[kind] = principal.ObjectGrant{Read: true}
			reader := e.As(e.Rep3, []ids.UUID{e.Team2}, perms)
			detail, err := fixture.detail(reader)
			got := referenceResponse(t, referenceRequest(reader, referenceRouter(e), kind, fixture.id))
			if errors.Is(err, apperrors.ErrNotFound) {
				if len(got) != 0 {
					t.Fatalf("hidden detail returned reference: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || !reflect.DeepEqual(got[0], detail) {
				t.Fatalf("reference=%+v detail=%+v", got, detail)
			}
		})
	}
}

func TestRecordReferencesKeepNamesOfMergedRecords(t *testing.T) {
	for _, kind := range []string{"contact", "company"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			source := makeReferenceFixture(t, e, kind, "Folded reference")
			target := makeReferenceFixture(t, e, kind, "Surviving reference")
			if kind == "contact" {
				_, err := e.Contacts.MergeContact(e.Admin(), ids.From[ids.ContactKind](source.id), ids.From[ids.ContactKind](target.id), nil)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_, err := e.Contacts.MergeCompany(e.Admin(), ids.From[ids.CompanyKind](source.id), ids.From[ids.CompanyKind](target.id), nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			want, err := source.detail(e.Admin())
			if err != nil {
				t.Fatal(err)
			}
			got := referenceResponse(t, referenceRequest(e.Admin(), referenceRouter(e), kind, source.id))
			if len(got) != 1 || !reflect.DeepEqual(got[0], want) || !got[0].Archived {
				t.Fatalf("merged reference=%+v, detail=%+v", got, want)
			}
			legacy, err := source.labels(e.Admin(), []ids.UUID{source.id})
			if err != nil || len(legacy) != 0 {
				t.Fatalf("merged legacy labels=%+v err=%v", legacy, err)
			}
		})
	}
}
