// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// GET /audit-log over the production wiring. An entry names its record only
// when the reader may see it, and reads null when hidden, erased or unnamed.

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditPageLabels reads one full audit page as the caller and answers each
// entity id's label, "" for a null one.
func auditPageLabels(ctx context.Context, t *testing.T, e *integration.Env) map[ids.UUID]string {
	t.Helper()
	server := newServer(e.Pool, slog.New(slog.DiscardHandler),
		identity.NewHandlers(identity.NewService(e.Pool)), deals.NewHandlers(InstallationDB(e.Pool), DealsInstallation()))
	router := crmcontracts.HandlerFromMuxWithBaseURL(server, chi.NewRouter(), "/v1")
	request := httptest.NewRequest(http.MethodGet, "/v1/audit-log?limit=200", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /audit-log: status %d: %s", response.Code, response.Body.String())
	}
	var page struct {
		Data []crmcontracts.AuditLogEntry `json:"data"`
		Page crmcontracts.PageInfo        `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatalf("decoding the audit page: %v", err)
	}
	if page.Page.HasMore {
		t.Fatal("the seeded trail outgrew one page, so a missing label could be a missing row")
	}
	labels := map[ids.UUID]string{}
	for _, entry := range page.Data {
		id := ids.UUID(entry.EntityId)
		if entry.EntityLabel != nil {
			labels[id] = *entry.EntityLabel
		} else if _, named := labels[id]; !named {
			labels[id] = ""
		}
	}
	return labels
}

// catalogAdmin is the harness admin plus the product grant the real admin seed
// carries and the harness grid leaves out.
func catalogAdmin(e *integration.Env) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["product"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return e.As(e.AdminUser, nil, perms)
}

func adminActor(e *integration.Env) identity.Identity {
	return identity.Identity{
		UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		Roles: []string{"admin"}, SeatType: string(principal.SeatFull), Permissions: integration.AdminPerms,
	}
}

// seedCatalog writes one record of each administered kind through its own
// module's writer, so each lands with the audit row production writes.
func seedCatalog(t *testing.T, e *integration.Env) map[string]ids.UUID {
	t.Helper()
	admin := catalogAdmin(e)
	db := InstallationDB(e.Pool)
	dealStore := deals.NewStore(db, DealsInstallation())
	pipeline, err := dealStore.CreatePipeline(admin, deals.CreatePipelineInput{Name: "Retrofit pipeline", Position: 7})
	if err != nil {
		t.Fatalf("creating a pipeline: %v", err)
	}
	stage, err := dealStore.CreateStage(admin, deals.CreateStageInput{
		PipelineID: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id)), Name: "Site survey", Semantic: "open",
	})
	if err != nil {
		t.Fatalf("creating a stage: %v", err)
	}
	product, err := dealStore.CreateProduct(admin, deals.CreateProductInput{Name: "Depot sensor", UnitPriceMinor: 4900, Currency: "EUR"})
	if err != nil {
		t.Fatalf("creating a product: %v", err)
	}
	tag, err := collections.NewStore(db).CreateTag(admin, "Key account", nil, nil)
	if err != nil {
		t.Fatalf("creating a tag: %v", err)
	}
	list, err := collections.NewStore(db).CreateList(admin, collections.CreateListInput{Name: "Q3 targets", EntityType: "contact"})
	if err != nil {
		t.Fatalf("creating a list: %v", err)
	}
	team, err := identity.NewServiceFor(db).CreateTeam(admin, adminActor(e), "Field sales")
	if err != nil {
		t.Fatalf("creating a team: %v", err)
	}
	if _, err := identity.NewServiceFor(db).CreateRole(admin, adminActor(e), "rep", "Regional lead"); err != nil {
		t.Fatalf("creating a role: %v", err)
	}
	role, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM role WHERE name = 'Regional lead'`))
	if err != nil {
		t.Fatalf("reading the role's id: %v", err)
	}
	return map[string]ids.UUID{
		"Regional lead": role, "Retrofit pipeline": ids.UUID(pipeline.Id), "Site survey": ids.UUID(stage.Id),
		"Depot sensor": ids.UUID(product.Id), "Key account": tag.ID.UUID, "Q3 targets": list.ID.UUID,
		"Field sales": team.ID,
	}
}

func TestEveryAuditEntryNamesTheRecordItIsAbout(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	want := seedCatalog(t, e)
	want["Dana Weiss"] = e.SeedContact(t, "Dana Weiss", nil)
	company := e.SeedCompany(t, "Weber GmbH", nil)
	want["Weber GmbH"] = company
	want["Fleet retrofit"] = e.SeedDeal(t, "Fleet retrofit",
		ids.From[ids.PipelineKind](want["Retrofit pipeline"]), ids.From[ids.StageKind](want["Site survey"]), nil)
	project, err := ProjectsStore(e.Pool).CreateProject(admin, projects.CreateProjectInput{
		Name: "Depot rollout", CompanyID: ids.From[ids.CompanyKind](company),
	})
	if err != nil {
		t.Fatalf("creating a project: %v", err)
	}
	want["Depot rollout"] = ids.UUID(project.Id)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, err := identity.NewServiceFor(InstallationDB(e.Pool)).SaveMyDisplayName(rep, "Rita Rep"); err != nil {
		t.Fatalf("renaming a seat: %v", err)
	}
	want["Rita Rep"] = e.Rep1

	labels := auditPageLabels(catalogAdmin(e), t, e)

	for name, id := range want {
		got, onPage := labels[id]
		if !onPage {
			t.Errorf("no audit entry is about %q (%s); the writer under test did not audit it", name, id)
			continue
		}
		if got != name {
			t.Errorf("entity_label = %q for %s, want %q", got, id, name)
		}
	}
}

func TestAnAuditEntryOutsideTheReadersScopeReadsNull(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	mine := e.SeedContact(t, "Dana Weiss", &e.Rep1)
	private := e.SeedContact(t, "Zeta Privatkontakt", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", private, e.Rep3)
	company := e.SeedCompany(t, "Weber GmbH", nil)
	lists := collections.NewStore(InstallationDB(e.Pool))
	ownList, err := lists.CreateList(admin, collections.CreateListInput{
		Name: "My targets", EntityType: "contact", Sharing: "private", OwnerID: userID(e.Rep1), StewardID: userID(e.Rep1),
	})
	if err != nil {
		t.Fatalf("creating the reader's list: %v", err)
	}
	theirList, err := lists.CreateList(admin, collections.CreateListInput{
		Name: "Their targets", EntityType: "contact", Sharing: "private", OwnerID: userID(e.Rep3), StewardID: userID(e.Rep3),
	})
	if err != nil {
		t.Fatalf("creating another rep's list: %v", err)
	}

	// A rep handed the trail but holding only their own rows. The grant on
	// audit_log admits the page and must not widen what its labels name.
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"audit_log": {Read: true}, "contact": {Read: true}, "list": {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	})
	if _, err := lists.GetList(reader, theirList.ID); err == nil {
		t.Fatal("the reader can open the other rep's private list, so this fixture proves no scope")
	}
	labels := auditPageLabels(reader, t, e)

	if labels[mine] != "Dana Weiss" || labels[ownList.ID.UUID] != "My targets" {
		t.Fatalf("labels = %q, %q for the reader's own records, want their names",
			labels[mine], labels[ownList.ID.UUID])
	}
	for name, id := range map[string]ids.UUID{
		"Zeta Privatkontakt": private, "Their targets": theirList.ID.UUID, "Weber GmbH": company,
	} {
		if got, onPage := labels[id]; !onPage || got != "" {
			t.Errorf("entity_label = %q (on page: %v) for %q, which this reader may not see; want a null label on a present row",
				got, onPage, name)
		}
	}
}

func TestAnErasedRecordsEntriesReadNull(t *testing.T) {
	e := integration.Setup(t)
	admin := e.Admin()
	erased := e.SeedContact(t, "Erika Mustermann", nil)
	if err := privacy.NewEraser(InstallationDB(e.Pool)).EraseContact(admin, erased, "subject request"); err != nil {
		t.Fatalf("erasing the contact: %v", err)
	}
	archived := e.SeedCompany(t, "Closed Holding", nil)
	e.WsExec(t, `UPDATE company SET archived_at = now() WHERE id = $1`, archived)

	labels := auditPageLabels(admin, t, e)

	for name, id := range map[string]ids.UUID{"Erika Mustermann": erased, "Closed Holding": archived} {
		if got, onPage := labels[id]; !onPage || got != "" {
			t.Errorf("entity_label = %q (on page: %v) for %q, which is gone; want a null label on a present row",
				got, onPage, name)
		}
	}
}

func userID(id ids.UUID) *ids.UserID {
	typed := ids.From[ids.UserKind](id)
	return &typed
}
