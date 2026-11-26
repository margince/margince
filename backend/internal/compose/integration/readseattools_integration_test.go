// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The stores admit a read seat's write, so each door must refuse it first.
// A passport lent by a read-seat member runs no write tool and no write route.
// The same passport writes once the member holds a full seat again.
func TestAPassportLentByAReadSeatRunsNoWrite(t *testing.T) {
	d := newTwoDoors(t, "seat-tools", "read", "write")
	e := d.e
	e.DescribeCompany(t)
	seeded := apptest.DiscoverSeededPipeline(t, e)
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Seat Tools", "source": "manual"})
	contact := createdID(t, e, "/v1/contacts", AnyMap{"linkedin_url": apptest.ContactProfile(), "full_name": "Seat Contact", "source": "manual"})
	lead := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Seat Lead", "email": "seat-tools@lead.test", "source": "manual"})
	project := createdID(t, e, "/v1/projects", AnyMap{"name": "Seat Project", "company_id": company, "source": "manual"})
	records := []struct {
		recordType, route, id string
		fields                AnyMap
		archives              bool
	}{
		{"contact", "/v1/contacts/", contact, AnyMap{"title": "Renamed"}, true},
		{"deal", "/v1/deals/", apptest.CreateOpenDeal(t, e, seeded), AnyMap{"name": "Renamed"}, true},
		{"lead", "/v1/leads/", lead, AnyMap{"full_name": "Renamed"}, false},
		{"project", "/v1/projects/", project, AnyMap{"name": "Renamed"}, true},
	}

	e.SetWorkspaceSeat(t, "read")
	for _, record := range records {
		target := AnyMap{"record_type": record.recordType, "id": record.id}
		update := AnyMap{"why": "A test asks for this change.", "record_type": record.recordType, "id": record.id, "fields": record.fields}
		requireSeatRefusal(t, record.recordType+" update_record", d.toolRefused(t, "update_record", update))
		if record.archives {
			requireSeatRefusal(t, record.recordType+" archive_record", d.toolRefused(t, "archive_record", target))
		}
		requireSeatRefusal(t, record.recordType+" PATCH",
			string(d.restStatus(t, http.MethodPatch, record.route+record.id, record.fields, http.StatusForbidden)))
	}
	var written int
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.id)
	}
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM audit_log
		WHERE entity_id::text = ANY($1) AND action IN ('update', 'archive')`, ids).Scan(&written); err != nil {
		t.Fatal(err)
	}
	if written != 0 {
		t.Fatalf("%d writes landed for a read seat's passport, want 0", written)
	}

	e.SetWorkspaceSeat(t, "full")
	deal := records[1]
	d.toolSeed(t, "update_record", AnyMap{"why": "A test asks for this change.", "record_type": deal.recordType, "id": deal.id, "fields": deal.fields})
}

func requireSeatRefusal(t *testing.T, call, refusal string) {
	t.Helper()
	if !strings.Contains(refusal, "seat") {
		t.Errorf("%s on a read seat was refused for another reason: %s", call, refusal)
	}
}
