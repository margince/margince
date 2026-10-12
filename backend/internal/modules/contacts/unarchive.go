// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Bringing an archived contact or company back: the statements storekit's
// un-archive runs for each, and the entry points that ask for it. What an
// archive records and why an un-archive refuses are in storekit/unarchive.go.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// restoreRelationship brings a link back only while every record it links is
// live: a live link to an archived record would answer the lists that record
// left.
const restoreRelationship = `UPDATE relationship r SET archived_at = NULL
	WHERE r.id = $1 AND r.archived_at = $2
	  AND NOT EXISTS (SELECT 1 FROM contact c WHERE c.id IN (r.contact_id, r.counterparty_contact_id) AND c.archived_at IS NOT NULL)
	  AND NOT EXISTS (SELECT 1 FROM company c WHERE c.id IN (r.company_id, r.counterparty_company_id) AND c.archived_at IS NOT NULL)
	  AND NOT EXISTS (SELECT 1 FROM deal d WHERE d.id = r.deal_id AND d.archived_at IS NOT NULL)
	  AND NOT EXISTS (SELECT 1 FROM project p WHERE p.id = r.project_id AND p.archived_at IS NOT NULL)`

// The tables a company archive retires that no other file here names, and the
// field an un-archive names when another record holds that value now.
const (
	tableCompanyDomain = "company_domain"
	tablePartner       = "partner"
	takenEmail         = "email"
	takenDomain        = "domain"
)

var contactUnarchive = storekit.UnarchiveShape{
	Table: contactEntity,
	Lock:  `SELECT full_name, archived_at, merged_into_id IS NOT NULL, version FROM contact WHERE id = $1 FOR UPDATE`,
	Taken: `SELECT e.email FROM contact_email e
		WHERE e.id = ANY($1) AND e.archived_at = $2
		  AND EXISTS (SELECT 1 FROM contact_email live WHERE live.email = e.email AND live.archived_at IS NULL)
		LIMIT 1`,
	TakenFrom: tableContactEmail, TakenField: takenEmail,
	Children: []storekit.ChildRestore{
		{Table: tableContactEmail, Statement: `UPDATE contact_email SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: tableContactPhone, Statement: `UPDATE contact_phone SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: "contact_channel_identity", Statement: `UPDATE contact_channel_identity SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: tableRelationship, Statement: restoreRelationship},
	},
	Restored: crmcontracts.PublicEventContactRestored{},
}

var companyUnarchive = storekit.UnarchiveShape{
	Table: companyEntity,
	Lock:  `SELECT display_name, archived_at, merged_into_id IS NOT NULL, version FROM company WHERE id = $1 FOR UPDATE`,
	Taken: `SELECT d.domain FROM company_domain d
		WHERE d.id = ANY($1) AND d.archived_at = $2
		  AND EXISTS (SELECT 1 FROM company_domain live WHERE live.domain = d.domain AND live.archived_at IS NULL)
		LIMIT 1`,
	TakenFrom: tableCompanyDomain, TakenField: takenDomain,
	// Types before the partner row: the partner invariant reads live types.
	Children: []storekit.ChildRestore{
		{Table: tableCompanyDomain, Statement: `UPDATE company_domain SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: "company_relationship_type", Statement: `UPDATE company_relationship_type SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: tablePartner, Statement: `UPDATE partner SET archived_at = NULL WHERE id = $1 AND archived_at = $2`},
		{Table: tableRelationship, Statement: restoreRelationship},
	},
	Restored: crmcontracts.PublicEventCompanyRestored{},
}

// RestoreContactTx brings an archived contact back on the caller's
// transaction, conditioned on ifVersion. with.Erased is required
// (storekit.RestoreWith says why).
func (s *Store) RestoreContactTx(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, ifVersion *int64, with storekit.RestoreWith,
) (storekit.RestoreReport, error) {
	if err := ensureRestorable(ctx, tx, contactEntity, id.UUID); err != nil {
		return storekit.RestoreReport{}, err
	}
	return storekit.Unarchive(ctx, tx, contactUnarchive, id.UUID, ifVersion, with)
}

// RestoreCompanyTx is RestoreContactTx for a company.
func (s *Store) RestoreCompanyTx(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, ifVersion *int64, with storekit.RestoreWith,
) (storekit.RestoreReport, error) {
	if err := ensureRestorable(ctx, tx, companyEntity, id.UUID); err != nil {
		return storekit.RestoreReport{}, err
	}
	return storekit.Unarchive(ctx, tx, companyUnarchive, id.UUID, ifVersion, with)
}

// ensureRestorable asks what the archive asks: the delete grant on the type
// and write authority on the row. Not the live twin — the row is archived by
// definition, and bringing it back is the write that reaches it on purpose.
func ensureRestorable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := auth.Require(ctx, table, principal.ActionDelete); err != nil {
		return err
	}
	return auth.EnsureWritable(ctx, tx, table, id)
}
