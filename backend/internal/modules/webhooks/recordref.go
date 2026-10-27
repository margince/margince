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

// namedRecords are the record types whose name and owner a delivery carries,
// with the table and column that hold the name. Each table keeps its owner in
// owner_id. Contacts and leads are left out: their name would stay in stored
// deliveries past an erasure, and the link is enough.
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

// recordRef is what a delivery says about its record beyond the id.
type recordRef struct {
	name      string
	ownerName *string
}

// readRecordRef reads a named record's name and its owner's display name,
// archived or not, so an archive event still says what was archived. found is
// false when the type carries no name or the record is gone.
func (s *Store) readRecordRef(ctx context.Context, entityType string, id ids.UUID) (ref recordRef, found bool, err error) {
	named, ok := namedRecords[entityType]
	if !ok {
		return recordRef{}, false, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT r."+pgx.Identifier{named.column}.Sanitize()+", u.display_name"+
			" FROM "+pgx.Identifier{named.table}.Sanitize()+" r LEFT JOIN app_user u ON u.id = r.owner_id"+
			" WHERE r.id = $1", id).Scan(&ref.name, &ref.ownerName)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return recordRef{}, false, nil
	}
	return ref, err == nil, err
}

// humanize adds the record's name, owner and link to a public envelope, once
// per event, before any subscription's body is rendered. A failed read is
// logged and the event still goes out without them.
func (d *Deliverer) humanize(ctx context.Context, wire *crmcontracts.PublicEventEnvelope) {
	id := ids.UUID(wire.Entity.Id)
	wire.Entity.Url = recordURL(d.publicBaseURL, wire.Entity.Type, id)
	ref, found, err := d.store.readRecordRef(ctx, wire.Entity.Type, id)
	if err != nil {
		d.log.Warn("webhooks: reading the record name and owner for a delivery", "entity", wire.Entity.Type, "id", id, "err", err)
		return
	}
	if found {
		wire.Entity.Name, wire.Entity.OwnerName = &ref.name, ref.ownerName
	}
}
