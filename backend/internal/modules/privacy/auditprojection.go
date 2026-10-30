// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// One projection of a record's audit spine: the columns, the joins that resolve
// the names in them, and the decode that reads them back.
//
// All three live together because they are one thing. The scanner is shared by
// both readers, so a column added to a hand-written list in one query and not
// the other breaks the other's scan on arity — the column list has to be as
// shared as the decode is, or the pair only agrees on the day it was written.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auditattribution"
)

// recordAuditColumns is the projection both readers of a record's audit spine
// select. One spelling, because the SCANNER below is shared: a column added to
// one query and not the other breaks the other's scan on arity, and the two
// would otherwise agree only on the day somebody wrote them.
const recordAuditColumns = `
		a.id, a.actor_type, a.actor_id, a.on_behalf_of, a.action, a.occurred_at,
		a.authorization_rule, a.before, a.after, a.passport_id,
		` + auditattribution.ActorNameColumn + ` AS actor_display_name,
		obo.display_name AS on_behalf_of_display_name, oc.client_name,
		` + reversalLinkColumn

// auditRowScanner is what both readers of this projection have in common: a
// pgx.Row and a pgx.Rows both scan. The column list is recordAuditColumns and
// the decode is here, so the projection has ONE spelling end to end: a column
// added to the list is scanned by both readers or by neither, where two
// hand-written lists would drift the moment one of them grew.
type auditRowScanner interface {
	Scan(dest ...any) error
}

func scanRecordAuditRow(src auditRowScanner, r *recordAuditRow) error {
	return scanRecordAuditRowWith(src, r)
}

// scanRecordAuditRowWith decodes the shared projection and, after it, whatever
// columns the caller's own SELECT appended — the record-history window carries
// the edge subject there. Trailing rather than woven in, because the projection
// above is what the two readers share and only one of them widens its window: a
// column spliced into the middle would make the narrow reader's scan a
// positional guess.
func scanRecordAuditRowWith(src auditRowScanner, r *recordAuditRow, trailing ...any) error {
	var beforeJSON, afterJSON []byte
	var reversalLink *string
	dests := []any{
		&r.id, &r.actorType, &r.actorID, &r.onBehalfOf, &r.action, &r.occurredAt,
		&r.authorizationRule, &beforeJSON, &afterJSON, &r.passportID,
		&r.actorDisplayName, &r.onBehalfOfName, &r.agentClientName, &reversalLink,
	}
	if err := src.Scan(append(dests, trailing...)...); err != nil {
		return err
	}
	undid, err := reversalLinkFromColumn(reversalLink)
	if err != nil {
		return fmt.Errorf("audit row %s reversal link: %w", r.id, err)
	}
	r.undidAuditLogID = undid
	if err := unmarshalJSONBMap(beforeJSON, &r.before); err != nil {
		return fmt.Errorf("audit row %s before: %w", r.id, err)
	}
	if err := unmarshalJSONBMap(afterJSON, &r.after); err != nil {
		return fmt.Errorf("audit row %s after: %w", r.id, err)
	}
	return nil
}
