// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordRoutes is the app's hash route for each record type that has a page,
// a mirror of frontend/src/lib/entity.ts ENTITY that recordroutes_test.go
// holds to it.
var recordRoutes = map[string]string{
	string(datasource.RecordContact): "contacts",
	string(datasource.RecordCompany): "companies",
	string(datasource.RecordDeal):    "deals",
	string(datasource.RecordLead):    "leads",
	string(datasource.RecordProject): "projects",
}

// namedRecords are the record types whose name a delivery carries, with the
// table and column that hold it. Contacts and leads are left out: their name
// would stay in stored deliveries past an erasure, and the link is enough.
//
// Not the attention-names labels (compose/attentionnames.go): those read live
// records only, and an archive event still has to say what was archived.
var namedRecords = map[string]struct{ table, column string }{
	string(datasource.RecordDeal):    {tableDeal, "name"},
	string(datasource.RecordCompany): {tableCompany, "display_name"},
	string(datasource.RecordProject): {tableProject, "name"},
}

const (
	tableDeal    = "deal"
	tableCompany = "company"
	tableProject = "project"
)

// recordURL links a record to its page, or nil when the type has no page or
// the installation has no public base URL. baseURL carries no trailing slash.
func recordURL(baseURL, entityType string, id ids.UUID) *string {
	route, ok := recordRoutes[entityType]
	if !ok || baseURL == "" || id.IsZero() {
		return nil
	}
	link := baseURL + "/#/" + route + "/" + id.String()
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
