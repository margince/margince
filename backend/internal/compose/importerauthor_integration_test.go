// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who wrote an imported record arrives with the create that lands it, on all
// six record wires, and only from a declared importer. The mapper tests hold
// each door's wiring; this file shows the handler asks, the value reaches the
// column, and a seat that does not exist answers the caller rather than the
// foreign key.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADeclaredImporterNamesTheAuthorOnEveryRecordWire(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())
	seat := openapi_types.UUID(e.Rep2)
	name := "Anna Müller"
	author := wireAuthor{id: &seat, name: &name}

	landed := map[string]string{}
	for _, w := range recordWires(t, e, "mirror:hubspot", author) {
		status, id := serveCreate(importer, t, w.path, w.body, w.serve)
		if status != http.StatusCreated {
			t.Errorf("%s answered %d, want 201: the importer states who wrote the record", w.table, status)
			continue
		}
		landed[w.table] = id
	}
	system := "mirror:hubspot"
	status, activity := postActivity(importer, t, e, crmcontracts.CreateActivityRequest{
		Kind: "email", Subject: strPtrIT("Betreff"), SourceSystem: &system, SourceId: strPtrIT("emails:950"),
		SourceAuthorId: &seat, SourceAuthorName: &name,
	})
	if status != http.StatusCreated {
		t.Fatalf("the activity answered %d, want 201", status)
	}
	landed["activity"] = ids.UUID(activity.Id).String()
	status, lead := postLead(importer, t, e, crmcontracts.CreateLeadRequest{
		FullName: strPtrIT("Imported Lead"), SourceSystem: &system, SourceId: strPtrIT("leads:950"),
		Source: "hubspot_import", SourceAuthorId: &seat, SourceAuthorName: &name,
	})
	if status != http.StatusCreated {
		t.Fatalf("the lead answered %d, want 201", status)
	}
	landed["lead"] = ids.UUID(lead.Id).String()

	if len(landed) != 6 {
		t.Fatalf("landed %d of six record wires", len(landed))
	}
	for table, id := range landed {
		if got := e.WsScalar(t, `SELECT source_author_id::text FROM `+table+` WHERE id = $1`, id); got != e.Rep2.String() {
			t.Errorf("%s.source_author_id = %q, want the seat the importer named", table, got)
		}
		if got := e.WsScalar(t, `SELECT source_author_name FROM `+table+` WHERE id = $1`, id); got != name {
			t.Errorf("%s.source_author_name = %q, want the name kept beside the seat", table, got)
		}
	}
}

// Under an ORDINARY source system, so the namespace check has nothing to
// refuse: the author alone is what these callers are refused for.
func TestAnAuthorIsRefusedToEveryoneButTheImporter(t *testing.T) {
	e := integration.Setup(t)
	name := "Anna Müller"
	callers := map[string]context.Context{
		"agent with the grant":    e.AgentFor(t, e.AdminUser, nil, importerPerms()),
		"human without the grant": e.As(e.AdminUser, nil, integration.AdminPerms),
	}
	for _, w := range recordWires(t, e, "legacy_crm", wireAuthor{name: &name}) {
		for who, as := range callers {
			if status, _ := serveCreate(as, t, w.path, w.body, w.serve); status != http.StatusUnprocessableEntity {
				t.Errorf("%s as %s answered %d, want 422 reserved_source_author", w.table, who, status)
			}
		}
	}
}

func TestAnAuthorSeatThatDoesNotExistIsTheCallersMistake(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())
	ghost := openapi_types.UUID(ids.NewV7())
	for _, w := range recordWires(t, e, "mirror:hubspot", wireAuthor{id: &ghost}) {
		if status, _ := serveCreate(importer, t, w.path, w.body, w.serve); status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, want 422 unknown_seat rather than a foreign-key failure", w.table, status)
		}
	}
}

// An author name written through the importer's own door is content about a
// human, and the Art. 17 erasure clears it with the subject: on the contact, on
// its lead twin, and on the note filed only under them. Seeded through the real
// create wires, because a hand-written UPDATE proves nothing about them.
func TestErasingTheSubjectClearsTheAuthorTheImporterWrote(t *testing.T) {
	e := integration.Setup(t)
	importer := e.As(e.AdminUser, nil, importerPerms())
	system := "mirror:hubspot"
	name := "Anna Müller"
	address := openapi_types.Email("erased.subject@authored.example")

	contactBody, err := json.Marshal(crmcontracts.CreateContactRequest{
		Source:   "manual",
		FullName: "Erased Subject", SourceSystem: &system, SourceAuthorName: &name,
		Emails: &[]crmcontracts.ContactEmailInput{{Email: address}},
	})
	if err != nil {
		t.Fatalf("encoding the contact: %v", err)
	}
	c := contacts.NewHandlers(InstallationDB(e.Pool))
	status, contactID := serveCreate(importer, t, "/v1/contacts", contactBody, func(w http.ResponseWriter, r *http.Request) {
		c.CreateContact(w, r, crmcontracts.CreateContactParams{})
	})
	if status != http.StatusCreated {
		t.Fatalf("the contact answered %d, want 201", status)
	}
	leadEmail := address
	if status, _ := postLead(importer, t, e, crmcontracts.CreateLeadRequest{
		FullName: strPtrIT("Erased Subject"), Email: &leadEmail, SourceSystem: &system, SourceId: strPtrIT("leads:960"),
		Source: "hubspot_import", SourceAuthorName: &name,
	}); status != http.StatusCreated {
		t.Fatalf("the lead twin answered %d, want 201", status)
	}
	subject, err := ids.Parse(contactID)
	if err != nil {
		t.Fatalf("parsing the contact id: %v", err)
	}
	links := []struct {
		EntityId   openapi_types.UUID                                `json:"entity_id"` //nolint:staticcheck // mirrors the generated inline struct, whose field is spelled EntityId
		EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: openapi_types.UUID(subject), EntityType: crmcontracts.CreateActivityRequestLinksEntityTypeContact}}
	status, note := postActivity(importer, t, e, crmcontracts.CreateActivityRequest{
		Kind: "note", Body: strPtrIT("about the subject"), SourceSystem: &system, SourceId: strPtrIT("notes:960"),
		SourceAuthorName: &name, Links: &links,
	})
	if status != http.StatusCreated {
		t.Fatalf("the note answered %d, want 201", status)
	}

	authored := map[string]struct {
		query string
		id    any
	}{
		"contact":   {`SELECT count(*) FROM contact WHERE id = $1 AND source_author_name IS NOT NULL`, subject},
		"lead twin": {`SELECT count(*) FROM lead WHERE source_id = $1 AND source_author_name IS NOT NULL`, "leads:960"},
		"note":      {`SELECT count(*) FROM activity WHERE id = $1 AND source_author_name IS NOT NULL`, ids.UUID(note.Id)},
	}
	for what, table := range authored {
		if got := e.WsScalar(t, table.query, table.id); got != "1" {
			t.Fatalf("%s carries no author name before the erasure, so clearing it would prove nothing", what)
		}
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}

	for what, table := range authored {
		if got := e.WsScalar(t, table.query, table.id); got != "0" {
			t.Errorf("%s still carries the author name after the erasure", what)
		}
	}
}
