// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"errors"
	"maps"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordRoutes is the app's hash route for each record type that has a page,
// a mirror of frontend/src/lib/entity.ts ENTITY that
// gates/webhookrecordroutes_test.go holds to it.
var recordRoutes = map[string]string{
	recordContact: "contacts",
	recordCompany: "companies",
	recordDeal:    "deals",
	recordLead:    "leads",
	recordProject: "projects",
}

// The record types a delivery can link to, as an event's entity names them.
const (
	recordContact = "contact"
	recordCompany = "company"
	recordDeal    = "deal"
	recordLead    = "lead"
	recordProject = "project"
)

// namedRecords are the record types whose name a delivery carries, with the
// table and column that hold it; each table bears its record type's name.
// Contacts and leads are left out: their name would stay in stored
// deliveries past an erasure, and the link is enough.
var namedRecords = map[string]struct{ table, column string }{
	recordDeal:    {recordDeal, "name"},
	recordCompany: {recordCompany, "display_name"},
	recordProject: {recordProject, "name"},
}

// RecordRoutes returns a copy of the record routes, for the gate that holds
// them to the frontend's.
func RecordRoutes() map[string]string {
	return maps.Clone(recordRoutes)
}

// recordURL links a record to its page, or nil when the type has no page or
// the installation has no public base URL.
func recordURL(baseURL, entityType string, id ids.UUID) *string {
	route, ok := recordRoutes[entityType]
	if !ok || baseURL == "" || id.IsZero() {
		return nil
	}
	link := strings.TrimRight(baseURL, "/") + "/#/" + route + "/" + id.String()
	return &link
}

// recordName reads a named record's name, archived or not, so an archive
// event still says what was archived. found is false when the type carries
// no name or the record is gone.
func (s *Store) recordName(ctx context.Context, entityType string, id ids.UUID) (name string, found bool, err error) {
	named, ok := namedRecords[entityType]
	if !ok {
		return "", false, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT "+pgx.Identifier{named.column}.Sanitize()+
			" FROM "+pgx.Identifier{named.table}.Sanitize()+" WHERE id = $1", id).Scan(&name)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return name, err == nil, err
}

// humanize adds the record's name and link to a public envelope, once per
// event, before any subscription's body is rendered. A failed name read is
// logged and the event still goes out without it.
func (d *Deliverer) humanize(ctx context.Context, wire *crmcontracts.PublicEventEnvelope) {
	id := ids.UUID(wire.Entity.Id)
	wire.Entity.Url = recordURL(d.publicBaseURL, wire.Entity.Type, id)
	name, found, err := d.store.recordName(ctx, wire.Entity.Type, id)
	if err != nil {
		d.log.Warn("webhooks: reading the record name for a delivery", "entity", wire.Entity.Type, "id", id, "err", err)
		return
	}
	if found {
		wire.Entity.Name = &name
	}
}
